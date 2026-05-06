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
		// Decide via TTY consent. Non-TTY: skip silently.
		if !g.w.IsHumanTTY() {
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
	Matcher string         `json:"matcher,omitempty"`
	Hooks   []hookCommand  `json:"hooks"`
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
func installHooks(root string) error {
	sp := settingsPath(root)
	settings, err := readSettings(sp)
	if err != nil {
		return err
	}

	hooks, _ := getHooksMap(settings)
	for _, event := range []string{"SessionStart", "Stop"} {
		hooks[event] = appendHook(stripSentinelEntries(getEventArray(hooks, event)), buildHookEntry())
	}
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
		stripped := stripSentinelEntries(arr)
		if len(stripped) != len(arr) {
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
// pattern shared with skill writes. 0644 mode, parent dir 0755.
func writeSettings(path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("install-skill: mkdir settings parent: %w", err)
	}
	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("install-skill: marshal settings: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := atomicWrite(path, encoded); err != nil {
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

// stripSentinelEntries returns a copy of `arr` with any entry whose
// nested `hooks[*].command` field contains the bv sentinel removed.
// Any entry we don't recognize is preserved verbatim.
func stripSentinelEntries(arr []any) []any {
	out := make([]any, 0, len(arr))
	for _, raw := range arr {
		if entryHasSentinel(raw) {
			continue
		}
		out = append(out, raw)
	}
	return out
}

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

// buildHookEntry constructs the SessionStart / Stop hook payload. The
// shell command uses `bv` from PATH (we do not bake an absolute path
// because the bv binary may be moved by package managers between
// install and session-start). The output line format matches EV2-E-8.
//
// The sentinel string is embedded as a comment that the shell ignores
// (`#bv-skill-hook=v1`) so our installer can recognize and replace
// these entries on re-install / uninstall.
func buildHookEntry() hookEntry {
	cmd := bvHookShellCommand()
	return hookEntry{
		Matcher: "",
		Hooks: []hookCommand{
			{Type: "command", Command: cmd, Timeout: 2},
		},
	}
}

// bvHookShellCommand returns the literal shell snippet stored in
// settings.json for SessionStart and Stop. Kept as a separate function
// so tests can reach it directly without parsing JSON.
func bvHookShellCommand() string {
	// The trailing `# bv-skill-hook=v1` is a shell comment that lets
	// our installer match-and-replace this exact entry. The body uses
	// command substitution + a short awk to format the [butverify] line
	// without depending on tools beyond a POSIX shell, awk, and bv
	// itself. Errors are swallowed so the hook never blocks a session.
	return `out=$(bv review list --unacknowledged --format=ids 2>/dev/null) || exit 0; ` +
		`[ -z "$out" ] && exit 0; ` +
		`n=$(printf '%s\n' "$out" | wc -l | tr -d ' '); ` +
		`ids=$(printf '%s\n' "$out" | tr '\n' ' ' | sed 's/ $//'); ` +
		`printf '[butverify] %s review(s) pending: %s\n' "$n" "$ids"; ` +
		`exit 0 # ` + bvHookSentinel
}
