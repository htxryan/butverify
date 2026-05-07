// Hook installation for `bv install-skill claude` (EV2-E-9a/b/c).
//
// Two hooks are written into the per-agent settings.json (Claude Code
// `<root>/.claude/settings.json`):
//
//   - SessionStart — fires when a new agent session opens. Calls
//     `bv review list --unacknowledged --format=ids`. If the call
//     returns IDs, the hook prints a single advisory line so the agent
//     and the human both see pending feedback at session start.
//
//   - Stop — fires when the agent session ends. Same call, same line.
//     Acts as a reminder before the terminal prompt returns.
//
// Both hooks are wrapped in a small POSIX shell guard so any failure
// (CLI missing, network error, auth expiry) exits 0 — review
// notifications are advisory and MUST NEVER block a session
// (EV2-U-13 + EV2-N-5: payload carries IDs only, no comment text).
//
// Idempotency: hooks are tagged with the sentinel `BV_HOOK_SENTINEL` so
// repeated installs replace the previous entries instead of appending
// duplicates, and uninstall finds and removes them by sentinel even if
// the user has hand-edited the surrounding settings.json structure.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// BV_HOOK_SENTINEL is embedded inside every hook command we write so a
// future install or uninstall can recognize OUR entries even if the user
// hand-edits surrounding hooks (e.g. compound-agent's own hooks). The
// string is intentionally distinctive — no other hook should embed
// "bv-skill-hook=v1".
const bvHookSentinel = "bv-skill-hook=v1"

// settingsPath returns the absolute path to the Claude Code settings
// file that hooks live in: `<root>/.claude/settings.json`.
func settingsPath(root string) string {
	return filepath.Join(root, ".claude", "settings.json")
}

// hookInstallOutcome is the JSON-friendly summary returned by
// maybeInstallHooks. Status values:
//
//   - "installed"      — hooks added (or replaced existing bv hooks).
//   - "skipped"        — TTY consent declined, --no-hook supplied, or
//     non-TTY without --enable-hook.
//   - "error"          — best-effort write failed; install still
//     succeeded since hooks are advisory.
type hookInstallOutcome struct {
	Status       string `json:"status"`
	Reason       string `json:"reason,omitempty"`
	SettingsPath string `json:"settings_path,omitempty"`
}

// maybeInstallHooks decides whether to install the SessionStart and
// Stop hooks based on opts (EnableHook / NoHook / TTY) and writes them
// into <root>/.claude/settings.json on consent. Returns a structured
// outcome; install never fails the install run.
func maybeInstallHooks(g globalContext, opts installSkillOptions, root string) hookInstallOutcome {
	switch {
	case opts.NoHook:
		return hookInstallOutcome{Status: "skipped", Reason: "--no-hook"}
	case opts.EnableHook:
		// fall through to install
	default:
		// Decide via TTY consent. Both stderr (where the prompt prints)
		// and stdin (where we read the answer) must be TTYs. If stdin
		// is piped while stderr is interactive, prompting would silently
		// consume pipeline data — skip and require --enable-hook.
		if !g.w.IsHumanTTY() || !isStdinTTY() {
			return hookInstallOutcome{Status: "skipped", Reason: "non-tty (pass --enable-hook to install hooks non-interactively)"}
		}
		ok, err := promptHookConsent(g)
		if err != nil {
			return hookInstallOutcome{Status: "skipped", Reason: fmt.Sprintf("prompt failed: %v", err)}
		}
		if !ok {
			return hookInstallOutcome{Status: "skipped", Reason: "user declined"}
		}
	}

	if err := installHooks(root); err != nil {
		logInstallSkillError(g, opts.Agent, "HOOK_WRITE", settingsPath(root))
		return hookInstallOutcome{Status: "error", Reason: err.Error(), SettingsPath: settingsPath(root)}
	}
	return hookInstallOutcome{Status: "installed", SettingsPath: settingsPath(root)}
}

