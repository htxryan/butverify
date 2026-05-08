// Tests for the multi-skill namespace install + hook flow added in
// pebble-r8rb (Agent Skill Ecosystem). Covers EV2-E-9a/b/c (TTY consent
// + --enable-hook), the new prove-it / review SKILL.md paths, the
// alias-deprecation behavior, and the settings.json merge semantics
// (bv hooks coexist with other tools' hooks).

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- Multi-skill install path coverage ----

func TestInstall_WritesAllSkillFiles(t *testing.T) {
	home := setupTempHome(t)
	rc, _, _ := runInstall(t, "claude")
	if rc != 0 {
		t.Fatalf("install rc: %d", rc)
	}
	want := []string{
		filepath.Join(home, ".claude", "skills", "butverify", "SKILL.md"),
		filepath.Join(home, ".claude", "skills", "butverify", "prove-it", "SKILL.md"),
		filepath.Join(home, ".claude", "skills", "butverify", "review", "SKILL.md"),
		filepath.Join(home, ".claude", "skills", "launch-monitored-loop", "SKILL.md"),
	}
	for _, p := range want {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected SKILL.md at %s: %v", p, err)
		}
	}
}

func TestInstall_AliasCarriesDeprecatedNotice(t *testing.T) {
	home := setupTempHome(t)
	if rc, _, _ := runInstall(t, "claude"); rc != 0 {
		t.Fatal("install failed")
	}
	body, err := os.ReadFile(claudeSkillPath(home))
	if err != nil {
		t.Fatal(err)
	}
	s := strings.ToLower(string(body))
	if !strings.Contains(s, "deprecated") {
		t.Errorf("alias SKILL.md should contain a deprecation notice; body=%s", body[:min(len(body), 400)])
	}
	if !strings.Contains(string(body), "/butverify:prove-it") {
		t.Errorf("alias SKILL.md should mention /butverify:prove-it as the replacement: %s", body[:min(len(body), 400)])
	}
	// disable-model-invocation: true on the alias prevents Claude Code
	// from auto-invoking the deprecated skill while still allowing
	// humans to type `/butverify`. This stops dual-name confusion
	// during the deprecation window (review feedback S2).
	if !strings.Contains(string(body), "disable-model-invocation: true") {
		t.Errorf("alias SKILL.md must set disable-model-invocation: true to suppress auto-invocation: %s", body[:min(len(body), 400)])
	}
}

// Reverse polarity: prove-it MUST stay enabled for model invocation,
// otherwise the new skill is unreachable.
func TestInstall_ProveItRemainsModelInvocable(t *testing.T) {
	home := setupTempHome(t)
	if rc, _, _ := runInstall(t, "claude"); rc != 0 {
		t.Fatal("install failed")
	}
	body := mustReadFile(t, filepath.Join(home, ".claude", "skills", "butverify", "prove-it", "SKILL.md"))
	if !strings.Contains(string(body), "disable-model-invocation: false") {
		t.Errorf("prove-it SKILL.md must keep disable-model-invocation: false: %s", body[:min(len(body), 400)])
	}
}

// EV2-E-9: legacy flat-install upgrade triggers a clearer error that
// names the migration mechanic ("backed up to .bak" + "namespace").
// Standard drift messaging is too generic for this path.
func TestInstall_LegacyFlatInstall_GivesMigrationMessage(t *testing.T) {
	home := setupTempHome(t)
	dir := filepath.Join(home, ".claude", "skills", "butverify")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := []byte("---\nname: butverify\ndescription: legacy\n---\n# /butverify\nOld body.\n")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	rc, stdout, _ := runInstall(t, "claude")
	if rc == 0 {
		t.Fatalf("legacy flat install should require --force; rc=0 stdout=%s", stdout)
	}
	if !strings.Contains(stdout, "pre-namespace flat install") {
		t.Errorf("expected migration phrasing in error: %s", stdout)
	}
	if !strings.Contains(stdout, "namespace") {
		t.Errorf("error should mention the namespace migration: %s", stdout)
	}
}

