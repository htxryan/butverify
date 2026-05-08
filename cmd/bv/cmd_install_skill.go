// `bv install-skill <agent>` writes a pre-baked agent prompt (the
// canonical /butverify skill) into the agent's per-agent install
// location, so the user can immediately invoke /butverify in their
// next session and have the agent capture+publish proof of finished
// work.
//
// v1 supports a single agent: `claude` (Claude Code), which writes to
// `$HOME/.claude/skills/butverify/SKILL.md` (default) or to
// `./.claude/skills/butverify/SKILL.md` when --project is supplied.
//
// Authoritative spec: docs/specs/butverify-skill.md rev 3 — EARS
// BVS-U-1..9, BVS-E-1..6, BVS-S-1..2, BVS-N-1..3.
//
// Embed approach: the canonical source lives at
// `bv-skills/claude/butverify.md`. Go's `//go:embed` cannot
// reference paths above the package directory (the `..` operator is
// rejected at compile time), so we keep a build-time mirror at
// `embedded_skills/claude_butverify.md` and use a dedicated test
// (TestEmbedMirror_MatchesCanonical) to assert byte-equality with
// the canonical source. If the mirror drifts, the test fails before
// the binary ships.

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/htxryan/butverify/internal/cliref"
)

// embeddedSkillBytes carries the canonical /butverify (deprecated alias)
// skill markdown at compile time. The file is a build-time mirror of
// bv-skills/claude/butverify.md.
//
//go:embed embedded_skills/claude_butverify.md
var embeddedSkillBytes []byte

// embeddedProveItBytes carries the new /butverify:prove-it skill markdown.
// Mirror of bv-skills/claude/prove-it.md.
//
//go:embed embedded_skills/claude_prove-it.md
var embeddedProveItBytes []byte

// embeddedReviewBytes carries the new /butverify:review skill markdown.
// Mirror of bv-skills/claude/review.md.
//
//go:embed embedded_skills/claude_review.md
var embeddedReviewBytes []byte

const skillMetadataPrefix = "<!-- bv-skill:"

// skillEntry describes one installable skill file under a Claude Code
// agent's namespace. The v1 install plants three skills under
// `<root>/.claude/skills/butverify/`:
//
//   - SKILL.md            — deprecated alias preserved for one release.
//   - prove-it/SKILL.md   — renamed from the historical /butverify skill.
//   - review/SKILL.md     — new skill that surfaces unacknowledged reviews.
//
// Each entry carries its own embedded bytes and its own per-file install
// path (relative to the agent's namespace dir). All three go through the
// same atomic-rename + canonical-hash-stamp + drift-detect pipeline.
type skillEntry struct {
	// LeafPath is the install path relative to the namespace dir
	// (i.e. relative to `<root>/.claude/skills/butverify/`). The empty
	// segment list means a flat `SKILL.md` in the namespace dir; a
	// single segment like "prove-it" means `<namespace>/prove-it/SKILL.md`.
	LeafSegments []string
	Embedded     []byte
}

// claudeSkills is the v1 install set for the `claude` agent. The order
// is documentary; install/uninstall iterate in slice order for stable
// log output.
func claudeSkills() []skillEntry {
	return []skillEntry{
		{LeafSegments: nil, Embedded: embeddedSkillBytes},
		{LeafSegments: []string{"prove-it"}, Embedded: embeddedProveItBytes},
		{LeafSegments: []string{"review"}, Embedded: embeddedReviewBytes},
	}
}

func canonicalHashDomain(markdown []byte) []byte {
	normalized := normalizeLineEndings(markdown)
	lines := strings.Split(string(normalized), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), skillMetadataPrefix) {
		lines = lines[:len(lines)-1]
	}
	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return []byte(out)
}

// normalizeLineEndings converts CRLF and lone CR to LF. Used by
// canonicalHashDomain so the hash is independent of whether the file
// was checked out with autocrlf=true (Windows clones).
func normalizeLineEndings(b []byte) []byte {
	// Two-pass to keep the algorithm simple and obviously correct.
	// First pass: CRLF -> LF. Second pass: any remaining CR -> LF.
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return []byte(s)
}

// skillVersionHash returns the BVS-U-7 12-hex-char prefix of the
// SHA-256 of the canonical hash domain.
func skillVersionHash(markdown []byte) string {
	canon := canonicalHashDomain(markdown)
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:])[:12]
}

func stampVersion(markdown []byte, hash string) []byte {
	canon := canonicalHashDomain(markdown)
	return []byte(fmt.Sprintf("%s<!-- bv-skill: release=%s sha256=%s -->\n", string(canon), Version, hash))
}

