// `bv ls` lists the authenticated tenant's sites.

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/htxryan/butverify/internal/api"
	"github.com/htxryan/butverify/internal/output"
)

// styledStatusCell renders a status string padded to the given width, then
// wraps the padded form with the right semantic color (green for healthy,
// red for terminal, yellow for transitional). Wrapping AFTER padding keeps
// column alignment correct in TTY mode where ANSI escape bytes would
// otherwise be counted toward width.
func styledStatusCell(st *output.Styler, status, padded string) string {
	switch strings.ToLower(status) {
	case "active":
		return st.Green(padded)
	case "expired", "deleted":
		return st.Red(padded)
	case "creating", "uploading", "pending", "local":
		return st.Yellow(padded)
	default:
		return padded
	}
}

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
		g.w.Note("(no sites)")
		return 0
	}
	// Human format: a tabular header + per-site rows. We don't import a
	// table library; fmt.Sprintf with fixed widths is plenty for a list
	// that's bounded at the paid tier (50 sites). The header row is
	// dim+bold in TTY mode so the eye drops immediately to the data;
	// status cells get green/red/yellow based on the canonical lifecycle
	// (active/expired/creating). Plain text in non-TTY mode preserves
	// log-grepping integrations.
	st := g.w.StdoutStyler()
	header := fmt.Sprintf("%-12s  %-8s  %12s  %s", "SITE", "STATUS", "BYTES", "URL")
	g.w.Human("%s", st.Bold(header))
	for _, s := range resp.Sites {
		bytes := fmt.Sprintf("%d", s.BytesUsed)
		// Style each column independently so the trailing reset never
		// bleeds into the alignment math (we render width-padded plain
		// text, then wrap with ANSI codes after padding).
		idCol := fmt.Sprintf("%-12s", s.SiteID)
		statusCol := fmt.Sprintf("%-8s", s.Status)
		statusStyled := styledStatusCell(st, s.Status, statusCol)
		// Pad first, THEN style — wrapping with ANSI codes before
		// padding inflates the byte count and breaks column alignment.
		bytesCol := st.Dim(fmt.Sprintf("%12s", bytes))
		g.w.Human("%s  %s  %s  %s", idCol, statusStyled, bytesCol, st.Cyan(s.URL))
	}
	return 0
}
