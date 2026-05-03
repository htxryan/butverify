// Package tarbundle bundles a directory tree into a single ustar tar archive
// suitable for upload to butverify.dev's signed PUT URL.
//
// Design choices, in order of importance:
//
//  1. **Deterministic output**: file entries are sorted lexicographically and
//     mtimes are zeroed so the same input directory produces byte-identical
//     output across runs. This is what lets the agent CLI compute a
//     reproducible upload_id from the bundle hash if it ever needs to.
//  2. **Path validation matches the Worker's parser** — see
//     `apps/control-plane/src/tar.ts validateTarPath`. The CLI rejects paths
//     the server would reject so an agent fails locally with a good error
//     instead of waiting for a 400 round-trip.
//  3. **Regular files only**. Symlinks, devices, sockets, named pipes are
//     refused. We follow the rule "build what the Worker accepts." The
//     server treats symlinks as a security hazard, so we do too.
//  4. **Total-bytes tracking**: callers get a TotalBytes on success so they
//     can assert against the upload_max_bytes returned by POST /v1/sites
//     before hitting the network.
//  5. **Stdlib only**: no third-party deps. Go stdlib's archive/tar is the
//     reference writer that the Worker's parser is calibrated against —
//     using anything else risks subtle incompatibilities.
package tarbundle

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/htxryan/butverify/internal/imageopt"
)

// Options control how a directory is bundled. The zero value is fine for
// most callers.
type Options struct {
	// MaxBytes caps the total uncompressed body bytes (sum of file sizes).
	// If zero, no client-side cap is enforced — the server's signed-URL
	// Content-Length cap will reject oversized uploads anyway, but a
	// well-behaved CLI surfaces the failure earlier.
	MaxBytes int64

	// IncludeHidden controls whether dot-files are included. Default is
	// false because:
	//   * `.git/`, `.DS_Store`, `.env`, `.vscode/` shipping to a public
	//     viewer is almost never what an agent intends.
	//   * It matches the convention of all major static-site hosts
	//     (Vercel, Netlify, GitHub Pages all skip dot-files by default).
	IncludeHidden bool

	// ImageQuality controls JPEG recompression quality for image files in the
	// upload bundle. Zero disables image optimization.
	ImageQuality int
}

// BundleInfo describes the bundle that was written.
type BundleInfo struct {
	// FileCount is the number of regular files written. Zero means the
	// directory was empty after filtering — the caller should refuse the
	// push (Worker will 400 anyway).
	FileCount int
	// TotalBytes is the sum of file body sizes (does NOT include tar
	// header/padding overhead). Use this to validate against the
	// Worker-supplied upload_max_bytes.
	TotalBytes int64
	// Files is the list of relative paths included, sorted lexically.
	Files []string
}

// ErrEmpty is returned by BundleDir when the source directory contains no
// regular files (after filtering).
var ErrEmpty = errors.New("tarbundle: directory contains no regular files")

const maxImageOptimizeInputBytes int64 = 25 * 1024 * 1024