// supportedAgents is the closed set of agent values the v1 installer
// accepts. BVS-E-5 errors list this set verbatim.
var supportedAgents = []string{"claude"}

// installSkillOptions captures the flag surface, normalized.
type installSkillOptions struct {
	Agent     string
	Force     bool
	Uninstall bool
	Project   bool
	// EnableHook is set by --enable-hook and overrides the TTY consent
	// prompt with an unconditional yes. NoHook is set by --no-hook and
	// overrides with an unconditional no. Both default false; if neither
	// is set and the installer is on a TTY, the installer prompts the
	// user. Non-TTY without --enable-hook silently skips hook install.
	EnableHook bool
	NoHook     bool
}

func runInstallSkill(ctx context.Context, g globalContext, args []string) int {
	fs, flags := newCLIFlagSet("install-skill")
	force := flags.Bool("force")
	uninstall := flags.Bool("uninstall")
	project := flags.Bool("project")
	enableHook := flags.Bool("enable-hook")
	noHook := flags.Bool("no-hook")

	// Allow `bv install-skill claude --force` (positional before
	// flags) the same as `bv install-skill --force claude`. The
	// stdlib flag package stops at the first non-flag, so we extract
	// the first non-flag arg up front and parse the remainder.
	flagsOnly, agent := splitAgentFromFlags(args)
	if err := fs.Parse(flagsOnly); err != nil {
		return handleFlagParseError(g, "install-skill", err)
	}
	// Detect any extra positional args after the agent. Two shapes:
	//
	//   - `bv install-skill claude foo`         — splitAgentFromFlags
	//     consumed `claude`, then appended the rest verbatim into
	//     flagsOnly. `foo` is non-flag, so fs.Parse() left it on
	//     fs.Args(). Reject with a usage error.
	//   - `bv install-skill --force claude foo` — all positionals land
	//     in fs.Args(); first is the agent, anything past index 0 is
	//     extra. Reject as well.
	//
	// Both branches converge on `extras` (the slice of positional args
	// past the agent). Surfacing a usage error here prevents a typo
	// (`bv install-skill claude --force` vs `bv install-skill claude
	// force`) from silently dropping the trailing token.
	var extras []string
	if agent == "" {
		// All-flags-first form. fs.Args() carries every positional.
		if rest := fs.Args(); len(rest) > 0 {
			agent = rest[0]
			extras = rest[1:]
		}
	} else {
		// agent-first form. fs.Args() holds only post-agent positionals.
		extras = fs.Args()
	}
	if agent == "" {
		// Usage error — surface the full help block (so the user sees
		// the scope choice inline) and a short structured envelope for
		// JSON consumers. Exit 2 per stdlib usage-error convention.
		printInstallSkillUsageError(g)
		return 2
	}
	if len(extras) > 0 {
		// AC-19: a typo like `bv install-skill claude foo`
		// previously parsed cleanly and silently dropped `foo`. Surface
		// it as a usage error instead so the typo is visible.
		if g.w.IsJSON() {
			g.w.Error(toErrorEnvelope(fmt.Errorf("unexpected extra arguments after agent: %v", extras)))
		} else {
			g.w.Status("bv install-skill: unexpected extra arguments after agent: %v", extras)
			g.w.Status("%s", cliref.CommandHelp("install-skill"))
		}
		return 2
	}
	opts := installSkillOptions{
		Agent:      agent,
		Force:      *force,
		Uninstall:  *uninstall,
		Project:    *project,
		EnableHook: *enableHook,
		NoHook:     *noHook,
	}
	if opts.EnableHook && opts.NoHook {
		g.w.Error(toErrorEnvelope(errors.New("--enable-hook and --no-hook are mutually exclusive")))
		return 2
	}

	// BVS-E-5: validate agent up front so an unsupported value never
	// runs the login probe or touches the filesystem.
	if !isSupportedAgent(opts.Agent) {
		g.w.Error(toErrorEnvelope(fmt.Errorf("unsupported agent %q (supported agents: %s)", opts.Agent, strings.Join(supportedAgents, ", "))))
		logInstallSkillError(g, opts.Agent, "UNSUPPORTED_AGENT", "")
		return 1
	}

	logInstallSkill(g, "started", opts.Agent, "")

	root, err := installRoot(opts)
	if err != nil {
		logInstallSkillError(g, opts.Agent, "ROOT_RESOLVE", "")
		return reportError(g.w, err)
	}

	skills := claudeSkills()
	// BVS-N-1 + BVS-N-3: containment check on every install path. We
	// resolve each install-target's PARENT (since the target itself may
	// not exist yet) under the install root. Mirrors EV-S-1 from the
	// evidence template.
	for _, e := range skills {
		p := claudeSkillPathFor(root, e)
		if _, err := containInstallPath(root, p); err != nil {
			g.w.Error(toErrorEnvelope(err))
			logInstallSkillError(g, opts.Agent, "CONTAINMENT", p)
			return 1
		}
	}

	if opts.Uninstall {
		return doUninstall(g, opts, root, skills)
	}
	return doInstall(g, opts, root, skills)
}

