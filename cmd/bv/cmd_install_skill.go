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

// embeddedSkillBytes carries the canonical /butverify skill markdown
// at compile time. The file is a build-time mirror of
// bv-skills/claude/butverify.md (see top-of-file comment for the
// `..` cross-module-boundary issue and the byte-equality test that
// keeps the two in sync).
//
//go:embed embedded_skills/claude_butverify.md
var embeddedSkillBytes []byte

// versionLinePrefix is the literal prefix of the YAML frontmatter line
// the canonical-hash-domain function (BVS-U-9) replaces, and the
// install-time stamper (stampVersion) updates. The single-source-of-
// truth here means a future schema change to the frontmatter shape
// edits exactly one constant.
const versionLinePrefix = "bv-skill-version:"

// versionSentinel is the literal value the canonical hash domain
// substitutes onto the bv-skill-version line BEFORE hashing. The
// embedded source carries `bv-skill-version: 0000000000ab` (a
// placeholder); the canonical-domain transform rewrites that to
// `bv-skill-version: 000000000000` so the hash is independent of the
// placeholder choice. Length is exactly 12 hex chars to match the
// installed hash width (BVS-U-7).
const versionSentinel = versionLinePrefix + " 000000000000"

// canonicalHashDomain returns the byte form of the markdown that
// BVS-U-9 hashes:
//
//	(a) line endings normalized to LF (\r\n -> \n; lone \r -> \n)
//	(b) the entire `bv-skill-version: ...` line replaced with the
//	    literal sentinel `bv-skill-version: 000000000000`
//	(c) trailing LF enforced (the result always ends with `\n`)
//
// This MUST be deterministic across builds (BVS-SC-13).
func canonicalHashDomain(markdown []byte) []byte {
	// (a) Line-ending normalization. We do this on the raw bytes
	// rather than line-by-line to avoid silently dropping a
	// non-LF-terminated final line.
	normalized := normalizeLineEndings(markdown)

	// (b) Replace the version line. We split on '\n' so we can
	// rewrite the matching line verbatim regardless of where it
	// appears in the frontmatter.
	lines := strings.Split(string(normalized), "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), versionLinePrefix) {
			lines[i] = versionSentinel
			break
		}
	}
	out := strings.Join(lines, "\n")

	// (c) Enforce trailing LF.
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

// stampVersion replaces the bv-skill-version line with `bv-skill-
// version: <hash>`. This happens AFTER hashing (BVS-U-9: the hash is
// computed against the canonical sentinel, then the install-time
// stamp goes onto the file written to disk). Idempotent: stamping a
// file that already has a stamp simply replaces the prior value.
//
// Operates on the line-ending-normalized form so a CRLF input still
// produces a sensible installed file (and the resulting bytes are
// LF-only, matching what the canonical-domain function expects on a
// future re-hash).
func stampVersion(markdown []byte, hash string) []byte {
	normalized := normalizeLineEndings(markdown)
	lines := strings.Split(string(normalized), "\n")
	replaced := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), versionLinePrefix) {
			lines[i] = versionLinePrefix + " " + hash
			replaced = true
			break
		}
	}
	if !replaced {
		// The canonical source always carries the line; if a caller
		// somehow passed in markdown without one, we don't synthesize
		// it — the build-time embed-validation test (BVS-N-2 / T6)
		// guarantees the embedded source has the line.
		return normalized
	}
	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return []byte(out)
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
}

func runInstallSkill(ctx context.Context, g globalContext, args []string) int {
	fs, flags := newCLIFlagSet("install-skill")
	force := flags.Bool("force")
	uninstall := flags.Bool("uninstall")
	project := flags.Bool("project")

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
		Agent:     agent,
		Force:     *force,
		Uninstall: *uninstall,
		Project:   *project,
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
	skillPath := claudeSkillPath(root)

	// BVS-N-1 + BVS-N-3: containment check. We resolve the
	// install-target's PARENT (since the target itself doesn't exist
	// yet at first install) under the install root. Mirrors EV-S-1
	// from the evidence template.
	if _, err := containInstallPath(root, skillPath); err != nil {
		g.w.Error(toErrorEnvelope(err))
		logInstallSkillError(g, opts.Agent, "CONTAINMENT", skillPath)
		return 1
	}

	if opts.Uninstall {
		return doUninstall(g, opts, skillPath)
	}
	return doInstall(g, opts, skillPath)
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

// claudeSkillPath returns the v1 Claude Code install path:
// <root>/.claude/skills/butverify/SKILL.md.
func claudeSkillPath(root string) string {
	return filepath.Join(root, ".claude", "skills", "butverify", "SKILL.md")
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
	// Walk the file line-by-line until we find the version line. We
	// don't pull in a YAML parser at v1 — BVS-N-2 + the build-time
	// embed-validation step keeps the canonical file's frontmatter
	// shape narrow enough that line-prefix matching is safe.
	normalized := normalizeLineEndings(raw)
	for _, line := range strings.Split(string(normalized), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, versionLinePrefix) {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, versionLinePrefix))
			return rest, true, nil
		}
	}
	return "", true, nil
}