// promptHookConsent prints the y/N prompt and reads one line of user
// input from stdin. Returns true on a "y" / "yes" answer (case-
// insensitive); any other answer (including empty / EOF) is "no".
func promptHookConsent(g globalContext) (bool, error) {
	g.w.Human("")
	g.w.Human("Optional: install Claude Code hooks that surface unacknowledged butverify reviews")
	g.w.Human("at session start and end. The hook command is:")
	g.w.Human("    bv review list --unacknowledged --format=ids")
	g.w.Human("It catches all errors and never blocks a session.")
	fmt.Fprint(os.Stderr, "Enable review notifications at session start and end? (y/N): ")
	var line string
	_, err := fmt.Fscanln(os.Stdin, &line)
	if err != nil {
		// Treat EOF / unexpected newline as a soft "no" rather than a
		// hard error, so a Ctrl-D mid-prompt produces a deterministic
		// skip rather than aborting the install.
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		// "unexpected newline" surfaces when the user just hits Enter.
		// Fscanln's error type is unexported, so match by string.
		if strings.Contains(err.Error(), "unexpected newline") {
			return false, nil
		}
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

// hookEntry is one entry in the Claude Code settings.json hooks array
// for a given event. The shape mirrors what Claude Code reads at
// session start / stop time — a list of `{matcher?, hooks: [{type,
// command}]}` blocks.
type hookEntry struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	// Timeout is in seconds; we set 2s for the SessionStart and Stop
	// hooks so a slow control-plane never holds up a session.
	Timeout int `json:"timeout,omitempty"`
}

// installHooks reads <root>/.claude/settings.json (creating an empty
// object if absent), removes any existing bv-sentinel entries from the
// SessionStart and Stop arrays, appends fresh entries, and writes the
// merged settings back atomically. The merge is robust to a settings.json
// that already contains other tools' hooks (e.g. compound-agent).
//
// The SessionStart entry uses the "pending" wording (EV2-E-9b) and the
// Stop entry uses "still pending" (EV2-E-9c) — at session end the
// human is meant to read this as a reminder that they have not yet
// dealt with the queued review.
func installHooks(root string) error {
	sp := settingsPath(root)
	settings, err := readSettings(sp)
	if err != nil {
		return err
	}

	hooksRaw, ok := settings["hooks"]
	if ok {
		if _, isMap := hooksRaw.(map[string]any); !isMap {
			return fmt.Errorf("install-skill: refusing to install hooks; settings.hooks is not a JSON object (got %T) — please review %s manually", hooksRaw, sp)
		}
	}
	hooks, _ := getHooksMap(settings)
	// Defense-in-depth: a wrongly-typed per-event value (e.g. somebody
	// hand-edited hooks.SessionStart to be a string or object) must not
	// be silently overwritten — surface a clear error instead.
	for _, evt := range []string{"SessionStart", "Stop"} {
		v, present := hooks[evt]
		if !present {
			continue
		}
		if _, isArr := v.([]any); !isArr {
			return fmt.Errorf("install-skill: refusing to install hooks; settings.hooks.%s is not a JSON array (got %T) — please review %s manually", evt, v, sp)
		}
	}
	startCmd, err := bvHookShellCommand("pending")
	if err != nil {
		return err
	}
	stopCmd, err := bvHookShellCommand("still pending")
	if err != nil {
		return err
	}
	startStripped, _ := stripSentinelEntries(getEventArray(hooks, "SessionStart"))
	stopStripped, _ := stripSentinelEntries(getEventArray(hooks, "Stop"))
	hooks["SessionStart"] = appendHook(startStripped, buildHookEntryFromCommand(startCmd))
	hooks["Stop"] = appendHook(stopStripped, buildHookEntryFromCommand(stopCmd))
	settings["hooks"] = hooks

	return writeSettings(sp, settings)
}

// uninstallHooks reads settings.json, removes any sentinel-tagged
// entries from the SessionStart and Stop arrays, and writes back. If
// the file did not exist or contained no bv hooks, returns (false, nil).
// Returns (true, nil) when at least one entry was removed.
func uninstallHooks(root string) (bool, error) {
	sp := settingsPath(root)
	if _, err := os.Stat(sp); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	settings, err := readSettings(sp)
	if err != nil {
		return false, err
	}
	hooks, ok := getHooksMap(settings)
	if !ok {
		return false, nil
	}
	removedAny := false
	for _, event := range []string{"SessionStart", "Stop"} {
		arr := getEventArray(hooks, event)
		stripped, didRemove := stripSentinelEntries(arr)
		if didRemove {
			removedAny = true
		}
		if len(stripped) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = stripped
		}
	}
	if !removedAny {
		return false, nil
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	} else {
		settings["hooks"] = hooks
	}
	if err := writeSettings(sp, settings); err != nil {
		return false, err
	}
	return true, nil
}