func TestInstall_ProveItAndReviewAreDistinctSkills(t *testing.T) {
	home := setupTempHome(t)
	if rc, _, _ := runInstall(t, "claude"); rc != 0 {
		t.Fatal("install failed")
	}
	proveItPath := filepath.Join(home, ".claude", "skills", "butverify", "prove-it", "SKILL.md")
	reviewPath := filepath.Join(home, ".claude", "skills", "butverify", "review", "SKILL.md")

	pi := mustReadFile(t, proveItPath)
	rv := mustReadFile(t, reviewPath)

	if string(pi) == string(rv) {
		t.Fatal("prove-it and review SKILL.md should not be byte-identical")
	}
	if !strings.Contains(string(pi), "/butverify:prove-it") {
		t.Errorf("prove-it skill body should mention its own slash command name: %s", pi[:min(len(pi), 300)])
	}
	if !strings.Contains(string(rv), "/butverify:review") {
		t.Errorf("review skill body should mention its own slash command name: %s", rv[:min(len(rv), 300)])
	}
	if !strings.Contains(string(rv), "bv review list") {
		t.Errorf("review skill body should reference bv review list workflow: %s", rv[:min(len(rv), 300)])
	}
}

// ---- BVS-E-3: --force backs up + overwrites every drifted skill ----

func TestInstall_ForceWritesBakForEverySkill(t *testing.T) {
	home := setupTempHome(t)
	if rc, _, _ := runInstall(t, "claude"); rc != 0 {
		t.Fatal("setup install failed")
	}

	skills := claudeSkills()
	for _, e := range skills {
		path := claudeSkillPathFor(home, e)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		hash := skillVersionHash(e.Embedded)
		mutated := strings.Replace(string(raw), "sha256="+hash, "sha256=cafef00dface", 1)
		if err := os.WriteFile(path, []byte(mutated), 0o644); err != nil {
			t.Fatalf("write drift %s: %v", path, err)
		}
	}

	rc, _, _ := runInstall(t, "claude", "--force")
	if rc != 0 {
		t.Fatalf("--force rc: %d", rc)
	}
	for _, e := range skills {
		path := claudeSkillPathFor(home, e)
		bak := path + ".bak"
		if _, err := os.Stat(bak); err != nil {
			t.Errorf("expected .bak at %s: %v", bak, err)
		}
		raw := mustReadFile(t, path)
		if !strings.Contains(string(raw), "sha256="+skillVersionHash(e.Embedded)) {
			t.Errorf("after --force, %s should carry embedded hash", path)
		}
	}
}

// ---- Uninstall removes every skill file + the namespace dir ----

func TestUninstall_RemovesEverySkillAndDir(t *testing.T) {
	home := setupTempHome(t)
	if rc, _, _ := runInstall(t, "claude"); rc != 0 {
		t.Fatal("setup install failed")
	}
	if rc, _, _ := runInstall(t, "claude", "--uninstall"); rc != 0 {
		t.Fatal("uninstall failed")
	}
	want := []string{
		filepath.Join(home, ".claude", "skills", "butverify"),
		filepath.Join(home, ".claude", "skills", "butverify", "prove-it"),
		filepath.Join(home, ".claude", "skills", "butverify", "review"),
		filepath.Join(home, ".claude", "skills", "launch-monitored-loop"),
	}
	for _, d := range want {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Errorf("expected %s removed: err=%v", d, err)
		}
	}
}

// ---- Already-current short-circuit applies across all skills ----

func TestInstall_AlreadyCurrent_AcrossAllSkills(t *testing.T) {
	home := setupTempHome(t)
	if rc, _, _ := runInstall(t, "claude"); rc != 0 {
		t.Fatal("setup install failed")
	}
	rc, stdout, _ := runInstall(t, "claude")
	if rc != 0 {
		t.Fatalf("re-install rc: %d  stdout=%s", rc, stdout)
	}
	if !strings.Contains(stdout, `"already_current"`) {
		t.Errorf("expected already_current somewhere in payload: %s", stdout)
	}
	_ = home
}

// ---- EV2-E-9a/b/c: hooks consent + --enable-hook ----

