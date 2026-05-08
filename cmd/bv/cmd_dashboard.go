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
	"fmt"

	"github.com/htxryan/butverify/pkg/templates"
)

func runDashboard(ctx context.Context, g globalContext, args []string) int {
	fs, flags := newCLIFlagSet("dashboard")
	from := flags.String("from")
	out := flags.String("out")
	push := flags.Bool("push")
	title := flags.String("title")
	subtitle := flags.String("subtitle")
	maxRows := flags.Int("max-table-rows")
	uploadIDFlag := flags.String("upload-id")
	ttlFlag := flags.Int64("ttl-seconds")
	imageQuality := flags.Int("image-quality")
	modeFlag := flags.String("mode")
	if err := fs.Parse(args); err != nil {
		return handleFlagParseError(g, "dashboard", err)
	}
	if *from == "" {
		g.w.Error(toErrorEnvelope(usageError("dashboard")))
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
		g.w.Success("Dashboard rendered to %s (%d rows)", outDir, rowCount)
		g.w.Hint("To publish:  bv push %s", outDir)
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
	return runPushFlowForMode(ctx, g, pushOptions{
		dir:          outDir,
		sourcePath:   publishSourcePath(*from),
		uploadID:     uploadID,
		ttlSeconds:   *ttlFlag,
		template:     "dashboard",
		imageQuality: *imageQuality,
		modeOverride: *modeFlag,
	})
}
