// `bv rm <site-id>` soft-deletes a site (DELETE /v1/sites/{id}).

package main

import (
	"context"
	"errors"

	"github.com/htxryan/butverify/internal/api"
)

func runRemove(ctx context.Context, g globalContext, args []string) int {
	if len(args) < 1 {
		g.w.Error(toErrorEnvelope(errors.New("usage: bv rm <site-id>")))
		return 2
	}
	siteID := args[0]
	client, _, err := newClient(g)
	if err != nil {
		return reportError(g.w, err)
	}
	var resp api.DeleteSiteResponse
	if err := client.Do(ctx, "DELETE", "/v1/sites/"+siteID, nil, &resp); err != nil {
		return reportError(g.w, err)
	}
	if g.w.IsJSON() {
		_ = g.w.JSON(resp)
		return 0
	}
	g.w.Human("Removed %s (status=%s)", resp.SiteID, resp.Status)
	return 0
}