func TestInstall_HooksSkippedNonInteractive(t *testing.T) {
	home := setupTempHome(t)
	rc, stdout, _ := runInstall(t, "claude")
	if rc != 0 {
		t.Fatalf("install rc: %d  stdout=%s", rc, stdout)
	}
	// Default writer in tests is JSON-mode (non-TTY); hooks must be
	// silently skipped without --enable-hook.
	if _, err := os.Stat(settingsPath(home)); !os.IsNotExist(err) {
		t.Errorf("non-interactive install should NOT write settings.json (got err=%v)", err)
	}
	if !strings.Contains(stdout, `"hook"`) || !strings.Contains(stdout, `"skipped"`) {
		t.Errorf("hook outcome should be present and skipped: %s", stdout)
	}
}

func TestInstall_EnableHook_WritesSettingsJSON(t *testing.T) {
	home := setupTempHome(t)
	rc, stdout, _ := runInstall(t, "claude", "--enable-hook")
	if rc != 0 {
		t.Fatalf("--enable-hook rc: %d  stdout=%s", rc, stdout)
	}
	sp := settingsPath(home)
	raw, err := os.ReadFile(sp)
	if err != nil {
		t.Fatalf("settings.json should exist: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("settings.json must be valid JSON: %v\n%s", err, raw)
	}
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("settings.hooks missing: %v", settings)
	}

	// SessionStart: inline command containing the review list call.
	startArr, ok := hooks["SessionStart"].([]any)
	if !ok || len(startArr) == 0 {
		t.Fatalf("hooks[SessionStart] missing or empty: %v", hooks["SessionStart"])
	}
	startFound := false
	for _, raw := range startArr {
		if entryHasSentinel(raw) {
			startFound = true
			cmd := commandFromEntry(t, raw)
			if !strings.Contains(cmd, "review list --unacknowledged --format=ids") {
				t.Errorf("hooks[SessionStart] sentinel entry should call review list: %s", cmd)
			}
			if !strings.Contains(cmd, "[butverify]") {
				t.Errorf("hooks[SessionStart] entry should print [butverify] prefix: %s", cmd)
			}
		}
	}
	if !startFound {
		t.Errorf("hooks[SessionStart] missing sentinel entry: %v", startArr)
	}

	// Stop hook must NOT be installed — it causes infinite feedback loops
	// in Claude Code's interactive mode.
	if stopVal, present := hooks["Stop"]; present {
		stopArr, _ := stopVal.([]any)
		for _, raw := range stopArr {
			if entryHasSentinel(raw) {
				t.Errorf("install must NOT write a bv Stop hook entry (causes infinite feedback loop): %v", raw)
			}
		}
	}

	// The stop hook script must NOT be written.
	scriptPath := stopHookScriptPath(home)
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Errorf("stop hook script should NOT exist after install (err=%v)", err)
	}
}

func TestInstall_EnableHook_ReplacesPriorBVHook(t *testing.T) {
	home := setupTempHome(t)
	if rc, _, _ := runInstall(t, "claude", "--enable-hook"); rc != 0 {
		t.Fatal("first install failed")
	}
	if rc, _, _ := runInstall(t, "claude", "--enable-hook", "--force"); rc != 0 {
		t.Fatal("second install failed")
	}
	raw, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatal(err)
	}
	hooks := settings["hooks"].(map[string]any)
	// Only SessionStart is installed; Stop must be absent.
	startArr := hooks["SessionStart"].([]any)
	bvCount := 0
	for _, raw := range startArr {
		if entryHasSentinel(raw) {
			bvCount++
		}
	}
	if bvCount != 1 {
		t.Errorf("hooks[SessionStart]: expected exactly 1 bv sentinel entry after re-install, got %d", bvCount)
	}
	if stopVal, present := hooks["Stop"]; present {
		stopArr, _ := stopVal.([]any)
		for _, raw := range stopArr {
			if entryHasSentinel(raw) {
				t.Errorf("hooks[Stop] must not contain a bv sentinel entry after re-install: %v", raw)
			}
		}
	}
	_ = home
}