// doInstall implements BVS-E-1 (fresh install), BVS-E-2 (drift detect),
// and BVS-E-3 (--force).
func doInstall(g globalContext, opts installSkillOptions, skillPath string) int {
	embeddedHash := skillVersionHash(embeddedSkillBytes)
	stamped := stampVersion(embeddedSkillBytes, embeddedHash)

	// Read existing version (if any) before we touch the filesystem.
	installedVersion, exists, err := readInstalledFrontmatter(skillPath)
	if err != nil {
		logInstallSkillError(g, opts.Agent, "READ_INSTALLED", skillPath)
		return reportError(g.w, err)
	}

	switch {
	case exists && !opts.Force:
		// BVS-E-2: file exists; compare versions.
		if installedVersion == embeddedHash {
			// Already up to date — exit 0, NO write, mtime untouched.
			if g.w.IsJSON() {
				_ = g.w.JSON(struct {
					OK      bool   `json:"ok"`
					Path    string `json:"path"`
					Version string `json:"version"`
					Status  string `json:"status"`
				}{true, skillPath, embeddedHash, "already_current"})
			} else {
				g.w.Human("/butverify skill is already up to date at %s (version %s).", skillPath, embeddedHash)
			}
			logInstallSkill(g, "completed", opts.Agent, skillPath)
			return 0
		}
		// Drift. Refuse non-zero with a message that names both
		// versions. The agent / human picks --force or --uninstall.
		shown := installedVersion
		if shown == "" {
			shown = "<unknown>"
		}
		msg := fmt.Sprintf(
			"/butverify skill at %s has a different version than the embedded one (installed=%s, embedded=%s); re-run with --force to overwrite (a .bak will be written) or --uninstall to remove",
			skillPath, shown, embeddedHash,
		)
		g.w.Error(toErrorEnvelope(errors.New(msg)))
		logInstallSkillError(g, opts.Agent, "DRIFT", skillPath)
		return 1

	case exists && opts.Force:
		// BVS-E-3: backup the previous file (always overwrite any
		// prior .bak — most-recent semantics, one level deep), then
		// atomic-write the new content.
		bakPath := skillPath + ".bak"
		oldBytes, rerr := os.ReadFile(skillPath)
		if rerr != nil {
			logInstallSkillError(g, opts.Agent, "READ_FOR_BACKUP", skillPath)
			return reportError(g.w, fmt.Errorf("install-skill: read prior SKILL.md for .bak: %w", rerr))
		}
		// Write .bak atomically too — a concurrent reader of the .bak
		// should never see a partial file.
		if err := atomicWrite(bakPath, oldBytes); err != nil {
			logInstallSkillError(g, opts.Agent, "WRITE_BAK", bakPath)
			return reportError(g.w, fmt.Errorf("install-skill: write .bak: %w", err))
		}
		if err := atomicWrite(skillPath, stamped); err != nil {
			logInstallSkillError(g, opts.Agent, "WRITE", skillPath)
			return reportError(g.w, fmt.Errorf("install-skill: write SKILL.md: %w", err))
		}
		emitInstallSuccess(g, opts, skillPath, embeddedHash, "force_overwrote")
		logInstallSkill(g, "completed", opts.Agent, skillPath)
		return 0

	case !exists && opts.Force:
		// --force on a clean directory is harmless — same as a fresh
		// install. Document it that way so a CI script that always
		// passes --force still works.
		fallthrough
	default:
		// BVS-E-1 fresh install.
		if err := atomicWrite(skillPath, stamped); err != nil {
			logInstallSkillError(g, opts.Agent, "WRITE", skillPath)
			return reportError(g.w, fmt.Errorf("install-skill: write SKILL.md: %w", err))
		}
		emitInstallSuccess(g, opts, skillPath, embeddedHash, "installed")
		logInstallSkill(g, "completed", opts.Agent, skillPath)
		return 0
	}
}

