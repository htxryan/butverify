// `bv get <site-id> <dest>` downloads every file in a site's manifest into
// <dest>, preserving the per-site directory layout. Sequential download at
// MVP; a parallel fetcher is a v1.x perf pass.

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/htxryan/butverify/internal/api"
)

func runGet(ctx context.Context, g globalContext, args []string) int {
	if len(args) < 2 {
		g.w.Error(toErrorEnvelope(usageError("get")))
		return 2
	}
	siteID := args[0]
	dest := args[1]
	client, _, err := newClient(g)
	if err != nil {
		return reportError(g.w, err)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return reportError(g.w, fmt.Errorf("mkdir %s: %w", dest, err))
	}
	var listing api.FilesListResponse
	if err := client.Do(ctx, "GET", "/v1/sites/"+siteID+"/files", nil, &listing); err != nil {
		return reportError(g.w, err)
	}
	written := 0
	totalBytes := int64(0)
	for _, f := range listing.Files {
		if !validateGetPath(f.Path) {
			return reportError(g.w, fmt.Errorf("server returned suspicious path %q", f.Path))
		}
		out := filepath.Join(dest, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return reportError(g.w, fmt.Errorf("mkdir for %s: %w", f.Path, err))
		}
		resp, err := client.DoRaw(ctx, "GET", "/v1/sites/"+siteID+"/files/"+escapePath(f.Path), nil)
		if err != nil {
			return reportError(g.w, err)
		}
		fp, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			_ = resp.Body.Close()
			return reportError(g.w, fmt.Errorf("open %s: %w", out, err))
		}
		n, copyErr := io.Copy(fp, resp.Body)
		_ = resp.Body.Close()
		_ = fp.Close()
		if copyErr != nil {
			return reportError(g.w, fmt.Errorf("write %s: %w", out, copyErr))
		}
		written++
		totalBytes += n
		g.w.Status("  %s %s", f.Path, g.w.StderrStyler().Dim(fmt.Sprintf("(%d bytes)", n)))
	}
	if g.w.IsJSON() {
		_ = g.w.JSON(struct {
			SiteID     string `json:"site_id"`
			FileCount  int    `json:"file_count"`
			TotalBytes int64  `json:"total_bytes"`
			Dest       string `json:"dest"`
		}{siteID, written, totalBytes, dest})
		return 0
	}
	g.w.Success("Downloaded %d files (%d bytes) to %s", written, totalBytes, dest)
	return 0
}

// validateGetPath defends against a malicious server that returns a path
// containing `..` or `/` shenanigans intended to escape the dest directory.
// The server already rejects these on write (tar.ts validateTarPath); this
// is defense in depth on read.
func validateGetPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}
