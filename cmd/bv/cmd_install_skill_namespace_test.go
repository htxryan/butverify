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

func TestInstall_WritesAllThreeSkillFiles(t *testing.T) {
	home := setupTempHome(t)
	rc, _, _ := runInstall(t, "claude")
	if rc != 0 {
		t.Fatalf("install rc: %d", rc)
	}
	want := []string{
		filepath.Join(home, ".claude", "skills", "butverify", "SKILL.md"),
		filepath.Join(home, ".claude", "skills", "butverify", "prove-it", "SKILL.md"),
		filepath.Join(home, ".claude", "skills", "butverify", "review", "SKILL.md"),
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
	for _, evt := range []string{"SessionStart", "Stop"} {
		arr, ok := hooks[evt].([]any)
		if !ok {
			t.Errorf("hooks[%s] not an array: %T %v", evt, hooks[evt], hooks[evt])
			continue
		}
		if len(arr) == 0 {
			t.Errorf("hooks[%s] is empty", evt)
			continue
		}
		// Every entry should have a hooks[*].command field; at least one
		// must contain the bv sentinel and the documented bv review
		// invocation.
		found := false
		for _, raw := range arr {
			if entryHasSentinel(raw) {
				found = true
				cmd := commandFromEntry(t, raw)
				if !strings.Contains(cmd, "bv review list --unacknowledged --format=ids") {
					t.Errorf("hooks[%s] sentinel entry should call bv review list ...; got %s", evt, cmd)
				}
				if !strings.Contains(cmd, "[butverify]") {
					t.Errorf("hooks[%s] entry should print the [butverify] prefix; got %s", evt, cmd)
				}
			}
		}
		if !found {
			t.Errorf("hooks[%s] missing sentinel entry: %v", evt, arr)
		}
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
	for _, evt := range []string{"SessionStart", "Stop"} {
		arr := hooks[evt].([]any)
		bvCount := 0
		for _, raw := range arr {
			if entryHasSentinel(raw) {
				bvCount++
			}
		}
		if bvCount != 1 {
			t.Errorf("hooks[%s]: expected exactly 1 bv sentinel entry after re-install, got %d", evt, bvCount)
		}
	}
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
	if rc, _, _ := runInstall(t, "claude", "--uninstall"); rc != 0 {
		t.Fatal("uninstall failed")
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

func TestHookShellCommand_NeverLeaksContent(t *testing.T) {
	for _, phase := range []string{"pending", "still pending"} {
		phase := phase
		t.Run(phase, func(t *testing.T) {
			cmd := bvHookShellCommand(phase)
			// EV2-N-5: hook output only emits IDs and a count. The shell
			// guard must call bv with --format=ids (no JSON, no content).
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
				t.Errorf("hook shell command must exit 0 unconditionally: %s", cmd)
			}
			// `command -v` guard must precede the bv call so a missing
			// binary on PATH never errors out.
			if !strings.Contains(cmd, "command -v bv") {
				t.Errorf("hook shell command must guard with `command -v bv`: %s", cmd)
			}
			// Phase wording must match the spec for the given event.
			if !strings.Contains(cmd, phase+":") {
				t.Errorf("hook shell command for phase %q missing wording in printf: %s", phase, cmd)
			}
		})
	}
}

// EV2-E-9b vs EV2-E-9c: the SessionStart entry says "pending" while
// the Stop entry says "still pending" so the human reads the Stop hook
// as a reminder of unfinished work. Pin both wordings via the actual
// settings.json that gets written.
func TestInstall_HookWordingDiffersByEvent(t *testing.T) {
	home := setupTempHome(t)
	if rc, _, _ := runInstall(t, "claude", "--enable-hook"); rc != 0 {
		t.Fatal("install with hooks failed")
	}
	raw := mustReadFile(t, settingsPath(home))
	body := string(raw)
	if !strings.Contains(body, "pending: %s") {
		t.Errorf("SessionStart hook should print '... pending: <ids>': %s", body)
	}
	if !strings.Contains(body, "still pending: %s") {
		t.Errorf("Stop hook should print '... still pending: <ids>' (EV2-E-9c): %s", body)
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