// emitInstallSuccess prints the post-install "next steps" block in
// human mode and a structured payload in JSON mode.
func emitInstallSuccess(g globalContext, opts installSkillOptions, skillPath, version, status string) {
	if g.w.IsJSON() {
		_ = g.w.JSON(struct {
			OK      bool   `json:"ok"`
			Agent   string `json:"agent"`
			Path    string `json:"path"`
			Version string `json:"version"`
			Status  string `json:"status"`
		}{true, opts.Agent, skillPath, version, status})
		return
	}
	g.w.Human("Installed /butverify skill for %s.", opts.Agent)
	g.w.Human("  Path:    %s", skillPath)
	g.w.Human("  Version: %s", version)
	g.w.Human("  Scope:   %s", scopeLabel(opts))
	g.w.Human("")
	g.w.Human("Next steps:")
	g.w.Human("  1. Open Claude Code in your project.")
	g.w.Human("  2. After delivering a piece of work, run /butverify in chat.")
	g.w.Human("  3. The agent will capture proof and publish it via `bv evidence --push --mode remote`.")
	g.w.Human("")
	// Surface the alternate-scope hint so a user who picked one scope
	// knows the other is one flag away. Mirrors the --help output.
	if opts.Project {
		g.w.Human("(To install at user level instead, omit --project; the file then lives at $HOME/.claude/skills/butverify/SKILL.md.)")
	} else {
		g.w.Human("(To install only into a specific project instead, use 'bv install-skill %s --project'.)", opts.Agent)
	}
	g.w.Human("")
	g.w.Human("Docs: https://butverify.dev/docs/reference/install-skill/")
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
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
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

// doUninstall implements BVS-E-4. The fixed uninstall set is exactly:
//
//	<dir>/SKILL.md
//	<dir>/SKILL.md.bak
//	<dir>/SKILL.md.tmp.*    (any orphan tmp from a crashed install)
//
// After removing the fixed set, the CLI attempts os.Remove on the
// containing dir IFF the dir is empty. The CLI NEVER calls RemoveAll;
// NEVER touches the parent (~/.claude/skills/) or any ancestor (BVS-E-4
// + BVS-N-3); preserves any sibling files the user may have planted.
func doUninstall(g globalContext, opts installSkillOptions, skillPath string) int {
	dir := filepath.Dir(skillPath)
	base := filepath.Base(skillPath) // "SKILL.md"

	// Build the fixed set of literal removals + the tmp glob.
	fixed := []string{
		skillPath,
		skillPath + ".bak",
	}
	tmpGlob := filepath.Join(dir, base+".tmp.*")
	tmps, err := filepath.Glob(tmpGlob)
	if err != nil {
		// filepath.Glob only errors on malformed pattern — our pattern
		// is static, so this is unreachable in practice. Treat as a
		// real error if it ever fires.
		logInstallSkillError(g, opts.Agent, "GLOB", tmpGlob)
		return reportError(g.w, fmt.Errorf("install-skill: glob tmp siblings: %w", err))
	}

	// Remove every entry that exists. We deliberately swallow
	// "does-not-exist" so an idempotent uninstall on a clean dir
	// returns 0.
	removed := []string{}
	anyExisted := false
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

	// rmdir the butverify/ dir IFF empty. NEVER touch parents.
	dirRemoved := false
	if entries, err := os.ReadDir(dir); err == nil {
		if len(entries) == 0 {
			if rerr := os.Remove(dir); rerr == nil {
				dirRemoved = true
			}
			// If os.Remove fails (permissions, race), leave the dir;
			// not worth surfacing as a uninstall failure.
		}
	} else if !os.IsNotExist(err) {
		// Non-existent dir is fine (already cleaned up by hand).
		// Anything else is a real error.
		logInstallSkillError(g, opts.Agent, "READDIR", dir)
		return reportError(g.w, fmt.Errorf("install-skill: read butverify dir %q: %w", dir, err))
	}

	logInstallSkill(g, "uninstalled", opts.Agent, skillPath)

	if g.w.IsJSON() {
		_ = g.w.JSON(struct {
			OK         bool     `json:"ok"`
			Agent      string   `json:"agent"`
			Removed    []string `json:"removed"`
			DirRemoved bool     `json:"dir_removed"`
			Status     string   `json:"status"`
		}{
			OK:         true,
			Agent:      opts.Agent,
			Removed:    removed,
			DirRemoved: dirRemoved,
			Status: func() string {
				if anyExisted {
					return "uninstalled"
				}
				return "not_installed"
			}(),
		})
		return 0
	}
	if !anyExisted {
		g.w.Human("/butverify skill is not installed at %s — nothing to remove.", skillPath)
		return 0
	}
	g.w.Human("Uninstalled /butverify skill for %s.", opts.Agent)
	for _, r := range removed {
		g.w.Human("  removed: %s", r)
	}
	if dirRemoved {
		g.w.Human("  removed dir: %s", dir)
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