func runAgentInit(ctx context.Context, g globalContext, args []string) int {
	return runInstallSkill(ctx, g, append([]string{"claude"}, args...))
}

// splitAgentFromFlags walks `args` and pulls out the first non-flag
// token (the agent name), returning the remaining args (flags only)
// and the extracted agent. If no non-flag token is found, agent is
// the empty string and flagsOnly == args. Flag values that follow a
// space-separated flag name (e.g. `--out PATH`) are kept attached to
// the flag — we recognize the small closed set of flags this command
// accepts, all of which are bools (no value follows).
func splitAgentFromFlags(args []string) (flagsOnly []string, agent string) {
	flagsOnly = make([]string, 0, len(args))
	for i, a := range args {
		if strings.HasPrefix(a, "-") {
			flagsOnly = append(flagsOnly, a)
			continue
		}
		// First non-flag token: take it as the agent and append the
		// rest verbatim. Any later non-flag tokens flow through to
		// fs.Parse which will surface them as `fs.Args()` for an
		// extra-args usage check (currently we don't error, but a
		// future flag with arguments would benefit).
		agent = a
		flagsOnly = append(flagsOnly, args[i+1:]...)
		return flagsOnly, agent
	}
	return flagsOnly, ""
}

// isSupportedAgent reports whether agent is in the v1 supported set.
func isSupportedAgent(agent string) bool {
	for _, a := range supportedAgents {
		if a == agent {
			return true
		}
	}
	return false
}

// installRoot resolves the install-time root directory. Default is
// $HOME (BVS-U-3); --project flips to the absolute CWD. BVS-N-1
// rejects any other root (we don't expose --prefix at v1).
func installRoot(opts installSkillOptions) (string, error) {
	if opts.Project {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("install-skill: resolve cwd: %w", err)
		}
		abs, err := filepath.Abs(cwd)
		if err != nil {
			return "", fmt.Errorf("install-skill: cwd abs: %w", err)
		}
		return abs, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("install-skill: resolve $HOME: %w", err)
	}
	return home, nil
}

// claudeNamespaceDir returns the namespace dir
// `<root>/.claude/skills/butverify/`.
func claudeNamespaceDir(root string) string {
	return filepath.Join(root, ".claude", "skills", "butverify")
}

// claudeSkillPath returns the deprecated-alias path
// `<root>/.claude/skills/butverify/SKILL.md`. The new namespaced
// skills live one directory deeper (see claudeSkillPathFor).
func claudeSkillPath(root string) string {
	return filepath.Join(claudeNamespaceDir(root), "SKILL.md")
}

// claudeSkillPathFor resolves the absolute install path for one
// skillEntry under the namespace `<root>/.claude/skills/butverify/`.
// LeafSegments are joined into the path; the file is always named
// `SKILL.md` (Claude Code convention).
func claudeSkillPathFor(root string, e skillEntry) string {
	parts := append([]string{claudeNamespaceDir(root)}, e.LeafSegments...)
	parts = append(parts, "SKILL.md")
	return filepath.Join(parts...)
}