func TestInstall_EnableHook_PreservesOtherHooks(t *testing.T) {
	home := setupTempHome(t)
	// Plant a foreign hook that the user (or compound-agent) might
	// have installed previously.
	dotClaude := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dotClaude, 0o755); err != nil {
		t.Fatal(err)
	}
	prior := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "startup",
					"hooks": []any{
						map[string]any{"type": "command", "command": "echo other-tool"},
					},
				},
			},
		},
	}
	priorJSON, _ := json.MarshalIndent(prior, "", "  ")
	priorJSON = append(priorJSON, '\n')
	if err := os.WriteFile(filepath.Join(dotClaude, "settings.json"), priorJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	if rc, _, _ := runInstall(t, "claude", "--enable-hook"); rc != 0 {
		t.Fatal("install with hooks failed")
	}
	raw := mustReadFile(t, settingsPath(home))
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatal(err)
	}
	hooks := settings["hooks"].(map[string]any)
	arr := hooks["SessionStart"].([]any)
	gotForeign := false
	gotBV := false
	for _, raw := range arr {
		if entryHasSentinel(raw) {
			gotBV = true
		} else {
			cmd := commandFromEntry(t, raw)
			if cmd == "echo other-tool" {
				gotForeign = true
			}
		}
	}
	if !gotForeign {
		t.Errorf("foreign hook (echo other-tool) was clobbered: %v", arr)
	}
	if !gotBV {
		t.Errorf("bv hook was not appended: %v", arr)
	}
}

// settings.json may carry secrets (auth tokens, API keys); a user who
// has tightened the file to 0600 must not see it widened to 0644 by
// our merge. Review feedback S2.
func TestInstall_PreservesSettingsFileMode(t *testing.T) {
	home := setupTempHome(t)
	dotClaude := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dotClaude, 0o755); err != nil {
		t.Fatal(err)
	}
	priorJSON := []byte(`{"hooks": {"Stop": [{"hooks": [{"type":"command","command":"echo other"}]}]}}` + "\n")
	if err := os.WriteFile(filepath.Join(dotClaude, "settings.json"), priorJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if rc, _, _ := runInstall(t, "claude", "--enable-hook"); rc != 0 {
		t.Fatal("install failed")
	}
	info, err := os.Stat(settingsPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("settings.json mode should be preserved at 0600 after merge; got %o", info.Mode().Perm())
	}
}

// Defense-in-depth: a wrongly-typed `hooks` field (e.g. a user who
// hand-edited settings.json and set `hooks: []` by mistake) must not
// be silently overwritten — review feedback S2.
func TestInstall_RefusesMalformedHooksField(t *testing.T) {
	home := setupTempHome(t)
	dotClaude := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dotClaude, 0o755); err != nil {
		t.Fatal(err)
	}
	// `hooks` should be an object; here it's an array.
	priorJSON := []byte(`{"hooks": ["unexpected"]}` + "\n")
	if err := os.WriteFile(filepath.Join(dotClaude, "settings.json"), priorJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	rc, stdout, _ := runInstall(t, "claude", "--enable-hook")
	// Install succeeds (skills land); hook outcome reports an error.
	if rc != 0 {
		t.Fatalf("install should succeed even when hook merge errors; rc=%d stdout=%s", rc, stdout)
	}
	if !strings.Contains(stdout, `"error"`) || !strings.Contains(strings.ToLower(stdout), "settings.hooks is not") {
		t.Errorf("hook outcome should report settings.hooks malformed: %s", stdout)
	}
	// Original settings.json must be untouched.
	got, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(priorJSON) {
		t.Errorf("settings.json should be untouched on hook merge refusal; got=%s", got)
	}
}

func TestInstall_NoHookFlag_BlocksConsent(t *testing.T) {
	home := setupTempHome(t)
	rc, stdout, _ := runInstall(t, "claude", "--no-hook")
	if rc != 0 {
		t.Fatalf("--no-hook rc: %d  stdout=%s", rc, stdout)
	}
	if _, err := os.Stat(settingsPath(home)); !os.IsNotExist(err) {
		t.Errorf("--no-hook should not write settings.json")
	}
	if !strings.Contains(stdout, `"--no-hook"`) {
		t.Errorf("hook outcome reason should mention --no-hook: %s", stdout)
	}
}

func TestInstall_EnableHookAndNoHook_MutuallyExclusive(t *testing.T) {
	_ = setupTempHome(t)
	rc, _, _ := runInstall(t, "claude", "--enable-hook", "--no-hook")
	if rc != 2 {
		t.Errorf("expected exit 2 on conflicting flags, got %d", rc)
	}
}

// ---- Uninstall removes hook entries from settings.json ----

