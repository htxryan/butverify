// `bv pin` and `bv unpin` toggle a site's TTL.

package main

import (
	"context"

	"github.com/htxryan/butverify/internal/api"
)

func runPin(ctx context.Context, g globalContext, args []string) int {
	return runPinUnpin(ctx, g, args, "pin")
}

func runUnpin(ctx context.Context, g globalContext, args []string) int {
	return runPinUnpin(ctx, g, args, "unpin")
}

func runPinUnpin(ctx context.Context, g globalContext, args []string, op string) int {
	if len(args) < 1 {
		g.w.Error(toErrorEnvelope(usageError(op)))
		return 2
	}
	siteID := args[0]
	client, _, err := newClient(g)
	if err != nil {
		return reportError(g.w, err)
	}
	var resp api.PinResponse
	if err := client.Do(ctx, "POST", "/v1/sites/"+siteID+"/"+op, struct{}{}, &resp); err != nil {
		return reportError(g.w, err)
	}
	if g.w.IsJSON() {
		_ = g.w.JSON(resp)
		return 0
	}
	if op == "pin" {
		g.w.Success("Pinned %s (TTL disabled)", resp.SiteID)
	} else {
		g.w.Success("Unpinned %s (expires_at=%s)", resp.SiteID, resp.ExpiresAt)
	}
	return 0
}