// containInstallPath enforces BVS-N-1 + BVS-N-3 by mirroring the
// EvalSymlinks+Rel algorithm in templates.containAsset (EV-S-1):
//
//  1. base   = filepath.EvalSymlinks(filepath.Clean(installRoot))
//  2. target = filepath.EvalSymlinks(parent(installPath))   — parent,
//     because the install path itself doesn't exist on first install,
//     and a not-yet-created file can't itself contain a symlink. We
//     fall back to the deepest extant ancestor when the parent dir
//     doesn't exist either, so a clean install (where the whole
//     `.claude/skills/butverify/` tree is missing) still passes the
//     containment gate.
//  3. require filepath.Rel(base, target) does NOT start with `..` and
//     is not absolute.
//
// Returns the resolved canonical install-target dir on success.
func containInstallPath(installRoot, installPath string) (string, error) {
	// (1) lexical pre-check using the un-symlink-resolved forms. This
	// catches explicit `..` escapes that don't depend on filesystem
	// state. We compare the cleaned absolute paths directly so a host
	// like macOS (where /var -> /private/var) doesn't false-positive
	// — both sides are un-resolved.
	absRoot, err := filepath.Abs(filepath.Clean(installRoot))
	if err != nil {
		return "", fmt.Errorf("install-skill: containment base abs %q: %w", installRoot, err)
	}
	absInstall, err := filepath.Abs(filepath.Clean(installPath))
	if err != nil {
		return "", fmt.Errorf("install-skill: containment install abs %q: %w", installPath, err)
	}
	if rel, relErr := filepath.Rel(absRoot, filepath.Dir(absInstall)); relErr == nil {
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return "", fmt.Errorf(
				"install-skill: install path %q escapes install root %q (lexical resolution outside root)",
				installPath, absRoot,
			)
		}
	}

	// (2) symlink-aware check. Resolve installRoot and the deepest
	// extant ancestor of the install target's parent through
	// EvalSymlinks, then take Rel. This catches a pre-existing symlink
	// (e.g. ~/.claude/skills/ pointing to /etc/skills/) redirecting
	// the atomic write outside the install root.
	cleanBase, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("install-skill: containment base %q: %w", installRoot, err)
	}
	cleanBase, err = filepath.Abs(cleanBase)
	if err != nil {
		return "", fmt.Errorf("install-skill: containment base abs %q: %w", installRoot, err)
	}

	resolved, err := resolveDeepestAncestor(filepath.Dir(absInstall))
	if err != nil {
		return "", fmt.Errorf("install-skill: resolve install target %q: %w", installPath, err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("install-skill: resolve abs %q: %w", installPath, err)
	}

	rel, err := filepath.Rel(cleanBase, resolved)
	if err != nil {
		return "", fmt.Errorf("install-skill: rel %q: %w", installPath, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf(
			"install-skill: install path %q escapes install root (resolved to %q, outside %q)",
			installPath, resolved, cleanBase,
		)
	}
	return resolved, nil
}

// resolveDeepestAncestor walks up `path` until it finds an extant
// directory whose symlinks resolve cleanly, then rejoins the unresolved
// tail. Returns the lexical join when no ancestor exists (e.g. on a
// freshly-mounted volume). Errors only on EvalSymlinks failures that
// are NOT "does-not-exist."
func resolveDeepestAncestor(path string) (string, error) {
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	ancestor := path
	var tail string
	for {
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return path, nil
		}
		if r, perr := filepath.EvalSymlinks(parent); perr == nil {
			if tail == "" {
				tail = filepath.Base(ancestor)
			} else {
				tail = filepath.Join(filepath.Base(ancestor), tail)
			}
			return filepath.Join(r, tail), nil
		} else if !os.IsNotExist(perr) {
			return "", perr
		}
		if tail == "" {
			tail = filepath.Base(ancestor)
		} else {
			tail = filepath.Join(filepath.Base(ancestor), tail)
		}
		ancestor = parent
	}
}

// readInstalledFrontmatter returns the bv-skill-version field from an
// installed SKILL.md, or ("", false, nil) when the file is absent.
// Tolerates unknown frontmatter fields per assumption A-1a (we only
// pluck the one key we care about). Returns an empty string + nil
// error when the file exists but the version key is missing — the
// caller treats this as "drift" because the installed file did not
// come from a v1 `bv install-skill`.
func readInstalledFrontmatter(path string) (version string, exists bool, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("install-skill: read installed SKILL.md %q: %w", path, err)
	}
	normalized := normalizeLineEndings(raw)
	lines := strings.Split(string(normalized), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > 0 {
		trimmed := strings.TrimSpace(lines[len(lines)-1])
		if strings.HasPrefix(trimmed, skillMetadataPrefix) && strings.HasSuffix(trimmed, "-->") {
			for _, field := range strings.Fields(strings.TrimSuffix(strings.TrimPrefix(trimmed, skillMetadataPrefix), "-->")) {
				if strings.HasPrefix(field, "sha256=") {
					return strings.TrimPrefix(field, "sha256="), true, nil
				}
			}
		}
	}
	return "", true, nil
}

// installOneOutcome captures the result of installing one skill. The
// multi-skill driver aggregates these into a single output payload so a
// run that touches three files reports per-file status.
type installOneOutcome struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Status  string `json:"status"` // "installed", "force_overwrote", "already_current"
}

// installPrep captures the per-skill state collected before any
// filesystem write. Lifted out of doInstall so helpers (e.g. flat-
// legacy migration detection) can range over it without aliasing a
// function-scoped type.
type installPrep struct {
	entry       skillEntry
	path        string
	hash        string
	stamped     []byte
	installed   bool
	instVer     string
	instContent string
}