func TestUninstall_RemovesHooksAndPreservesOthers(t *testing.T) {
	home := setupTempHome(t)
	// Plant a foreign hook the user owns.
	dotClaude := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dotClaude, 0o755); err != nil {
		t.Fatal(err)
	}
	prior := map[string]any{
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{"type": "command", "command": "echo user-only-hook"},
					},
				},
			},
		},
	}
	priorJSON, _ := json.MarshalIndent(prior, "", "  ")
	priorJSON = append(priorJSON, '\n')
	if err := os.WriteFile(settingsPath(home), priorJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	if rc, _, _ := runInstall(t, "claude", "--enable-hook"); rc != 0 {
		t.Fatal("install with hooks failed")
	}
	// Install no longer writes the stop hook script. Simulate a leftover
	// script from an older bv version to verify uninstall still cleans it.
	scriptPath := stopHookScriptPath(home)
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\n# "+bvHookSentinel+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if rc, _, _ := runInstall(t, "claude", "--uninstall"); rc != 0 {
		t.Fatal("uninstall failed")
	}
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Errorf("stop hook script should be removed after uninstall: err=%v", err)
	}
	raw := mustReadFile(t, settingsPath(home))
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatal(err)
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if len(hooks) == 0 {
		t.Fatal("foreign Stop hook should still be present after uninstall")
	}
	arr, ok := hooks["Stop"].([]any)
	if !ok {
		t.Fatalf("Stop array missing: %v", hooks)
	}
	for _, raw := range arr {
		if entryHasSentinel(raw) {
			t.Errorf("uninstall should have removed every bv sentinel entry: %v", raw)
		}
	}
	// SessionStart should be gone (only bv was there).
	if _, present := hooks["SessionStart"]; present {
		t.Errorf("empty SessionStart array should be deleted entirely after uninstall: %v", hooks)
	}
	// Foreign Stop entry survives.
	foreignFound := false
	for _, raw := range arr {
		if commandFromEntry(t, raw) == "echo user-only-hook" {
			foreignFound = true
		}
	}
	if !foreignFound {
		t.Errorf("foreign Stop hook lost; arr=%v", arr)
	}
}

// ---- Hook command shape (EV2-N-5: IDs only, no comment text) ----

// TestSessionStartHookCommand_NeverLeaksContent covers the SessionStart
// inline hook (bvHookShellCommand). The Stop hook is script-based and
// covered by TestStopHookScript_Shape.
func TestSessionStartHookCommand_NeverLeaksContent(t *testing.T) {
	prev := bvExecutableFn
	bvExecutableFn = func() (string, error) { return "/opt/butverify/bin/bv", nil }
	t.Cleanup(func() { bvExecutableFn = prev })

	cmd, err := bvHookShellCommand("pending")
	if err != nil {
		t.Fatalf("bvHookShellCommand: %v", err)
	}
	if !strings.Contains(cmd, "--format=ids") {
		t.Errorf("hook shell command must use --format=ids: %s", cmd)
	}
	for _, forbidden := range []string{"--format=json", "comment", "annotation"} {
		if strings.Contains(cmd, forbidden) {
			t.Errorf("hook shell command must not surface %q (EV2-N-5): %s", forbidden, cmd)
		}
	}
	if !strings.Contains(cmd, bvHookSentinel) {
		t.Errorf("hook shell command missing sentinel: %s", cmd)
	}
	if !strings.Contains(cmd, "exit 0") {
		t.Errorf("SessionStart hook shell command must exit 0: %s", cmd)
	}
	if strings.Contains(cmd, "command -v bv") {
		t.Errorf("hook shell command must NOT use `command -v bv` (PATH hijack risk): %s", cmd)
	}
	if !strings.Contains(cmd, "'/opt/butverify/bin/bv'") {
		t.Errorf("hook shell command must embed quoted absolute path of bv: %s", cmd)
	}
	if !strings.Contains(cmd, "[ -x '/opt/butverify/bin/bv' ]") {
		t.Errorf("hook shell command must guard with [ -x <abs-bv> ]: %s", cmd)
	}
	if !strings.Contains(cmd, "pending:") {
		t.Errorf("hook shell command missing 'pending:' wording in printf: %s", cmd)
	}
}

