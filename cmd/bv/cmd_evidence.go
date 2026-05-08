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
//	bv evidence --from in.json           # start validation workflow (EV-E-9)
//	bv evidence validate --item N        # mark item N validated (EV-E-9)
//
// When invoked without --push and without --out, the evidence command
// renders the bundle to .butverify/evidence-bundle/ and starts a
// human-in-the-loop validation workflow. The agent reviews each item
// one at a time by running `bv evidence validate --item N`; after all
// items are validated the CLI publishes automatically.
//
// Rendered evidence pages include a viewer-side layout switcher and image
// lightbox; layout is intentionally not a publish-time CLI option.
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
	"encoding/json"
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

// ===========================================================================
// Session types
// ===========================================================================

// evidenceSession persists the in-progress validation state between
// `bv evidence --from` and subsequent `bv evidence validate --item N`
// invocations. Stored at .butverify/evidence-session.json in CWD.
type evidenceSession struct {
	UploadID      string                `json:"upload_id"`
	BundleDir     string                `json:"bundle_dir"`
	FromPath      string                `json:"from_path"`
	TTLSeconds    int64                 `json:"ttl_seconds,omitempty"`
	EnableReviews bool                  `json:"enable_reviews,omitempty"`
	ModeOverride  string                `json:"mode_override,omitempty"`
	Items         []evidenceSessionItem `json:"items"`
}

// evidenceSessionItem tracks a single gallery entry during validation.
type evidenceSessionItem struct {
	Index     int    `json:"index"`      // 1-based
	Title     string `json:"title"`
	Desc      string `json:"desc"`
	AssetPath string `json:"asset_path"` // relative path from CWD to asset file
	Validated bool   `json:"validated"`
}

const evidenceSessionPath = ".butverify/evidence-session.json"

func saveEvidenceSession(s evidenceSession) error {
	if err := os.MkdirAll(filepath.Dir(evidenceSessionPath), 0o755); err != nil {
		return fmt.Errorf("evidence session mkdir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("evidence session marshal: %w", err)
	}
	return os.WriteFile(evidenceSessionPath, data, 0o644)
}

func loadEvidenceSession() (evidenceSession, error) {
	data, err := os.ReadFile(evidenceSessionPath)
	if err != nil {
		return evidenceSession{}, err
	}
	var s evidenceSession
	if err := json.Unmarshal(data, &s); err != nil {
		return evidenceSession{}, fmt.Errorf("evidence session parse: %w", err)
	}
	return s, nil
}

func deleteEvidenceSession() {
	_ = os.Remove(evidenceSessionPath)
}

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
	from          *string
	out           *string
	push          *bool
	schema        *bool
	uploadID      *string
	ttl           *int64
	quality       *int
	mode          *string
	enableReviews *bool
}

// newEvidenceFlagSet constructs the canonical `bv evidence` flag set.
// flag.ContinueOnError + io.Discard so callers (runEvidence, tests)
// own how parse errors are surfaced.
func newEvidenceFlagSet() (*flag.FlagSet, evidenceFlags) {
	fs, values := newCLIFlagSet("evidence")
	f := evidenceFlags{
		from:          values.String("from"),
		out:           values.String("out"),
		push:          values.Bool("push"),
		schema:        values.Bool("schema"),
		uploadID:      values.String("upload-id"),
		ttl:           values.Int64("ttl-seconds"),
		quality:       values.Int("image-quality"),
		mode:          values.String("mode"),
		enableReviews: values.Bool("enable-reviews"),
	}
	return fs, f
}