// isFlatLegacyMigration reports whether the install is a "pre-namespace
// flat install" upgrade: the alias SKILL.md exists at the namespace dir
// AND none of the namespaced skills (prove-it / review) are installed
// yet. The metadata stamp is intentionally NOT consulted — older
// `bv install-skill` releases stamped a flat SKILL.md before the
// namespace refactor, so a clean stamped pre-refactor install is just
// as much a migration as a hand-edited or unstamped one. When true the
// drift error message switches to the migration-call-out variant so
// the user understands --force will move them onto the new layout
// rather than just clobbering one file.
func isFlatLegacyMigration(preps []installPrep) bool {
	flatExists := false
	subdirInstalled := false
	for _, p := range preps {
		if len(p.entry.LeafSegments) == 0 {
			if p.installed {
				flatExists = true
			}
			continue
		}
		if p.installed {
			subdirInstalled = true
		}
	}
	return flatExists && !subdirInstalled
}

// doInstall installs every skill in `skills` under `root`. BVS-E-2's
// drift contract still applies: if ANY installed skill differs from the
// embedded one and --force is not set, the whole run aborts before any
// write. This keeps the install set atomic from the user's POV — either
// the new namespace is fully landed, or the old state is preserved.
func doInstall(g globalContext, opts installSkillOptions, root string, skills []skillEntry) int {
	preps := make([]installPrep, 0, len(skills))
	for _, e := range skills {
		path := claudeSkillPathFor(root, e)
		hash := skillVersionHash(e.Embedded)
		stamped := stampVersion(e.Embedded, hash)

		installedVersion, exists, err := readInstalledFrontmatter(path)
		if err != nil {
			logInstallSkillError(g, opts.Agent, "READ_INSTALLED", path)
			return reportError(g.w, err)
		}
		instContent := ""
		if exists {
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				logInstallSkillError(g, opts.Agent, "READ_INSTALLED", path)
				return reportError(g.w, fmt.Errorf("install-skill: read installed SKILL.md %q: %w", path, rerr))
			}
			instContent = skillVersionHash(b)
		}
		preps = append(preps, installPrep{
			entry:       e,
			path:        path,
			hash:        hash,
			stamped:     stamped,
			installed:   exists,
			instVer:     installedVersion,
			instContent: instContent,
		})
	}

	// First pass: drift detection across all skills. Any drift with no
	// --force aborts the run with a single combined error envelope so
	// the user sees the full picture. Special case: an existing flat
	// SKILL.md from a pre-namespace `bv` release has no bv-skill-version
	// metadata (instVer == "") and no peer prove-it/review SKILL.md;
	// surface the migration call-out so the user knows what --force
	// will actually do.
	if !opts.Force {
		flatLegacyMigration := isFlatLegacyMigration(preps)
		for _, p := range preps {
			if !p.installed {
				continue
			}
			if p.instVer == p.hash && p.instContent == p.hash {
				continue
			}
			shown := p.instVer
			if shown == "" {
				shown = "<unknown>"
			}
			if p.instContent != "" && p.instContent != p.instVer {
				shown = fmt.Sprintf("%s, current_content=%s", shown, p.instContent)
			}
			var msg string
			if flatLegacyMigration {
				msg = fmt.Sprintf(
					"/butverify skill at %s is a pre-namespace flat install (installed=%s, embedded=%s). Re-run with --force to migrate: the current SKILL.md will be backed up to %s.bak and replaced by a deprecated alias, and the new /butverify:prove-it and /butverify:review skills will be installed under the butverify/ namespace. Use --uninstall to remove instead.",
					p.path, shown, p.hash, p.path,
				)
			} else {
				msg = fmt.Sprintf(
					"/butverify skill at %s has a different version than the embedded one (installed=%s, embedded=%s); re-run with --force to overwrite (a .bak will be written) or --uninstall to remove",
					p.path, shown, p.hash,
				)
			}
			g.w.Error(toErrorEnvelope(errors.New(msg)))
			logInstallSkillError(g, opts.Agent, "DRIFT", p.path)
			return 1
		}
	}

	// Second pass: write or short-circuit per skill.
	outcomes := make([]installOneOutcome, 0, len(preps))
	for _, p := range preps {
		switch {
		case p.installed && !opts.Force && p.instVer == p.hash && p.instContent == p.hash:
			outcomes = append(outcomes, installOneOutcome{Path: p.path, Version: p.hash, Status: "already_current"})

		case p.installed && opts.Force:
			// BVS-E-3: backup the previous file (always overwrite any
			// prior .bak), then atomic-write the new content.
			bakPath := p.path + ".bak"
			oldBytes, rerr := os.ReadFile(p.path)
			if rerr != nil {
				logInstallSkillError(g, opts.Agent, "READ_FOR_BACKUP", p.path)
				return reportError(g.w, fmt.Errorf("install-skill: read prior SKILL.md for .bak: %w", rerr))
			}
			if err := atomicWrite(bakPath, oldBytes); err != nil {
				logInstallSkillError(g, opts.Agent, "WRITE_BAK", bakPath)
				return reportError(g.w, fmt.Errorf("install-skill: write .bak: %w", err))
			}
			if err := atomicWrite(p.path, p.stamped); err != nil {
				logInstallSkillError(g, opts.Agent, "WRITE", p.path)
				return reportError(g.w, fmt.Errorf("install-skill: write SKILL.md: %w", err))
			}
			outcomes = append(outcomes, installOneOutcome{Path: p.path, Version: p.hash, Status: "force_overwrote"})

		default:
			// Fresh install (or --force on clean dir).
			if err := atomicWrite(p.path, p.stamped); err != nil {
				logInstallSkillError(g, opts.Agent, "WRITE", p.path)
				return reportError(g.w, fmt.Errorf("install-skill: write SKILL.md: %w", err))
			}
			outcomes = append(outcomes, installOneOutcome{Path: p.path, Version: p.hash, Status: "installed"})
		}
	}

	// Optional hooks installation (EV2-E-9). Hooks are advisory and a
	// failure to write them must not fail the install — log and move on.
	hookOutcome := maybeInstallHooks(g, opts, root)

	// Pick a single primary path for log emission (the legacy alias
	// path keeps the existing log shape stable for downstream consumers).
	primaryPath := claudeSkillPath(root)
	primaryHash := outcomes[0].Version
	primaryStatus := outcomes[0].Status
	for _, o := range outcomes {
		if o.Path == primaryPath {
			primaryHash = o.Version
			primaryStatus = o.Status
			break
		}
	}
	emitInstallSuccess(g, opts, primaryPath, primaryHash, primaryStatus, outcomes, hookOutcome)
	logInstallSkill(g, "completed", opts.Agent, primaryPath)
	return 0
}

