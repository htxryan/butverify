package tarbundle

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// helper — write src files to tmpdir, return path.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return dir
}

// readTar parses the tar bytes and returns a map[path]body for assertions.
func readTar(t *testing.T, b []byte) map[string]string {
	t.Helper()
	r := tar.NewReader(bytes.NewReader(b))
	out := map[string]string{}
	for {
		h, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		body, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("tar body read: %v", err)
		}
		out[h.Name] = string(body)
	}
	return out
}

func TestBundleDir_HappyPath(t *testing.T) {
	src := writeTree(t, map[string]string{
		"index.html":   "<h1>hi</h1>",
		"style.css":    "body{}",
		"img/logo.png": "PNGFAKEDATA",
	})

	var buf bytes.Buffer
	info, err := BundleDir(src, &buf, Options{})
	if err != nil {
		t.Fatalf("BundleDir: %v", err)
	}
	if info.FileCount != 3 {
		t.Errorf("FileCount=%d, want 3", info.FileCount)
	}
	if info.TotalBytes != int64(len("<h1>hi</h1>")+len("body{}")+len("PNGFAKEDATA")) {
		t.Errorf("TotalBytes=%d", info.TotalBytes)
	}
	want := []string{"img/logo.png", "index.html", "style.css"}
	if got := info.Files; !equalStrings(got, want) {
		t.Errorf("Files=%v, want %v", got, want)
	}

	got := readTar(t, buf.Bytes())
	if got["index.html"] != "<h1>hi</h1>" {
		t.Errorf("index.html body mismatch")
	}
	if got["style.css"] != "body{}" {
		t.Errorf("style.css body mismatch")
	}
	if got["img/logo.png"] != "PNGFAKEDATA" {
		t.Errorf("img/logo.png body mismatch")
	}
}

func TestBundleDir_Deterministic(t *testing.T) {
	// Two independent runs over the same input → byte-identical output.
	files := map[string]string{
		"a.html": "a",
		"b.html": "b",
		"sub/c":  "c",
	}
	src1 := writeTree(t, files)
	src2 := writeTree(t, files)

	var b1, b2 bytes.Buffer
	if _, err := BundleDir(src1, &b1, Options{}); err != nil {
		t.Fatalf("BundleDir(src1): %v", err)
	}
	if _, err := BundleDir(src2, &b2, Options{}); err != nil {
		t.Fatalf("BundleDir(src2): %v", err)
	}
	if !bytes.Equal(b1.Bytes(), b2.Bytes()) {
		t.Errorf("non-deterministic bundle output (sizes %d vs %d)", b1.Len(), b2.Len())
	}
}

func TestBundleDir_SkipsHiddenByDefault(t *testing.T) {
	src := writeTree(t, map[string]string{
		"index.html":      "<p>",
		".env":            "SECRET=1",
		".git/HEAD":       "ref",
		".vscode/x.json":  "{}",
		"docs/.DS_Store":  "junk",
		"docs/index.html": "<p>",
	})
	var buf bytes.Buffer
	info, err := BundleDir(src, &buf, Options{})
	if err != nil {
		t.Fatalf("BundleDir: %v", err)
	}
	got := readTar(t, buf.Bytes())
	if _, ok := got[".env"]; ok {
		t.Error(".env should be skipped")
	}
	if _, ok := got[".git/HEAD"]; ok {
		t.Error(".git/HEAD should be skipped")
	}
	if _, ok := got[".vscode/x.json"]; ok {
		t.Error(".vscode/x.json should be skipped")
	}
	if _, ok := got["docs/.DS_Store"]; ok {
		t.Error("docs/.DS_Store should be skipped")
	}
	if got["index.html"] == "" {
		t.Error("index.html should be included")
	}
	if got["docs/index.html"] == "" {
		t.Error("docs/index.html should be included")
	}
	if info.FileCount != 2 {
		t.Errorf("FileCount=%d, want 2", info.FileCount)
	}
}

func TestBundleDir_IncludeHidden(t *testing.T) {
	src := writeTree(t, map[string]string{
		"index.html":               "<p>",
		".well-known/security.txt": "Contact: x@y.test",
	})
	var buf bytes.Buffer
	info, err := BundleDir(src, &buf, Options{IncludeHidden: true})
	if err != nil {
		t.Fatalf("BundleDir: %v", err)
	}
	got := readTar(t, buf.Bytes())
	if got[".well-known/security.txt"] == "" {
		t.Error(".well-known/security.txt should be included with IncludeHidden")
	}
	if info.FileCount != 2 {
		t.Errorf("FileCount=%d, want 2", info.FileCount)
	}
}

func TestBundleDir_EmptyDir(t *testing.T) {
	src := t.TempDir()
	var buf bytes.Buffer
	_, err := BundleDir(src, &buf, Options{})
	if !errors.Is(err, ErrEmpty) {
		t.Errorf("BundleDir empty err=%v, want ErrEmpty", err)
	}
}

func TestBundleDir_MaxBytesEnforced(t *testing.T) {
	src := writeTree(t, map[string]string{
		"a.html": strings.Repeat("a", 100),
		"b.html": strings.Repeat("b", 100),
	})
	var buf bytes.Buffer
	_, err := BundleDir(src, &buf, Options{MaxBytes: 150})
	if err == nil || !strings.Contains(err.Error(), "max_bytes") {
		t.Errorf("BundleDir max_bytes err=%v, want max_bytes error", err)
	}
}

func TestBundleDir_RejectsSymlink(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "real.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.html", filepath.Join(src, "link.html")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	var buf bytes.Buffer
	_, err := BundleDir(src, &buf, Options{})
	if err == nil || !strings.Contains(err.Error(), "non-regular") {
		t.Errorf("BundleDir symlink err=%v, want non-regular error", err)
	}
}

func TestBundleDir_NotADirectory(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "x.txt")
	if err := os.WriteFile(f, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	_, err := BundleDir(f, &buf, Options{})
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("BundleDir non-dir err=%v, want not-a-directory", err)
	}
}

func TestValidatePath(t *testing.T) {
	good := []string{"a", "a/b", "a/b/c.html", "img/logo.png", "ok-file_1.txt"}
	for _, p := range good {
		if err := ValidatePath(p); err != nil {
			t.Errorf("ValidatePath(%q) err=%v, want nil", p, err)
		}
	}
	bad := []string{
		"",
		"/abs",
		"../up",
		"./here",
		"a//b",
		"a\x00b",
		"a\\b",
		"a\nb",
		"a\x7fb",
	}
	for _, p := range bad {
		if err := ValidatePath(p); err == nil {
			t.Errorf("ValidatePath(%q) ok, want err", p)
		}
	}
	if err := ValidatePath("a/" + strings.Repeat("x", 2000)); err == nil {
		t.Error("ValidatePath long ok, want err")
	}
}

func TestBundleDir_PreservesPathEntries(t *testing.T) {
	// Every path in the manifest is reachable from the tar entries — no
	// silent skips after sorting.
	src := writeTree(t, map[string]string{
		"a.html":             "a",
		"sub/b.html":         "b",
		"deep/nested/c.html": "c",
	})
	var buf bytes.Buffer
	info, err := BundleDir(src, &buf, Options{})
	if err != nil {
		t.Fatalf("BundleDir: %v", err)
	}
	got := readTar(t, buf.Bytes())
	if len(got) != info.FileCount {
		t.Errorf("tar entries=%d, info.FileCount=%d", len(got), info.FileCount)
	}
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if !equalStrings(keys, info.Files) {
		t.Errorf("keys=%v, info.Files=%v", keys, info.Files)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