// readSettings reads settings.json into a generic map. Returns an empty
// map when the file does not exist (so a first-install can write a
// fresh file).
func readSettings(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("install-skill: read settings %q: %w", path, err)
	}
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("install-skill: parse settings %q: %w", path, err)
	}
	return out, nil
}

// writeSettings writes the merged settings back via the atomic-rename
// pattern shared with skill writes. Preserves the existing file's mode
// when present (so a user-tightened 0600 settings.json carrying tokens
// is not silently widened to 0644 by our merge); falls back to 0644
// for a fresh write where no prior file exists.
func writeSettings(path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("install-skill: mkdir settings parent: %w", err)
	}
	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("install-skill: marshal settings: %w", err)
	}
	encoded = append(encoded, '\n')

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := atomicWriteMode(path, encoded, mode); err != nil {
		return fmt.Errorf("install-skill: write settings %q: %w", path, err)
	}
	return nil
}

// getHooksMap returns settings["hooks"] as a map[string]any, or a fresh
// map if the field is absent or wrongly typed. Wrong-typed input
// (somebody set hooks: []) is treated as absent so we don't clobber
// the user's data destructively — the second return value flags that.
func getHooksMap(settings map[string]any) (map[string]any, bool) {
	v, ok := settings["hooks"]
	if !ok {
		return map[string]any{}, false
	}
	m, ok := v.(map[string]any)
	if !ok {
		return map[string]any{}, false
	}
	return m, true
}

// getEventArray returns hooks[event] as a slice of generic entries, or
// an empty slice if absent. Wrong-typed entries pass through; the
// merge logic preserves them.
func getEventArray(hooks map[string]any, event string) []any {
	v, ok := hooks[event]
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	return arr
}

// stripSentinelEntries returns a copy of `arr` with the bv sentinel
// commands stripped out, plus a flag indicating whether any sentinel
// command was actually removed (across either dropped entries or
// rewritten entries). The flag is necessary because uninstall must
// notice the case where an entry kept its top-level slot but had a
// sentinel-tagged sibling command stripped — comparing only top-level
// length would miss those mixed entries and skip the write, leaving
// the bv command behind.
//
// The original implementation dropped the whole entry if any nested
// `hooks[*].command` carried the sentinel, which would also lose any
// sibling commands the user added to the same entry (matchers like "*"
// can group multiple commands). Now it removes only the sentinel-tagged
// commands; an entry whose `hooks[]` becomes empty is dropped (since an
// entry with no commands is meaningless), but entries with surviving
// sibling commands are preserved.
func stripSentinelEntries(arr []any) ([]any, bool) {
	out := make([]any, 0, len(arr))
	removed := false
	for _, raw := range arr {
		filtered, didRemove, ok := filterSentinelCommands(raw)
		if !ok {
			// Unrecognized shape (not a map or no hooks array we can
			// reason about) — preserve verbatim.
			out = append(out, raw)
			continue
		}
		if didRemove {
			removed = true
		}
		if filtered == nil {
			// All commands in this entry were sentinel-tagged.
			continue
		}
		out = append(out, filtered)
	}
	return out, removed
}

// filterSentinelCommands inspects one hooks-array entry and returns
// (filteredEntry, removedAny, recognized).
//
//   - recognized=false: the entry shape is unrecognized; caller preserves
//     it verbatim. removedAny is meaningless in this case.
//   - recognized=true, removedAny=false: no sentinel commands present.
//     filteredEntry is `raw` unchanged.
//   - recognized=true, removedAny=true, filteredEntry==nil: all commands
//     in the entry were sentinel-tagged; caller drops the entry.
//   - recognized=true, removedAny=true, filteredEntry!=nil: at least one
//     sentinel command was stripped while sibling commands survived;
//     caller keeps the rewritten entry.
func filterSentinelCommands(raw any) (any, bool, bool) {
	m, ok := raw.(map[string]any)
	if !ok {
		return raw, false, false
	}
	hooks, ok := m["hooks"].([]any)
	if !ok {
		return raw, false, false
	}
	kept := make([]any, 0, len(hooks))
	stripped := false
	for _, h := range hooks {
		hm, isMap := h.(map[string]any)
		if !isMap {
			kept = append(kept, h)
			continue
		}
		if cmd, _ := hm["command"].(string); strings.Contains(cmd, bvHookSentinel) {
			stripped = true
			continue
		}
		kept = append(kept, h)
	}
	if !stripped {
		// Nothing changed; return the original entry untouched.
		return raw, false, true
	}
	if len(kept) == 0 {
		return nil, true, true
	}
	// Rebuild a shallow copy of the entry with the filtered hooks list
	// so we don't mutate the caller's input map.
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	out["hooks"] = kept
	return out, true, true
}