// emitInstallSuccess prints the post-install "next steps" block in
// human mode and a structured payload in JSON mode. The `outcomes`
// slice carries one entry per skill (alias + namespaced) so a JSON
// consumer sees per-skill status.
func emitInstallSuccess(g globalContext, opts installSkillOptions, skillPath, version, status string, outcomes []installOneOutcome, hook hookInstallOutcome) {
	if g.w.IsJSON() {
		_ = g.w.JSON(struct {
			OK      bool                `json:"ok"`
			Agent   string              `json:"agent"`
			Path    string              `json:"path"`
			Version string              `json:"version"`
			Status  string              `json:"status"`
			Skills  []installOneOutcome `json:"skills,omitempty"`
			Hook    hookInstallOutcome  `json:"hook"`
		}{true, opts.Agent, skillPath, version, status, outcomes, hook})
		return
	}
	st := g.w.StdoutStyler()
	g.w.Success("Installed /butverify skills for %s.", opts.Agent)
	for _, o := range outcomes {
		g.w.Human("  %s  (%s, version %s)", o.Path, st.Dim(o.Status), st.Dim(o.Version))
	}
	g.w.KV("Scope", scopeLabel(opts))
	if hook.Status != "" && hook.Status != "skipped" {
		g.w.KVf("Hooks", "%s (%s)", hook.Status, hook.SettingsPath)
	} else if hook.Status == "skipped" && hook.Reason != "" {
		g.w.KVf("Hooks", "skipped (%s)", hook.Reason)
	}
	g.w.Section("Next steps")
	g.w.Human("  1. Open Claude Code in your project.")
	g.w.Human("  2. After delivering a piece of work, run %s in chat.", st.Cyan("/butverify:prove-it"))
	g.w.Human("  3. The agent captures proof and publishes it via %s.", st.Dim("`bv evidence --push --mode remote`"))
	g.w.Human("  4. To check for human feedback later, run %s.", st.Cyan("/butverify:review"))
	g.w.Human("")
	if opts.Project {
		g.w.Note("(To install at user level instead, omit --project; files then live under $HOME/.claude/skills/butverify/.)")
	} else {
		g.w.Note("(To install only into a specific project instead, use 'bv install-skill %s --project'.)", opts.Agent)
	}
	g.w.Hint("Docs: https://butverify.dev/docs/reference/install-skill/")
}

// scopeLabel returns a short human-readable scope name for the
// post-install summary. Kept tiny so the next-steps block reads at a
// glance.
func scopeLabel(opts installSkillOptions) string {
	if opts.Project {
		return "project (./.claude/...)"
	}
	return "user ($HOME/.claude/...)"
}