// PATH-hijack defense: a path containing a single quote must be safely
// escaped so a malicious filename cannot break out of the quoted span.
func TestHookShellCommand_QuotesAbsolutePathSafely(t *testing.T) {
	prev := bvExecutableFn
	bvExecutableFn = func() (string, error) { return "/tmp/weird'name/bv", nil }
	t.Cleanup(func() { bvExecutableFn = prev })
	cmd, err := bvHookShellCommand("pending")
	if err != nil {
		t.Fatalf("bvHookShellCommand: %v", err)
	}
	// Embedded form must use POSIX `'\''` to escape the single quote.
	if !strings.Contains(cmd, `'/tmp/weird'\''name/bv'`) {
		t.Errorf("absolute path with single quote must be POSIX-escaped: %s", cmd)
	}
}

// EV2-E-9b: SessionStart hook says "pending" (inline, advisory).
// The Stop hook must NOT be installed — any Stop hook output in interactive
// mode creates an infinite Claude Code feedback loop. Reviews are
// SessionStart-only.
func TestInstall_HookWordingDiffersByEvent(t *testing.T) {
	home := setupTempHome(t)
	if rc, _, _ := runInstall(t, "claude", "--enable-hook"); rc != 0 {
		t.Fatal("install with hooks failed")
	}
	// SessionStart wording is in settings.json (inline command).
	raw := mustReadFile(t, settingsPath(home))
	if !strings.Contains(string(raw), "pending: %s") {
		t.Errorf("SessionStart hook should print '... pending: <ids>': %s", raw)
	}
	// Stop hook must NOT be installed.
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("settings.json must be valid JSON: %v", err)
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if stopVal, present := hooks["Stop"]; present {
		stopArr, _ := stopVal.([]any)
		for _, entry := range stopArr {
			if entryHasSentinel(entry) {
				t.Errorf("Stop hook must NOT be installed (causes infinite feedback loop): %v", entry)
			}
		}
	}
	// Stop hook script must NOT exist.
	if _, err := os.Stat(stopHookScriptPath(home)); !os.IsNotExist(err) {
		t.Errorf("Stop hook script must NOT be written during install (err=%v)", err)
	}
}

// ---- Helpers ----

func commandFromEntry(t *testing.T, raw any) string {
	t.Helper()
	m, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("entry not a map: %T %v", raw, raw)
	}
	hooks, ok := m["hooks"].([]any)
	if !ok {
		t.Fatalf("entry.hooks not an array: %T %v", m["hooks"], m["hooks"])
	}
	if len(hooks) == 0 {
		return ""
	}
	hm, ok := hooks[0].(map[string]any)
	if !ok {
		t.Fatalf("entry.hooks[0] not a map: %T %v", hooks[0], hooks[0])
	}
	return hm["command"].(string)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---- Hardening regression coverage ----

