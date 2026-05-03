// `bv evidence --from <evidence.json>` — render a JSON manifest into a
// static evidence/gallery site (image+video grid with stacked or
// carousel layout) and optionally push it via the standard push flow.
//
// Modes mirror `bv report` and `bv dashboard`:
//
//	bv evidence --schema                 # print JSON Schema, exit 0 (EV-E-1)
//	bv evidence --from in.json --out DIR # render-only (EV-E-2)
//	bv evidence --from in.json --push    # render to ephemeral tmp + push (EV-E-5)
//	bv evidence --from - --out DIR       # read manifest from stdin
//
// The CLI passes `template=evidence` on POST /v1/sites so the server
// counts the templated site against the tenant's monthly fairness
// counter (EV-O-1 / O-3). EV-E-8: the server-side rollout adding
// "evidence" to VALID_TEMPLATES may lag the CLI rollout; when POST
// /v1/sites returns HTTP 400 with a body indicating the template is
// not accepted, we surface a distinctive envelope so the agent can
// tell "rollout still in flight" apart from a real bug.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/htxryan/butverify/internal/api"
	"github.com/htxryan/butverify/pkg/templates"
)

// stdinIsTTY reports whether os.Stdin is currently attached to a
// terminal. Wired through a package-level var so tests can swap in a
// deterministic fake without an exec.Command subprocess. Production
// callers always go through the real os.Stdin path.
var stdinIsTTY = func() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		// Fail closed: if we can't tell, assume non-TTY so a piped
		// invocation in an unusual environment still works. The 4 MiB
		// cap inside templates.ReadStdin is the real safety net.
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// evidenceFlags bundles the parsed flag pointers for `bv evidence`.
// Single source of truth shared by runEvidence and the BVS-U-8(c)
// build-time round-trip test in cohesion_test.go (E13-T3):
// the test re-uses this exact flag set to verify every `bv evidence …`
// invocation in the embedded skill markdown parses cleanly. If
// runEvidence's flag surface ever drifts, the test fails before the
// binary ships.
type evidenceFlags struct {
	from     *string
	out      *string
	push     *bool
	schema   *bool
	layout   *string
	uploadID *string
	ttl      *int64
	mode     *string
}