// printInstallSkillUsageError writes the help block on the error path
// (no agent supplied). Stderr in human mode + a short structured
// error envelope in JSON mode so a piped consumer can branch on the
// BAD_REQUEST code without parsing the long usage text.
func printInstallSkillUsageError(g globalContext) {
	if g.w.IsJSON() {
		g.w.Error(toErrorEnvelope(errors.New("missing required <agent> argument; supported agents: " + strings.Join(supportedAgents, ", ") + " (run `bv install-skill --help` for the full flag list)")))
		return
	}
	// Status writes to stderr (human mode), which is the right sink
	// for a usage error printed alongside an exit-2.
	g.w.Status("%s", cliref.CommandHelp("install-skill"))
}

// atomicWrite writes data to dstPath via a same-directory tmp sibling
// + os.Rename. Same-dir tmp guarantees the rename is intra-filesystem
// (BVS-S-1 cross-device-safe). File mode 0644; created parent dir
// mode 0755.
//
// AC-19 sibling-tmp guarantee: tmpPath is constructed via
// `filepath.Join(dir, tmpName)` where `dir = filepath.Dir(dstPath)`,
// so the tmp lives in the SAME directory as the final destination by
// construction. A fault-injection test that paused mid-write to assert
// no /tmp leak is filed as a P3 follow-up — the lexical
// guarantee here makes the test redundant for a code-inspection level
// of confidence.
func atomicWrite(dstPath string, data []byte) error {
	return atomicWriteMode(dstPath, data, 0o644)
}

// atomicWriteMode writes data to dstPath with the supplied mode. Hook
// settings.json may carry an existing 0600 mode that we must preserve
// to avoid widening file permissions on a user-tightened file.
func atomicWriteMode(dstPath string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", dir, err)
	}
	tmpName, err := tmpSiblingName(filepath.Base(dstPath))
	if err != nil {
		return fmt.Errorf("tmp name: %w", err)
	}
	tmpPath := filepath.Join(dir, tmpName)
	// O_EXCL so two racing renames don't both write the same tmp.
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("create tmp %q: %w", tmpPath, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write tmp %q: %w", tmpPath, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close tmp %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename %q -> %q: %w", tmpPath, dstPath, err)
	}
	return nil
}

// tmpSiblingName builds a unique sibling-tmp basename for dst. The
// fixed `.tmp.` infix is part of BVS-E-4's uninstall fixed-set glob:
// `<dir>/<base>.tmp.*` is removed so a crashed install never leaves
// orphan tmps behind.
func tmpSiblingName(base string) (string, error) {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return base + ".tmp." + hex.EncodeToString(buf[:]), nil
}

