// `bv ls` lists the authenticated tenant's sites.

package main

import (
	"context"
	"fmt"

	"github.com/htxryan/butverify/internal/api"
)

func runList(ctx context.Context, g globalContext, args []string) int {
	_ = args
	client, _, err := newClient(g)
	if err != nil {
		return reportError(g.w, err)
	}
	var resp api.ListSitesResponse
	if err := client.Do(ctx, "GET", "/v1/sites", nil, &resp); err != nil {
		return reportError(g.w, err)
	}
	if g.w.IsJSON() {
		_ = g.w.JSON(resp)
		return 0
	}
	if len(resp.Sites) == 0 {
		g.w.Human("(no sites)")
		return 0
	}
	// Human format: a tabular header + per-site rows. We don't import a
	// table library; fmt.Sprintf with fixed widths is plenty for a list
	// that's bounded at the paid tier (50 sites).
	g.w.Human("%-12s  %-8s  %12s  %s", "SITE", "STATUS", "BYTES", "URL")
	for _, s := range resp.Sites {
		bytes := fmt.Sprintf("%d", s.BytesUsed)
		g.w.Human("%-12s  %-8s  %12s  %s", s.SiteID, s.Status, bytes, s.URL)
	}
	return 0
}
