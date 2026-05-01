// `bv manifest <site-id>` streams the manifest.json from the server to
// stdout. In --json mode the body is passed through verbatim (it's already
// valid JSON); in human mode we still print the raw JSON because that's
// what callers want to pipe into `jq`.

package main

import (
	"context"
	"errors"
	"io"
	"os"
)

func runManifest(ctx context.Context, g globalContext, args []string) int {
	if len(args) < 1 {
		g.w.Error(toErrorEnvelope(errors.New("usage: bv manifest <site-id>")))
		return 2
	}
	siteID := args[0]
	client, _, err := newClient(g)
	if err != nil {
		return reportError(g.w, err)
	}
	resp, err := client.DoRaw(ctx, "GET", "/v1/sites/"+siteID+"/manifest", nil)
	if err != nil {
		return reportError(g.w, err)
	}
	defer func() { _ = resp.Body.Close() }()
	// Stream straight to stdout — manifest.json can be large for a site
	// with many files; buffering serves no purpose.
	if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
		return reportError(g.w, err)
	}
	return 0
}