// BundleDir walks srcDir and writes a USTAR archive of its regular files to
// w. Returns a summary on success or an error describing the failure (with
// the offending path inline so an agent can react).
//
// Path layout: entries are written as `<relative_path>` from srcDir. We do
// NOT include srcDir itself as a top-level directory (e.g. "out/index.html");
// the bundle is the *contents* of srcDir.
func BundleDir(srcDir string, w io.Writer, opts Options) (BundleInfo, error) {
	abs, err := filepath.Abs(srcDir)
	if err != nil {
		return BundleInfo{}, fmt.Errorf("tarbundle: resolve source: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return BundleInfo{}, fmt.Errorf("tarbundle: stat source: %w", err)
	}
	if !info.IsDir() {
		return BundleInfo{}, fmt.Errorf("tarbundle: %s is not a directory", srcDir)
	}

	// Collect first, sort, write second. The Worker parser doesn't require
	// sorted input but the manifest sorts entries by path for deterministic
	// hashing; making the archive deterministic too keeps byte-for-byte
	// equality across CLI invocations a reachable property.
	type entry struct {
		rel       string
		full      string
		size      int64
		optimized []byte
	}
	var entries []entry
	walkErr := filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == abs {
			return nil
		}
		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return fmt.Errorf("tarbundle: relpath: %w", err)
		}
		// Always use forward slashes in the tar — the Worker validates
		// paths under POSIX rules, and Windows-style backslashes would be
		// rejected.
		rel = filepath.ToSlash(rel)
		base := filepath.Base(path)
		if !opts.IncludeHidden && strings.HasPrefix(base, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		// Regular file? d.Type() is a fs.FileMode; non-regular includes
		// symlinks, sockets, devices, etc.
		if !d.Type().IsRegular() {
			return fmt.Errorf("tarbundle: refusing non-regular file: %s", rel)
		}
		// Validate the path under the same rules the Worker enforces. We
		// do this here so a bad path produces a clear local error instead
		// of a 400 round-trip.
		if err := ValidatePath(rel); err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return fmt.Errorf("tarbundle: stat %s: %w", rel, err)
		}
		entries = append(entries, entry{rel: rel, full: path, size: fi.Size()})
		return nil
	})
	if walkErr != nil {
		return BundleInfo{}, walkErr
	}
	if len(entries) == 0 {
		return BundleInfo{}, ErrEmpty
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })

	for i := range entries {
		if opts.ImageQuality == 0 || !imageopt.CanOptimizePath(entries[i].rel) {
			continue
		}
		if entries[i].size > maxImageOptimizeInputBytes {
			continue
		}
		data, err := os.ReadFile(entries[i].full)
		if err != nil {
			return BundleInfo{}, fmt.Errorf("tarbundle: read %s: %w", entries[i].rel, err)
		}
		optimized, changed, err := imageopt.OptimizePath(entries[i].rel, data, imageopt.Options{Quality: opts.ImageQuality})
		if err != nil {
			return BundleInfo{}, fmt.Errorf("tarbundle: optimize %s: %w", entries[i].rel, err)
		}
		if changed {
			entries[i].optimized = optimized
			entries[i].size = int64(len(optimized))
		}
	}

	// Pre-flight cap check — sum of optimized file sizes against the byte budget.
	var total int64
	for _, e := range entries {
		total += e.size
		if opts.MaxBytes > 0 && total > opts.MaxBytes {
			return BundleInfo{}, fmt.Errorf(
				"tarbundle: bundle would exceed max_bytes=%d at %s (cumulative %d)",
				opts.MaxBytes, e.rel, total,
			)
		}
	}

	tw := tar.NewWriter(w)
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.rel,
			Size:     e.size,
			Mode:     0644,
			Typeflag: tar.TypeReg,
			Format:   tar.FormatUSTAR, // ustar prefix for paths > 100 chars
			// mtime/ctime/atime deliberately zero — deterministic output.
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return BundleInfo{}, fmt.Errorf("tarbundle: write header %s: %w", e.rel, err)
		}
		var n int64
		var err error
		if e.optimized != nil {
			_, err = tw.Write(e.optimized)
			n = int64(len(e.optimized))
		} else {
			f, openErr := os.Open(e.full)
			if openErr != nil {
				return BundleInfo{}, fmt.Errorf("tarbundle: open %s: %w", e.rel, openErr)
			}
			n, err = io.Copy(tw, f)
			_ = f.Close()
		}
		if err != nil {
			return BundleInfo{}, fmt.Errorf("tarbundle: copy %s: %w", e.rel, err)
		}
		if n != e.size {
			return BundleInfo{}, fmt.Errorf(
				"tarbundle: short read for %s: wrote %d, expected %d",
				e.rel, n, e.size,
			)
		}
		files = append(files, e.rel)
	}
	if err := tw.Close(); err != nil {
		return BundleInfo{}, fmt.Errorf("tarbundle: close: %w", err)
	}

	return BundleInfo{
		FileCount:  len(entries),
		TotalBytes: total,
		Files:      files,
	}, nil
}

// ValidatePath enforces the same rules as the Worker parser. Returns nil on
// a clean path, or an error describing why it was rejected. Exposed so the
// CLI can validate a list of paths before bundling (e.g. for `bv push
// --dry-run`).
//
// The rules are:
//   - non-empty, ≤1024 chars
//   - no NUL or control characters (incl. CR/LF/DEL)
//   - no backslashes (Windows-path smuggling)
//   - no leading slash (absolute path)
//   - no `.` or `..` segments
//   - no empty segments (// → ambiguous)
func ValidatePath(p string) error {
	if p == "" {
		return errors.New("tarbundle: empty path")
	}
	if len(p) > 1024 {
		return fmt.Errorf("tarbundle: path too long (%d bytes): %s", len(p), p)
	}
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c == 0 {
			return fmt.Errorf("tarbundle: path contains NUL: %q", p)
		}
		if c == '\\' {
			return fmt.Errorf("tarbundle: path contains backslash: %q", p)
		}
		if c < 0x20 || c == 0x7f {
			return fmt.Errorf("tarbundle: path contains control character: %q", p)
		}
	}
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("tarbundle: absolute path disallowed: %q", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" {
			return fmt.Errorf("tarbundle: empty path segment: %q", p)
		}
		if seg == "." || seg == ".." {
			return fmt.Errorf("tarbundle: path traversal disallowed: %q", p)
		}
	}
	return nil
}