// doUninstall implements BVS-E-4 across the multi-skill namespace. The
// per-skill fixed uninstall set is:
//
//	<dir>/SKILL.md
//	<dir>/SKILL.md.bak
//	<dir>/SKILL.md.tmp.*    (any orphan tmp from a crashed install)
//
// After removing the fixed set for each installed skill, the CLI also
// rmdirs each skill subdir if empty AND the namespace dir if empty.
// The CLI NEVER calls RemoveAll; NEVER touches the parent
// (~/.claude/skills/) or any ancestor (BVS-E-4 + BVS-N-3); preserves any
// sibling files the user may have planted. Hooks (if installed) are
// removed from settings.json by sentinel.
func doUninstall(g globalContext, opts installSkillOptions, root string, skills []skillEntry) int {
	primaryPath := claudeSkillPath(root)
	removed := []string{}
	anyExisted := false

	for _, e := range skills {
		path := claudeSkillPathFor(root, e)
		dir := filepath.Dir(path)
		base := filepath.Base(path) // "SKILL.md"

		fixed := []string{
			path,
			path + ".bak",
		}
		tmpGlob := filepath.Join(dir, base+".tmp.*")
		tmps, err := filepath.Glob(tmpGlob)
		if err != nil {
			logInstallSkillError(g, opts.Agent, "GLOB", tmpGlob)
			return reportError(g.w, fmt.Errorf("install-skill: glob tmp siblings: %w", err))
		}

		for _, p := range append(fixed, tmps...) {
			if _, statErr := os.Lstat(p); statErr != nil {
				if os.IsNotExist(statErr) {
					continue
				}
				logInstallSkillError(g, opts.Agent, "STAT_FOR_REMOVE", p)
				return reportError(g.w, fmt.Errorf("install-skill: stat %q: %w", p, statErr))
			}
			anyExisted = true
			if err := os.Remove(p); err != nil {
				logInstallSkillError(g, opts.Agent, "REMOVE", p)
				return reportError(g.w, fmt.Errorf("install-skill: remove %q: %w", p, err))
			}
			removed = append(removed, p)
		}
	}

	// rmdir each empty skill subdir, then the namespace dir if empty.
	// Iterate from deepest to shallowest so children get a chance to
	// disappear before their parent is checked.
	dirsToTry := []string{}
	for _, e := range skills {
		if len(e.LeafSegments) == 0 {
			continue
		}
		dirsToTry = append(dirsToTry, filepath.Dir(claudeSkillPathFor(root, e)))
	}
	dirsToTry = append(dirsToTry, claudeNamespaceDir(root))

	dirRemoved := false
	for _, d := range dirsToTry {
		if entries, err := os.ReadDir(d); err == nil {
			if len(entries) == 0 {
				if rerr := os.Remove(d); rerr == nil {
					if d == claudeNamespaceDir(root) {
						dirRemoved = true
					}
				}
			}
		} else if !os.IsNotExist(err) {
			logInstallSkillError(g, opts.Agent, "READDIR", d)
			return reportError(g.w, fmt.Errorf("install-skill: read dir %q: %w", d, err))
		}
	}

	hookRemoved, hookErr := uninstallHooks(root)
	hookErrMsg := ""
	if hookErr != nil {
		// Hooks removal failure is a partial-uninstall: skill files are
		// gone but the auto-executing SessionStart/Stop hooks remain in
		// settings.json. Surface the failure and exit non-zero so a
		// caller (or human) sees that the uninstall is incomplete.
		hookErrMsg = hookErr.Error()
		logInstallSkillError(g, opts.Agent, "HOOK_REMOVE", settingsPath(root))
	}

	logInstallSkill(g, "uninstalled", opts.Agent, primaryPath)

	status := "not_installed"
	if anyExisted || hookRemoved {
		status = "uninstalled"
	}
	if hookErr != nil {
		status = "partial"
	}

	if g.w.IsJSON() {
		_ = g.w.JSON(struct {
			OK          bool     `json:"ok"`
			Agent       string   `json:"agent"`
			Removed     []string `json:"removed"`
			DirRemoved  bool     `json:"dir_removed"`
			HookRemoved bool     `json:"hook_removed"`
			HookError   string   `json:"hook_error,omitempty"`
			Status      string   `json:"status"`
		}{
			OK:          hookErr == nil,
			Agent:       opts.Agent,
			Removed:     removed,
			DirRemoved:  dirRemoved,
			HookRemoved: hookRemoved,
			HookError:   hookErrMsg,
			Status:      status,
		})
		if hookErr != nil {
			return 1
		}
		return 0
	}
	if !anyExisted && !hookRemoved && hookErr == nil {
		g.w.Note("/butverify skills are not installed under %s — nothing to remove.", claudeNamespaceDir(root))
		return 0
	}
	st := g.w.StdoutStyler()
	g.w.Success("Uninstalled /butverify skills for %s.", opts.Agent)
	for _, r := range removed {
		g.w.Human("  %s %s", st.Dim("removed:"), r)
	}
	if dirRemoved {
		g.w.Human("  %s %s", st.Dim("removed dir:"), claudeNamespaceDir(root))
	}
	if hookRemoved {
		g.w.Human("  %s %s", st.Dim("removed hooks from"), settingsPath(root))
	}
	if hookErr != nil {
		g.w.Error(toErrorEnvelope(fmt.Errorf(
			"install-skill: failed to remove hooks from %s: %w; SessionStart/Stop bv hooks may still be installed — please review manually",
			settingsPath(root), hookErr,
		)))
		return 1
	}
	return 0
}

// logInstallSkill emits a structured log line per spec §9.4. We use
// the writer's Status sink (stderr in human mode; suppressed in JSON
// mode for now — JSON consumers parse the structured stdout payload).
// Format follows the dotted event-name convention used in the spec:
// install-skill.<event>{...}.
//
// Spec §9.4 lists `install-skill.uninstalled{agent}` only; we also
// emit a `path` field for parity with `completed` and `error` since
// it aids debugging (the same path is already in the structured JSON
// payload on stdout). Spec is treated as a strict-subset contract:
// the implementation may add fields, but the listed fields must be
// present. No spec edit needed.
func logInstallSkill(g globalContext, event, agent, path string) {
	if path == "" {
		g.w.Status("install-skill.%s{agent=%s}", event, agent)
		return
	}
	g.w.Status("install-skill.%s{agent=%s, path=%s}", event, agent, path)
}

// logInstallSkillError emits an install-skill.error structured log
// line. Code is a short uppercase token; path may be empty when the
// error is pre-filesystem (e.g. UNSUPPORTED_AGENT).
func logInstallSkillError(g globalContext, agent, code, path string) {
	if path == "" {
		g.w.Status("install-skill.error{agent=%s, code=%s}", agent, code)
		return
	}
	g.w.Status("install-skill.error{agent=%s, code=%s, path=%s}", agent, code, path)
}