// newEvidenceFlagSet constructs the canonical `bv evidence` flag set.
// flag.ContinueOnError + io.Discard so callers (runEvidence, tests)
// own how parse errors are surfaced.
func newEvidenceFlagSet() (*flag.FlagSet, evidenceFlags) {
	fs := flag.NewFlagSet("evidence", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	f := evidenceFlags{
		from:     fs.String("from", "", "input JSON file (use - for stdin)"),
		out:      fs.String("out", "", "output directory (omit when only --push is set)"),
		push:     fs.Bool("push", false, "after rendering, push the directory as a new site"),
		schema:   fs.Bool("schema", false, "print the JSON Schema for the evidence input and exit"),
		layout:   fs.String("layout", "stacked", "gallery layout: stacked (default) or carousel"),
		uploadID: fs.String("upload-id", "", "explicit upload_id for idempotent --push retry"),
		ttl:      fs.Int64("ttl-seconds", 0, "site TTL in seconds (paid plan; 0 = use server default)"),
		mode:     fs.String("mode", "", "publish mode for --push: local or remote (default: configured mode)"),
	}
	return fs, f
}

func runEvidence(ctx context.Context, g globalContext, args []string) int {
	fs, f := newEvidenceFlagSet()
	from := f.from
	out := f.out
	push := f.push
	schema := f.schema
	layout := f.layout
	uploadIDFlag := f.uploadID
	ttlFlag := f.ttl
	modeFlag := f.mode
	if err := fs.Parse(args); err != nil {
		g.w.Error(toErrorEnvelope(err))
		return 2
	}

	// EV-E-1: --schema short-circuits everything else.
	if *schema {
		// Schema goes to real stdout (not the writer's mode-aware
		// stdout) because consumers pipe it into jq / a schema
		// validator regardless of --json.
		_, _ = io.WriteString(os.Stdout, templates.EvidenceSchema)
		return 0
	}

	// EV-E-4: --from required when --schema is not set.
	if *from == "" {
		g.w.Error(toErrorEnvelope(errors.New("usage: bv evidence (--schema | --from <evidence.json|-> [--out DIR] [--push] [--layout stacked|carousel] [--ttl-seconds N])")))
		return 2
	}

	// EV-E-7: validate --layout up front so an unknown value fails
	// before any I/O.
	switch *layout {
	case "stacked", "carousel":
		// ok
	default:
		g.w.Error(toErrorEnvelope(fmt.Errorf("unknown --layout %q (supported values: stacked, carousel)", *layout)))
		return 2
	}

	// Read the manifest. We fork on stdin vs file because the stdin
	// path enforces the EV-N-6 4 MiB cap and the EV-E-4 TTY guard.
	var (
		input           []byte
		containmentRoot string
	)
	if *from == "-" {
		if stdinIsTTY() {
			g.w.Error(toErrorEnvelope(errors.New("--from - requires non-TTY stdin (pipe or redirect a JSON manifest)")))
			return 2
		}
		raw, err := templates.ReadStdin(os.Stdin)
		if err != nil {
			// EV-N-6 over-cap surfaces here; treat as usage error
			// (exit 2) because the agent's manifest is too big.
			g.w.Error(toErrorEnvelope(err))
			return 2
		}
		input = raw
		// EV-E-7: stdin → containment root is CWD.
		containmentRoot = "."
	} else {
		raw, err := os.ReadFile(*from)
		if err != nil {
			return reportError(g.w, fmt.Errorf("read --from: %w", err))
		}
		input = raw
		// EV-S-1 / spec §5: file mode → containment root is the file's
		// parent directory.
		containmentRoot = filepath.Dir(*from)
	}

	// --out + --push allowed (render to user dir, then push from there).
	// --out alone: render only. --push alone: ephemeral tmp dir.
	opts := templates.RenderOptions{
		Layout:          *layout,
		OutDir:          *out,
		ContainmentRoot: containmentRoot,
	}
	in, bundleDir, err := templates.RenderEvidence(input, opts, templates.Generator{Version: Version})
	if err != nil {
		return reportError(g.w, err)
	}

	// If --push is set without --out, RenderEvidence created an
	// ephemeral CLI-owned temp dir; we own cleanup. The deferred
	// RemoveAll runs whether the push succeeds or fails (EV-E-5). The
	// process-level signal handler in main.go cancels ctx, which the
	// HTTP client honors; the deferred cleanup then fires on the way
	// out. We do NOT install a competing handler — RenderEvidence's
	// own handler covers --out mode, and main.go's handler covers
	// process-level signals.
	cleanupEphemeral := func() {}
	if *out == "" && *push {
		dir := bundleDir
		cleanupEphemeral = func() { _ = os.RemoveAll(dir) }
	}
	defer cleanupEphemeral()

	g.w.Status("Rendered evidence (%d items, layout=%s) to %s", len(in.Items), *layout, bundleDir)

	if !*push {
		// Render-only mode: emit a small JSON summary so a piped
		// consumer can chain into `bv push` programmatically.
		if g.w.IsJSON() {
			_ = g.w.JSON(struct {
				OK       bool   `json:"ok"`
				OutDir   string `json:"out_dir"`
				Title    string `json:"title"`
				Items    int    `json:"items"`
				Layout   string `json:"layout"`
				Template string `json:"template"`
			}{true, bundleDir, in.Title, len(in.Items), *layout, "evidence"})
			return 0
		}
		g.w.Human("Evidence rendered to %s (%d items, layout=%s)", bundleDir, len(in.Items), *layout)
		g.w.Human("To publish:  bv push %s", bundleDir)
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
		dir:                bundleDir,
		sourcePath:         publishSourcePath(*from),
		uploadID:           uploadID,
		ttlSeconds:         *ttlFlag,
		template:           "evidence",
		modeOverride:       *modeFlag,
		createErrTransform: classifyTemplateRolloutErr,
	})
}

// classifyTemplateRolloutErr rewrites a 400 APIError from POST
// /v1/sites whose body indicates the server has not yet enabled the
// "evidence" template (EV-E-8). All other errors pass through
// unchanged so the standard reportError exit-code mapping still
// applies.
//
// We match the spec's canonical phrase ("template must be one of") as
// the primary signal, plus a less-specific fallback ("unknown
// template") so a wording shift on the server doesn't break the
// distinctive-envelope contract. The unit test in cmd_evidence_test.go
// pins both the matching and non-matching cases.
func classifyTemplateRolloutErr(err error) error {
	if err == nil {
		return nil
	}
	var ae *api.APIError
	if !errors.As(err, &ae) {
		return err
	}
	if ae.Status != 400 {
		return err
	}
	hay := strings.ToLower(ae.Message)
	if !strings.Contains(hay, "template must be one of") && !strings.Contains(hay, "unknown template") {
		return err
	}
	// Replace the message inside the APIError so the existing
	// reportError formatter surfaces our distinctive wording while
	// preserving the request-id, status, and code for downstream
	// branching.
	rewritten := *ae
	rewritten.Message = fmt.Sprintf(
		"evidence template not yet enabled on this control plane; retry after the server-side rollout completes (EV-E-8). Underlying error: %s",
		ae.Message,
	)
	return &rewritten
}