// entryHasSentinel reports whether the entry contains any sentinel-tagged
// commands. Retained for tests that assert on the bv-owned subset.
func entryHasSentinel(raw any) bool {
	m, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	hooks, ok := m["hooks"].([]any)
	if !ok {
		return false
	}
	for _, h := range hooks {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		cmd, _ := hm["command"].(string)
		if strings.Contains(cmd, bvHookSentinel) {
			return true
		}
	}
	return false
}

// appendHook appends our hookEntry to a generic entries slice as a map
// (so it round-trips through json.Marshal cleanly with the user's
// other entries).
func appendHook(arr []any, entry hookEntry) []any {
	encoded, err := json.Marshal(entry)
	if err != nil {
		// json.Marshal of our well-typed hookEntry cannot fail in
		// practice; treat as a defense-in-depth no-op.
		return arr
	}
	var generic any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		return arr
	}
	return append(arr, generic)
}

// buildHookEntryFromCommand wraps a precomputed shell command in the
// JSON shape Claude Code expects for one hooks-array entry.
func buildHookEntryFromCommand(cmd string) hookEntry {
	return hookEntry{
		Matcher: "",
		Hooks: []hookCommand{
			{Type: "command", Command: cmd, Timeout: 2},
		},
	}
}

// resolveBVExecutable returns the absolute, symlink-resolved path of the
// running `bv` binary. The hook command embeds this path verbatim so a
// later PATH change (e.g. an attacker dropping `~/.local/bin/bv` ahead of
// the real binary) cannot redirect the auto-executing hook to an
// unrelated binary. Override-friendly for tests via bvExecutableFn.
var bvExecutableFn = realBVExecutable

func realBVExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		// Fall back to the unresolved absolute path; better than aborting
		// install and arguably equivalent for the threat model we care
		// about (PATH-resolution hijack — not symlink swap on a binary
		// the attacker already controls).
		return exe, nil
	}
	return resolved, nil
}

// shellSingleQuote wraps `s` in POSIX single quotes so the resulting
// token is safe to splice into a /bin/sh command. Single quotes inside
// `s` close the quoted span and are emitted as `'\”` so the original
// character survives intact.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// bvHookShellCommand returns the literal shell snippet stored in
// settings.json for the given phase ("pending" for SessionStart,
// "still pending" for Stop). The resolved absolute path of `bv` is
// embedded at install time so the auto-executing hook cannot be
// hijacked by a later PATH change. If the file at that path is missing
// or non-executable at hook time the snippet exits 0 silently — hooks
// are advisory and MUST NEVER block a session (EV2-N-5).
func bvHookShellCommand(phase string) (string, error) {
	exe, err := bvExecutableFn()
	if err != nil {
		return "", fmt.Errorf("install-skill: resolve bv executable for hook command: %w", err)
	}
	q := shellSingleQuote(exe)
	return `[ -x ` + q + ` ] || exit 0; ` +
		`out=$(` + q + ` review list --unacknowledged --format=ids 2>/dev/null) || exit 0; ` +
		`[ -z "$out" ] && exit 0; ` +
		`n=$(printf '%s\n' "$out" | wc -l | tr -d ' '); ` +
		`ids=$(printf '%s\n' "$out" | tr '\n' ' ' | sed 's/ $//'); ` +
		`printf '[butverify] %s review(s) ` + phase + `: %s\n' "$n" "$ids"; ` +
		`exit 0 # ` + bvHookSentinel, nil
}