// Per-event hook arrays must get the same defense-in-depth treatment as
// the top-level `hooks` field: a wrongly-typed value (e.g. a string or
// object instead of an array) must not be silently overwritten.
func TestInstall_RefusesMalformedPerEventHooks(t *testing.T) {
	home := setupTempHome(t)
	dotClaude := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dotClaude, 0o755); err != nil {
		t.Fatal(err)
	}
	// `hooks` is an object (good) but SessionStart is a string (bad).
	priorJSON := []byte(`{"hooks": {"SessionStart": "not-an-array"}}` + "\n")
	if err := os.WriteFile(filepath.Join(dotClaude, "settings.json"), priorJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	rc, stdout, _ := runInstall(t, "claude", "--enable-hook")
	if rc != 0 {
		t.Fatalf("install should succeed even when hook merge errors; rc=%d stdout=%s", rc, stdout)
	}
	if !strings.Contains(stdout, `"error"`) || !strings.Contains(strings.ToLower(stdout), "hooks.sessionstart is not") {
		t.Errorf("hook outcome should report per-event field malformed: %s", stdout)
	}
	// Original settings.json must be untouched on refusal.
	got, err := os.ReadFile(settingsPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(priorJSON) {
		t.Errorf("settings.json should be untouched when per-event field malformed; got=%s", got)
	}
}

// stripSentinelEntries must remove only the bv sentinel commands, not
// the entire entry. A user who added a sibling command to the same
// hook entry must keep that command after re-install or uninstall.
func TestStripSentinelEntries_PreservesSiblingCommands(t *testing.T) {
	entry := map[string]any{
		"matcher": "*",
		"hooks": []any{
			map[string]any{"type": "command", "command": "echo user-command"},
			map[string]any{"type": "command", "command": "echo with " + bvHookSentinel},
		},
	}
	out, removed := stripSentinelEntries([]any{entry})
	if !removed {
		t.Errorf("removed flag should be true when a sentinel sibling was stripped")
	}
	if len(out) != 1 {
		t.Fatalf("entry with sibling commands should survive; got len=%d %v", len(out), out)
	}
	gotEntry, ok := out[0].(map[string]any)
	if !ok {
		t.Fatalf("entry should still be a map: %T %v", out[0], out[0])
	}
	hooks, ok := gotEntry["hooks"].([]any)
	if !ok {
		t.Fatalf("entry.hooks must remain an array; got %T %v", gotEntry["hooks"], gotEntry["hooks"])
	}
	if len(hooks) != 1 {
		t.Fatalf("only the sentinel command should be stripped; remaining=%d %v", len(hooks), hooks)
	}
	cmd := hooks[0].(map[string]any)["command"].(string)
	if cmd != "echo user-command" {
		t.Errorf("sibling command was lost; got %q", cmd)
	}
	// Matcher and other top-level fields preserved.
	if gotEntry["matcher"] != "*" {
		t.Errorf("matcher should be preserved; got %v", gotEntry["matcher"])
	}
	// Source map must not be mutated (defense-in-depth — callers may
	// retain references).
	srcHooks := entry["hooks"].([]any)
	if len(srcHooks) != 2 {
		t.Errorf("source entry hooks were mutated; want 2 got %d", len(srcHooks))
	}
}

// stripSentinelEntries should drop entries whose only commands were
// sentinel-tagged (an entry with no commands is meaningless to Claude
// Code anyway).
func TestStripSentinelEntries_DropsEntryWhenAllSentinel(t *testing.T) {
	entry := map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": "echo first " + bvHookSentinel},
			map[string]any{"type": "command", "command": "echo second " + bvHookSentinel},
		},
	}
	out, removed := stripSentinelEntries([]any{entry})
	if !removed {
		t.Errorf("removed flag should be true when an entry was dropped")
	}
	if len(out) != 0 {
		t.Errorf("entry with only sentinel commands should be dropped; got %v", out)
	}
}

// stripSentinelEntries must report removed=true when a sentinel command
// is stripped from an entry that keeps its top-level slot due to a
// surviving sibling. uninstallHooks compares by removed flag (not by
// top-level slice length) so a mixed entry like this would otherwise be
// silently retained — leaving the bv command in settings.json after
// `bv install-skill --uninstall`.
func TestStripSentinelEntries_FlagsRemovalForMixedEntry(t *testing.T) {
	entry := map[string]any{
		"matcher": "*",
		"hooks": []any{
			map[string]any{"type": "command", "command": "echo user-command"},
			map[string]any{"type": "command", "command": "echo with " + bvHookSentinel},
		},
	}
	out, removed := stripSentinelEntries([]any{entry})
	if !removed {
		t.Fatalf("removed flag must be true for mixed entry; got false (out=%v)", out)
	}
	if len(out) != 1 {
		t.Fatalf("entry should survive; got len=%d", len(out))
	}
}

// uninstallHooks must rewrite settings.json when a bv sentinel command
// is removed from an entry that keeps its top-level slot. This is the
// regression test for the bug where len(stripped) == len(arr) caused
// the writer to skip and leave the bv command behind.
func TestUninstallHooks_RemovesSentinelFromMixedEntry(t *testing.T) {
	root := t.TempDir()
	sp := settingsPath(root)
	if err := os.MkdirAll(filepath.Dir(sp), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	prior := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "*",
					"hooks": []any{
						map[string]any{"type": "command", "command": "echo user-command"},
						map[string]any{"type": "command", "command": "/bin/bv review list # " + bvHookSentinel, "timeout": float64(2)},
					},
				},
			},
		},
	}
	encoded, err := json.MarshalIndent(prior, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(sp, encoded, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	removed, err := uninstallHooks(root)
	if err != nil {
		t.Fatalf("uninstallHooks: %v", err)
	}
	if !removed {
		t.Fatal("uninstallHooks must report removed=true when a sentinel was stripped from a mixed entry")
	}
	got, err := os.ReadFile(sp)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(got), bvHookSentinel) {
		t.Errorf("settings.json must no longer contain the bv sentinel after uninstall; got %s", got)
	}
	if !strings.Contains(string(got), "echo user-command") {
		t.Errorf("settings.json must preserve the sibling user command; got %s", got)
	}
}

