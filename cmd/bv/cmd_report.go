// `bv report --from <out.json>` — render a JSON file into a static report
// site and (optionally with --push) upload it via the standard push flow.
//
// Two-step usage (render only):
//
//	bv report --from out.json --out ./build/report
//	bv push ./build/report   # standard push flow
//
// One-step usage (render + push):
//
//	bv report --from out.json --push
//
// In one-step mode the rendered files go to a temp directory that is cleaned
// up after the push completes (success or failure). The CLI passes
// `template=report` on POST /v1/sites so the server can count the templated
// site against the tenant's monthly fairness counter (closes O-3).

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/htxryan/butverify/pkg/templates"
)

func runReport(ctx context.Context, g globalContext, args []string) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	from := fs.String("from", "", "input JSON file (use - for stdin)")
	out := fs.String("out", "", "output directory (defaults to ./bv-report when --push not set)")
	push := fs.Bool("push", false, "after rendering, push the directory as a new site")
	uploadIDFlag := fs.String("upload-id", "", "explicit upload_id for idempotent --push retry")
	ttlFlag := fs.Int64("ttl-seconds", 0, "site TTL in seconds (paid plan; 0 = use server default)")
	if err := fs.Parse(args); err != nil {
		g.w.Error(toErrorEnvelope(err))
		return 2
	}
	if *from == "" {
		g.w.Error(toErrorEnvelope(errors.New("usage: bv report --from <out.json|-> [--out DIR] [--push] [--ttl-seconds N]")))
		return 2
	}

	input, err := readInput(*from)
	if err != nil {
		return reportError(g.w, fmt.Errorf("read --from: %w", err))
	}

	outDir, cleanup, err := resolveOutDir(*out, *push, "bv-report-")
	if err != nil {
		return reportError(g.w, err)
	}
	defer cleanup()

	in, err := templates.RenderReport(input, outDir, templates.Generator{Version: Version})
	if err != nil {
		return reportError(g.w, err)
	}
	g.w.Status("Rendered report (%d sections) to %s", len(in.Sections), outDir)

	if !*push {
		// Render-only mode: emit a small JSON summary so a piped consumer
		// can chain into `bv push` programmatically.
		if g.w.IsJSON() {
			_ = g.w.JSON(struct {
				OK       bool   `json:"ok"`
				OutDir   string `json:"out_dir"`
				Title    string `json:"title"`
				Sections int    `json:"sections"`
				Template string `json:"template"`
			}{true, outDir, in.Title, len(in.Sections), "report"})
			return 0
		}
		g.w.Human("Report rendered to %s", outDir)
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
		template:   "report",
	})
}

// readInput slurps the contents of path, or stdin when path == "-".
func readInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

// resolveOutDir returns the directory to render into. Behavior:
//   - explicit --out: use that directory verbatim (created if missing).
//   - --push without --out: render into a temp dir that is removed when
//     cleanup() runs.
//   - neither: default to "./<defaultName>" so a render-only invocation has
//     a predictable output location.
//
// The cleanup callback always runs in `defer`; it is a no-op for explicit
// --out paths so we never delete a user-controlled directory.
func resolveOutDir(explicit string, push bool, tempPrefix string) (string, func(), error) {
	if explicit != "" {
		if err := os.MkdirAll(explicit, 0o755); err != nil {
			return "", func() {}, fmt.Errorf("create --out: %w", err)
		}
		return explicit, func() {}, nil
	}
	if push {
		dir, err := os.MkdirTemp("", tempPrefix)
		if err != nil {
			return "", func() {}, fmt.Errorf("create temp dir: %w", err)
		}
		return dir, func() { _ = os.RemoveAll(dir) }, nil
	}
	def := "./" + tempPrefix + "out"
	if err := os.MkdirAll(def, 0o755); err != nil {
		return "", func() {}, fmt.Errorf("create default --out: %w", err)
	}
	return def, func() {}, nil
}
