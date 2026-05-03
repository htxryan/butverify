// `bv dashboard --from <data.csv>` — render a CSV file into a static
// dashboard site (stat cards + per-numeric-column charts + data table) and
// optionally push it.
//
// Modes mirror `bv report`: --out for render-only, --push for the
// render+push pipeline. The CLI passes `template=dashboard` on POST
// /v1/sites so the server counts against the templated-site fairness cap.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/htxryan/butverify/pkg/templates"
)

func runDashboard(ctx context.Context, g globalContext, args []string) int {
	fs := flag.NewFlagSet("dashboard", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	from := fs.String("from", "", "input CSV file (use - for stdin)")
	out := fs.String("out", "", "output directory (defaults to ./bv-dashboard when --push not set)")
	push := fs.Bool("push", false, "after rendering, push the directory as a new site")
	title := fs.String("title", "", "page title (defaults to 'Dashboard')")
	subtitle := fs.String("subtitle", "", "page subtitle")
	maxRows := fs.Int("max-table-rows", 200, "cap rows shown in the HTML table (0 = no cap; data.csv always carries the full set)")
	uploadIDFlag := fs.String("upload-id", "", "explicit upload_id for idempotent --push retry")
	ttlFlag := fs.Int64("ttl-seconds", 0, "site TTL in seconds (paid plan; 0 = use server default)")
	if err := fs.Parse(args); err != nil {
		g.w.Error(toErrorEnvelope(err))
		return 2
	}
	if *from == "" {
		g.w.Error(toErrorEnvelope(errors.New("usage: bv dashboard --from <data.csv|-> [--out DIR] [--title T] [--push] [--ttl-seconds N]")))
		return 2
	}
	input, err := readInput(*from)
	if err != nil {
		return reportError(g.w, fmt.Errorf("read --from: %w", err))
	}

	outDir, cleanup, err := resolveOutDir(*out, *push, "bv-dashboard-")
	if err != nil {
		return reportError(g.w, err)
	}
	defer cleanup()

	rowCount, err := templates.RenderDashboard(input, outDir, templates.Generator{Version: Version},
		templates.DashboardOptions{Title: *title, Subtitle: *subtitle, MaxTableRows: *maxRows})
	if err != nil {
		return reportError(g.w, err)
	}
	g.w.Status("Rendered dashboard (%d rows) to %s", rowCount, outDir)

	if !*push {
		if g.w.IsJSON() {
			_ = g.w.JSON(struct {
				OK       bool   `json:"ok"`
				OutDir   string `json:"out_dir"`
				RowCount int    `json:"row_count"`
				Template string `json:"template"`
			}{true, outDir, rowCount, "dashboard"})
			return 0
		}
		g.w.Human("Dashboard rendered to %s (%d rows)", outDir, rowCount)
		g.w.Human("To publish:  bv push %s", outDir)
		return 0
	}

	uploadID := *uploadIDFlag
	if uploadID == "" {
		var err error
		uploadID, err = newUploadID()
		if err != nil {
			return reportError(g.w, fmt.Errorf("generate upload_id: %w", err))
		}
	}
	return runPushFlow(ctx, g, pushOptions{
		dir:        outDir,
		sourcePath: publishSourcePath(*from),
		uploadID:   uploadID,
		ttlSeconds: *ttlFlag,
		template:   "dashboard",
	})
}
