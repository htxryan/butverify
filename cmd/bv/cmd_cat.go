// `bv cat <site-id> <path>` streams a single file to stdout.

package main

import (
	"context"
	"io"
	"net/url"
	"os"
	"strings"
)

// escapePath percent-escapes each `/`-separated segment of a tar path so
// reserved URL characters (`?`, `#`, `&`, etc.) inside a filename don't get
// reinterpreted as query/fragment delimiters by the server's router. Slashes
// between segments are preserved as path separators (NOT escaped); a tar
// entry CANNOT contain `/` in a single segment because the path validator
// rejects it on the server side, so this is a faithful encoding.
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

func runCat(ctx context.Context, g globalContext, args []string) int {
	if len(args) < 2 {
		g.w.Error(toErrorEnvelope(usageError("cat")))
		return 2
	}
	siteID := args[0]
	path := args[1]
	client, _, err := newClient(g)
	if err != nil {
		return reportError(g.w, err)
	}
	// Per-segment PathEscape so a tar entry containing `?`, `#`, or `&`
	// doesn't get reinterpreted as URL query/fragment delimiters by
	// `url.URL.Parse`. Slashes between segments are preserved as path
	// separators (NOT escaped) because the server's route table walks the
	// `/files/<path>` suffix as a slash-delimited subtree.
	resp, err := client.DoRaw(ctx, "GET", "/v1/sites/"+siteID+"/files/"+escapePath(path), nil)
	if err != nil {
		return reportError(g.w, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
		return reportError(g.w, err)
	}
	return 0
}