func runEvidence(ctx context.Context, g globalContext, args []string) int {
	// Subcommand dispatch: `bv evidence validate …` is handled before flag
	// parsing so `--item` is not seen as an unknown flag by newEvidenceFlagSet.
	if len(args) > 0 && args[0] == "validate" {
		return runEvidenceValidate(ctx, g, args[1:])
	}

	fs, f := newEvidenceFlagSet()
	from := f.from
	out := f.out
	push := f.push
	schema := f.schema
	uploadIDFlag := f.uploadID
	ttlFlag := f.ttl
	imageQuality := f.quality
	modeFlag := f.mode
	enableReviewsFlag := f.enableReviews
	if err := fs.Parse(args); err != nil {
		return handleFlagParseError(g, "evidence", err)
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
		g.w.Error(toErrorEnvelope(usageError("evidence")))
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
	// no flags: validation workflow — render to .butverify/evidence-bundle/.
	//
	// UseBundleV2: the CDN-bundle render path (Astro/Svelte gallery
	// served from /assets/evidence/v{ver}/_astro/) is the new default
	// per spec docs/specs/evidence-v2.md §3.1 (EV2-U-2). The CLI no
	// longer embeds the gallery JS/CSS — only the version string.

	// Determine the effective output dir and whether to clean first.
	effectiveOut := *out
	if !*push && *out == "" {
		// Validation workflow: use the well-known bundle directory.
		effectiveOut = filepath.Join(".butverify", "evidence-bundle")
		// Clean any existing bundle so RenderEvidence won't reject it.
		_ = os.RemoveAll(effectiveOut)
	}

	opts := templates.RenderOptions{
		OutDir:          effectiveOut,
		ContainmentRoot: containmentRoot,
		UseBundleV2:     true,
		EnableReviews:   *enableReviewsFlag,
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

	g.w.Status("Rendered evidence (%d items) to %s", len(in.Items), bundleDir)

	if !*push {
		// If --out was explicitly set (render-only mode), just report and exit.
		if *out != "" {
			if g.w.IsJSON() {
				_ = g.w.JSON(struct {
					OK       bool   `json:"ok"`
					OutDir   string `json:"out_dir"`
					Title    string `json:"title"`
					Items    int    `json:"items"`
					Template string `json:"template"`
				}{true, bundleDir, in.Title, len(in.Items), "evidence"})
				return 0
			}
			g.w.Human("Evidence rendered to %s (%d items)", bundleDir, len(in.Items))
			g.w.Human("To publish:  bv push %s", bundleDir)
			return 0
		}

		// No --out and no --push: start the validation workflow (EV-E-9).
		// Generate a stable upload_id for the eventual push.
		uploadID := *uploadIDFlag
		if uploadID == "" {
			var genErr error
			uploadID, genErr = newUploadID()
			if genErr != nil {
				return reportError(g.w, fmt.Errorf("generate upload_id: %w", genErr))
			}
		}

		// Build session items from the rendered bundle.
		sessionItems := make([]evidenceSessionItem, len(in.Items))
		for i, item := range in.Items {
			safeName := templates.SafeAssetName(item, i)
			assetAbs := filepath.Join(bundleDir, "assets", safeName)
			// Make the asset path relative to CWD for display.
			cwd, _ := os.Getwd()
			assetRel := assetAbs
			if rel, err := filepath.Rel(cwd, assetAbs); err == nil {
				assetRel = rel
			}
			sessionItems[i] = evidenceSessionItem{
				Index:     i + 1,
				Title:     item.Title,
				Desc:      item.Description,
				AssetPath: assetRel,
				Validated: false,
			}
		}

		session := evidenceSession{
			UploadID:      uploadID,
			BundleDir:     bundleDir,
			FromPath:      *from,
			TTLSeconds:    *ttlFlag,
			EnableReviews: *enableReviewsFlag,
			ModeOverride:  *modeFlag,
			Items:         sessionItems,
		}
		if err := saveEvidenceSession(session); err != nil {
			return reportError(g.w, fmt.Errorf("save evidence session: %w", err))
		}

		// Print the prompt for the first item.
		return printEvidenceItemPrompt(g, session.Items, 0, *from)
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
		imageQuality:       *imageQuality,
		modeOverride:       *modeFlag,
		enableReviews:      *enableReviewsFlag,
		createErrTransform: classifyTemplateRolloutErr,
	})
}

// printEvidenceItemPrompt writes the human/JSON validation prompt for one
// item in the session. itemIdx is the 0-based index into items.
// fromPath is the original --from value so the restart command is correct.
func printEvidenceItemPrompt(g globalContext, items []evidenceSessionItem, itemIdx int, fromPath string) int {
	if itemIdx >= len(items) {
		return reportError(g.w, fmt.Errorf("item index %d out of range (have %d items)", itemIdx, len(items)))
	}
	item := items[itemIdx]
	total := len(items)
	restartCmd := "bv evidence --from " + fromPath
	validateCmd := fmt.Sprintf("bv evidence validate --item %d", item.Index)

	if g.w.IsJSON() {
		type promptJSON struct {
			OK              bool   `json:"ok"`
			Action          string `json:"action"`
			ItemIndex       int    `json:"item_index"`
			ItemCount       int    `json:"item_count"`
			Title           string `json:"title,omitempty"`
			Description     string `json:"description,omitempty"`
			AssetPath       string `json:"asset_path"`
			ValidateCommand string `json:"validate_command"`
			RestartCommand  string `json:"restart_command"`
		}
		_ = g.w.JSON(promptJSON{
			OK:              true,
			Action:          "validate_item",
			ItemIndex:       item.Index,
			ItemCount:       total,
			Title:           item.Title,
			Description:     item.Desc,
			AssetPath:       item.AssetPath,
			ValidateCommand: validateCmd,
			RestartCommand:  restartCmd,
		})
		return 0
	}

	// Human output.
	header := fmt.Sprintf("  Item %d of %d", item.Index, total)
	if item.Title != "" {
		header += fmt.Sprintf(" — %q", item.Title)
	}
	ruler := strings.Repeat("─", 65)
	lines := []string{
		header,
		"  " + ruler,
	}
	if item.Desc != "" {
		lines = append(lines, fmt.Sprintf("  Description:  %s", item.Desc))
	}
	lines = append(lines,
		fmt.Sprintf("  Screenshot:   %s", item.AssetPath),
		"",
		"  Before continuing:",
		"    1. Open the screenshot/recording and inspect it carefully.",
		"    2. Confirm it accurately shows what is described above.",
		"    3. Check for errors, unexpected behavior, or incomplete states.",
		"       (Web apps: also check browser DevTools console for JS errors.)",
		"    4. If anything is wrong, fix it and restart:",
		"         "+restartCmd,
		"",
		fmt.Sprintf("  When verified:  %s", validateCmd),
	)
	g.w.Human("%s", strings.Join(lines, "\n"))
	return 0
}

// runEvidenceValidate handles `bv evidence validate --item N`.
func runEvidenceValidate(ctx context.Context, g globalContext, args []string) int {
	fs := flag.NewFlagSet("evidence-validate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	itemFlag := fs.Int("item", 0, "1-based item index to mark validated")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		g.w.Error(toErrorEnvelope(err))
		return 2
	}
	if *itemFlag < 1 {
		g.w.Error(toErrorEnvelope(fmt.Errorf("--item is required and must be >= 1 (got %d)", *itemFlag)))
		return 2
	}

	// Load session.
	session, err := loadEvidenceSession()
	if err != nil {
		if os.IsNotExist(err) {
			g.w.Error(toErrorEnvelope(fmt.Errorf("no evidence session in progress; run: bv evidence --from <file>")))
		} else {
			g.w.Error(toErrorEnvelope(fmt.Errorf("load evidence session: %w", err)))
		}
		return 1
	}

	// Validate item index.
	if *itemFlag > len(session.Items) {
		g.w.Error(toErrorEnvelope(fmt.Errorf("--item %d out of range (session has %d items)", *itemFlag, len(session.Items))))
		return 2
	}

	// Find the item (1-based index stored in session).
	idx := -1
	for i, it := range session.Items {
		if it.Index == *itemFlag {
			idx = i
			break
		}
	}
	if idx < 0 {
		g.w.Error(toErrorEnvelope(fmt.Errorf("--item %d not found in session", *itemFlag)))
		return 2
	}

	// Idempotent: warn if already validated but continue.
	if session.Items[idx].Validated {
		g.w.Status("Item %d was already validated; marking again (idempotent)", *itemFlag)
	}

	// Mark validated and persist.
	session.Items[idx].Validated = true
	if err := saveEvidenceSession(session); err != nil {
		return reportError(g.w, fmt.Errorf("save evidence session: %w", err))
	}

	// Find the next unvalidated item.
	nextIdx := -1
	for i, it := range session.Items {
		if !it.Validated {
			nextIdx = i
			break
		}
	}

	if nextIdx >= 0 {
		// More items to validate.
		return printEvidenceItemPrompt(g, session.Items, nextIdx, session.FromPath)
	}

	// All items validated — publish.
	total := len(session.Items)
	g.w.Status("All %d items validated  Publishing...", total)

	rc := runPushFlowForMode(ctx, g, pushOptions{
		dir:                session.BundleDir,
		sourcePath:         publishSourcePath(session.FromPath),
		uploadID:           session.UploadID,
		ttlSeconds:         session.TTLSeconds,
		template:           "evidence",
		modeOverride:       session.ModeOverride,
		enableReviews:      session.EnableReviews,
		createErrTransform: classifyTemplateRolloutErr,
	})
	if rc == 0 {
		deleteEvidenceSession()
	}
	return rc
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
