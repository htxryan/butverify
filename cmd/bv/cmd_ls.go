// `bv ls` lists the authenticated tenant's sites.
//
// By default, the list hides sites with status "expired". Pass --expired
// to also include them. The EXPIRES column shows each site's expiry in
// the user's local timezone plus a humanized magnitude phrase, e.g.
// "2026-05-09 10:38 AM (one hour)". Pinned sites (no expiry) render as
// an em dash.

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/htxryan/butverify/internal/api"
)

func runList(ctx context.Context, g globalContext, args []string) int {
	fs, values := newCLIFlagSet("ls")
	expired := values.Bool("expired")
	if err := fs.Parse(args); err != nil {
		return handleFlagParseError(g, "ls", err)
	}
	client, _, err := newClient(g)
	if err != nil {
		return reportError(g.w, err)
	}
	var resp api.ListSitesResponse
	if err := client.Do(ctx, "GET", "/v1/sites", nil, &resp); err != nil {
		return reportError(g.w, err)
	}
	filtered := filterSites(resp.Sites, *expired)
	if g.w.IsJSON() {
		_ = g.w.JSON(api.ListSitesResponse{Sites: filtered})
		return 0
	}
	if len(filtered) == 0 {
		g.w.Human("(no sites)")
		return 0
	}
	// Human format: a tabular header + per-site rows. We don't import a
	// table library; fmt.Sprintf with fixed widths is plenty for a list
	// that's bounded at the paid tier (50 sites).
	now := time.Now()
	g.w.Human("%-12s  %-8s  %-32s  %12s  %s", "SITE", "STATUS", "EXPIRES", "BYTES", "URL")
	for _, s := range filtered {
		bytes := fmt.Sprintf("%d", s.BytesUsed)
		g.w.Human("%-12s  %-8s  %-32s  %12s  %s", s.SiteID, s.Status, formatExpires(s.ExpiresAt, now), bytes, s.URL)
	}
	return 0
}

// filterSites returns the subset of sites visible under the current
// --expired flag. The filter is a no-op when includeExpired is true;
// otherwise it drops only sites whose status is exactly "expired".
// Sites with statuses creating/uploading/active/pinned/expiring/failed/
// suspended are always included.
func filterSites(sites []api.SiteSummary, includeExpired bool) []api.SiteSummary {
	if includeExpired {
		return sites
	}
	out := make([]api.SiteSummary, 0, len(sites))
	for _, s := range sites {
		if s.Status == "expired" {
			continue
		}
		out = append(out, s)
	}
	return out
}