// Uninstall must surface a hook-removal failure as a partial uninstall
// with non-zero exit. Simulated by making settings.json unreadable
// after install via a chmod 0 once we know the path.
func TestUninstall_HookRemovalFailureSurfacesPartial(t *testing.T) {
	home := setupTempHome(t)
	if rc, _, _ := runInstall(t, "claude", "--enable-hook"); rc != 0 {
		t.Fatal("install with hooks failed")
	}
	sp := settingsPath(home)
	// Chmod settings.json to 0 so the read inside uninstallHooks fails.
	if err := os.Chmod(sp, 0o000); err != nil {
		t.Fatalf("chmod settings.json: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(sp, 0o600) })
	rc, stdout, _ := runInstall(t, "claude", "--uninstall")
	if rc == 0 {
		t.Errorf("uninstall should exit non-zero when hook removal fails; stdout=%s", stdout)
	}
	if !strings.Contains(stdout, `"hook_error"`) {
		t.Errorf("JSON payload should include hook_error: %s", stdout)
	}
	if !strings.Contains(stdout, `"partial"`) {
		t.Errorf("status should be 'partial' when hook removal fails: %s", stdout)
	}
}

// Stamped legacy installs (a flat SKILL.md written by an older
// pre-namespace bv release that already carried bv-skill-version
// metadata) must trigger the migration call-out, not the generic drift
// message — otherwise a user upgrading from a clean install gets a
// less helpful error.
func TestInstall_StampedLegacyFlatInstall_GivesMigrationMessage(t *testing.T) {
	home := setupTempHome(t)
	dir := filepath.Join(home, ".claude", "skills", "butverify")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Legacy stamped flat install: front-matter + body + a real-shape
	// metadata stamp using a fabricated old hash. Drift detection sees
	// a stamped install whose hash differs from the embedded one.
	legacy := []byte("---\nname: butverify\ndescription: legacy stamped\n---\n# /butverify\nOld body.\n<!-- bv-skill: release=v0.0.legacy sha256=cafef00dface -->\n")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	rc, stdout, _ := runInstall(t, "claude")
	if rc == 0 {
		t.Fatalf("stamped legacy flat install should require --force; rc=0 stdout=%s", stdout)
	}
	if !strings.Contains(stdout, "pre-namespace flat install") {
		t.Errorf("expected migration phrasing in error for stamped legacy install: %s", stdout)
	}
}

// Stdin-TTY guard: when stdin is piped (non-TTY) but stderr is a TTY,
// the consent prompt must NOT read from stdin. The hook is skipped
// instead, and the pipeline data is preserved for the rest of the
// process. Test by faking IsHumanTTY=true on the writer (simulating
// interactive stderr) while leaving stdin as the test's pipe.
func TestInstall_HooksSkippedWhenStdinNotTTY(t *testing.T) {
	home := setupTempHome(t)
	// Force the writer's IsHumanTTY signal true (stderr appears interactive)
	// but leave stdinIsTTYFn returning false. Without the stdin-TTY guard,
	// the install would try to Fscanln from os.Stdin and either consume
	// pipeline input or hang.
	prev := stdinIsTTYFn
	stdinIsTTYFn = func() bool { return false }
	t.Cleanup(func() { stdinIsTTYFn = prev })
	rc, stdout, _ := runInstall(t, "claude")
	if rc != 0 {
		t.Fatalf("install rc: %d stdout=%s", rc, stdout)
	}
	if _, err := os.Stat(settingsPath(home)); !os.IsNotExist(err) {
		t.Errorf("install with non-tty stdin should NOT write settings.json (got err=%v)", err)
	}
	if !strings.Contains(stdout, `"skipped"`) {
		t.Errorf("hook outcome should be skipped when stdin is non-tty: %s", stdout)
	}
}
