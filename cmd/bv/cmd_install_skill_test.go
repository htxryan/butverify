// Tests for `bv install-skill claude` (E13-T2). Cover the
// EARS-E surface (BVS-E-1..6), pure-function determinism for the BVS-U-9
// canonical-hash-domain transform (BVS-U-7 / BVS-SC-13), the BVS-N-3
// symlink-containment rejection, the BVS-S-1 atomic-write +
// fixed-uninstall-set scope (BVS-E-4), and the build-time embed
// fitness functions (size cap + canonical-source byte equality).

package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/htxryan/butverify/internal/config"
)

// setupTempHome stages a tempdir as $HOME (and points the bv config
// inside it as a logged-in installation), then returns the home path.
// Tests pass the resulting `home` to claudeSkillPath() to derive the
// expected install path.
func setupTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Plant a logged-in config — most tests want the login probe to
	// succeed so they can exercise remote-mode-adjacent logic. Local-first
	// install also works without this config.
	cfgPath := filepath.Join(home, "bv-config.json")
	t.Setenv("BV_CONFIG_PATH", cfgPath)
	if err := config.Save(&config.Config{
		APIURL:            "https://api.example.test",
		InstallationToken: "ghs_test",
		TenantID:          "t_test",
		AccountLogin:      "tester",
		Mode:              config.ModeRemote,
	}); err != nil {
		t.Fatalf("setup config: %v", err)
	}
	return home
}

// setupTempHomeUnconfigured stages $HOME without writing a bv config.
// Used for the BVS-E-6 not-logged-in test.
func setupTempHomeUnconfigured(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BV_CONFIG_PATH", filepath.Join(home, "absent-config.json"))
	return home
}

// runInstall runs the runInstallSkill entry point with a JSON writer
// and returns the exit code, stdout buffer, and stderr buffer.
func runInstall(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	w, stdout, stderr := newJSONWriter(t)
	rc := runInstallSkill(context.Background(), globalContext{w: w}, args)
	return rc, stdout.String(), stderr.String()
}

// ---- BVS-U-9: pure-function determinism for the canonical hash domain ----

func TestSkillVersionHash_Determinism(t *testing.T) {
	hash := skillVersionHash(embeddedSkillBytes)
	if len(hash) != 12 {
		t.Errorf("hash length: want 12 hex chars, got %d (%q)", len(hash), hash)
	}
	for _, r := range hash {
		ok := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
		if !ok {
			t.Errorf("hash should be lowercase hex; got rune %q in %q", r, hash)
		}
	}
	// Round-trip: stamping with the embed's own hash MUST yield bytes
	// whose canonical-domain hash equals the original. This is the
	// install-time contract — a re-hash of the installed file matches
	// what the installer wrote into the frontmatter.
	stamped := stampVersion(embeddedSkillBytes, hash)
	rehash := skillVersionHash(stamped)
	if rehash != hash {
		t.Errorf("BVS-U-9 round-trip broke: hash=%q rehash=%q", hash, rehash)
	}
}

func TestSkillVersionHash_LFNormalize(t *testing.T) {
	lf := []byte("---\nname: x\nbv-skill-version: aaaaaaaaaaaa\n---\nbody\n")
	crlf := []byte("---\r\nname: x\r\nbv-skill-version: aaaaaaaaaaaa\r\n---\r\nbody\r\n")
	cr := []byte("---\rname: x\rbv-skill-version: aaaaaaaaaaaa\r---\rbody\r")
	a := skillVersionHash(lf)
	b := skillVersionHash(crlf)
	c := skillVersionHash(cr)
	if a != b || a != c {
		t.Errorf("line-ending normalization broken: lf=%s crlf=%s cr=%s", a, b, c)
	}
}

func TestSkillVersionHash_SentinelSubstitution(t *testing.T) {
	// Inputs that differ only in the bv-skill-version value MUST hash
	// identically — the canonical domain rewrites the line to the
	// sentinel before hashing.
	a := []byte("---\nname: x\nbv-skill-version: 0000000000ab\n---\nbody\n")
	b := []byte("---\nname: x\nbv-skill-version: ffffffffffff\n---\nbody\n")
	c := []byte("---\nname: x\nbv-skill-version: 000000000000\n---\nbody\n")
	ha := skillVersionHash(a)
	hb := skillVersionHash(b)
	hc := skillVersionHash(c)
	if ha != hb || ha != hc {
		t.Errorf("sentinel substitution broken: a=%s b=%s c=%s", ha, hb, hc)
	}
}

func TestSkillVersionHash_TrailingLF(t *testing.T) {
	// With and without trailing LF MUST hash identically.
	a := []byte("---\nname: x\nbv-skill-version: 000000000000\n---\nbody\n")
	b := []byte("---\nname: x\nbv-skill-version: 000000000000\n---\nbody")
	if skillVersionHash(a) != skillVersionHash(b) {
		t.Errorf("trailing-LF normalization broken: %s vs %s", skillVersionHash(a), skillVersionHash(b))
	}
}

func TestStampVersion_Idempotent(t *testing.T) {
	in := []byte("---\nname: x\nbv-skill-version: 000000000000\n---\nbody\n")
	once := stampVersion(in, "abc123abc123")
	twice := stampVersion(once, "deffeeffadda")
	// Latest wins; the line is REPLACED, not appended.
	if !strings.Contains(string(twice), "bv-skill-version: deffeeffadda") {
		t.Errorf("twice-stamped should carry latest hash: %s", twice)
	}
	if strings.Contains(string(twice), "bv-skill-version: abc123abc123") {
		t.Errorf("twice-stamped should NOT keep prior hash: %s", twice)
	}
	if strings.Count(string(twice), versionLinePrefix) != 1 {
		t.Errorf("expected exactly one bv-skill-version line, got: %s", twice)
	}
}

// ---- Embed mirror byte-equality + size budget ----

func TestEmbedMirror_MatchesCanonical(t *testing.T) {
	// Locate the canonical source file relative to this test's
	// runtime cwd. Tests run from the package directory (cmd/bv),
	// so bv-skills lives two directories up at the repo root.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	canonicalPath := filepath.Join(cwd, "..", "..", "bv-skills", "claude", "butverify.md")
	canonical, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical %q: %v", canonicalPath, err)
	}
	if string(canonical) != string(embeddedSkillBytes) {
		t.Errorf("embed mirror has drifted from canonical source (%d vs %d bytes); rerun the build-time mirror copy",
			len(canonical), len(embeddedSkillBytes))
	}
}

func TestEmbeddedSize(t *testing.T) {
	// Fitness function 2: skill markdown stays under 10 KiB. The
	// current canonical content is ~2.4 KiB, so this gate gives ~4x
	// growth headroom before a refactor is forced.
	const cap = 10 * 1024
	if len(embeddedSkillBytes) >= cap {
		t.Errorf("embedded skill size %d >= cap %d; trim the markdown or raise the cap with a deliberate decision",
			len(embeddedSkillBytes), cap)
	}
}

// ---- BVS-E-1: fresh install ----

func TestInstall_FreshHome(t *testing.T) {
	home := setupTempHome(t)
	rc, _, _ := runInstall(t, "claude")
	if rc != 0 {
		t.Fatalf("rc: %d", rc)
	}
	skillPath := claudeSkillPath(home)
	info, err := os.Stat(skillPath)
	if err != nil {
		t.Fatalf("SKILL.md not written: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("SKILL.md mode: want 0644, got %o", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(skillPath))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o755 {
		t.Errorf("butverify dir mode: want 0755, got %o", dirInfo.Mode().Perm())
	}
	contents, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	// Frontmatter should carry a 12-hex stamp (NOT zeros).
	embeddedHash := skillVersionHash(embeddedSkillBytes)
	wantLine := "bv-skill-version: " + embeddedHash
	if !strings.Contains(string(contents), wantLine) {
		t.Errorf("installed file missing stamped hash %q (got: %s)", wantLine, contents[:200])
	}
	if strings.Contains(string(contents), "bv-skill-version: 000000000000") {
		t.Errorf("installed file still has zero sentinel: %s", contents[:200])
	}
}

// ---- BVS-E-2: re-install drift detection ----

func TestInstall_AlreadyCurrent(t *testing.T) {
	home := setupTempHome(t)
	if rc, _, _ := runInstall(t, "claude"); rc != 0 {
		t.Fatalf("first install rc: %d", rc)
	}
	skillPath := claudeSkillPath(home)
	infoBefore, err := os.Stat(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	// Sleep briefly so a touch would change mtime detectably on
	// filesystems with second-resolution mtime.
	time.Sleep(20 * time.Millisecond)

	rc, stdout, _ := runInstall(t, "claude")
	if rc != 0 {
		t.Fatalf("second install rc: %d  stdout=%s", rc, stdout)
	}
	if !strings.Contains(stdout, `"already_current"`) {
		t.Errorf("expected already_current status: %s", stdout)
	}
	infoAfter, err := os.Stat(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Errorf("BVS-E-2: mtime should be unchanged when already current (before=%v after=%v)",
			infoBefore.ModTime(), infoAfter.ModTime())
	}
}

func TestInstall_DriftDetected(t *testing.T) {
	home := setupTempHome(t)
	if rc, _, _ := runInstall(t, "claude"); rc != 0 {
		t.Fatalf("first install rc: %d", rc)
	}
	skillPath := claudeSkillPath(home)
	// Manually rewrite the frontmatter to a different version.
	cur, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	embeddedHash := skillVersionHash(embeddedSkillBytes)
	driftedHash := "deadbeef0000"
	if embeddedHash == driftedHash {
		t.Fatal("hash collision; pick a different drift value")
	}
	mutated := strings.Replace(string(cur), "bv-skill-version: "+embeddedHash, "bv-skill-version: "+driftedHash, 1)
	if err := os.WriteFile(skillPath, []byte(mutated), 0o644); err != nil {
		t.Fatal(err)
	}

	rc, stdout, _ := runInstall(t, "claude")
	if rc == 0 {
		t.Fatalf("expected non-zero rc on drift; got 0 (stdout=%s)", stdout)
	}
	if !strings.Contains(stdout, driftedHash) {
		t.Errorf("error envelope should name installed (drifted) version %s: %s", driftedHash, stdout)
	}
	if !strings.Contains(stdout, embeddedHash) {
		t.Errorf("error envelope should name embedded version %s: %s", embeddedHash, stdout)
	}
	if !strings.Contains(stdout, "--force") {
		t.Errorf("error envelope should advise --force: %s", stdout)
	}
	// File MUST NOT have been overwritten.
	postCur, _ := os.ReadFile(skillPath)
	if !strings.Contains(string(postCur), "bv-skill-version: "+driftedHash) {
		t.Errorf("BVS-E-2: file should be unchanged on drift refusal; got: %s", postCur[:200])
	}
}

// ---- BVS-E-3: --force ----

func TestInstall_Force(t *testing.T) {
	home := setupTempHome(t)
	if rc, _, _ := runInstall(t, "claude"); rc != 0 {
		t.Fatalf("first install rc: %d", rc)
	}
	skillPath := claudeSkillPath(home)
	embeddedHash := skillVersionHash(embeddedSkillBytes)
	cur, _ := os.ReadFile(skillPath)
	driftedBytes := []byte(strings.Replace(string(cur), "bv-skill-version: "+embeddedHash, "bv-skill-version: cafef00dface", 1))
	if err := os.WriteFile(skillPath, driftedBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	rc, _, _ := runInstall(t, "claude", "--force")
	if rc != 0 {
		t.Fatalf("--force rc: %d", rc)
	}
	// New file should carry the embedded hash.
	newCur, _ := os.ReadFile(skillPath)
	if !strings.Contains(string(newCur), "bv-skill-version: "+embeddedHash) {
		t.Errorf("after --force, file should carry embedded hash; got: %s", newCur[:200])
	}
	// .bak should hold the OLD (drifted) bytes.
	bak, err := os.ReadFile(skillPath + ".bak")
	if err != nil {
		t.Fatalf("read .bak: %v", err)
	}
	if string(bak) != string(driftedBytes) {
		t.Errorf("BVS-E-3: .bak should hold the immediate-previous bytes; got len=%d, want len=%d",
			len(bak), len(driftedBytes))
	}
	bakInfo, _ := os.Stat(skillPath + ".bak")
	if bakInfo.Mode().Perm() != 0o644 {
		t.Errorf(".bak mode: want 0644, got %o", bakInfo.Mode().Perm())
	}
}

func TestInstall_Force_OverwritesPriorBak(t *testing.T) {
	home := setupTempHome(t)
	if rc, _, _ := runInstall(t, "claude"); rc != 0 {
		t.Fatalf("first install rc: %d", rc)
	}
	skillPath := claudeSkillPath(home)

	// Drift v1 -> --force produces .bak v1 (containing v1).
	v1 := mustReadFile(t, skillPath)
	driftV1 := strings.Replace(string(v1), "bv-skill-version: "+skillVersionHash(embeddedSkillBytes), "bv-skill-version: 111111111111", 1)
	if err := os.WriteFile(skillPath, []byte(driftV1), 0o644); err != nil {
		t.Fatal(err)
	}
	if rc, _, _ := runInstall(t, "claude", "--force"); rc != 0 {
		t.Fatal("force #1 failed")
	}
	bakV1 := mustReadFile(t, skillPath+".bak")
	if !strings.Contains(string(bakV1), "111111111111") {
		t.Fatalf("setup: .bak v1 should contain drift v1; got: %s", bakV1)
	}

	// Drift v2 -> --force produces .bak v2 (containing v2). Prior .bak
	// (v1) MUST be overwritten — .bak holds the IMMEDIATE-PREVIOUS
	// state, NOT the original.
	v2 := mustReadFile(t, skillPath)
	driftV2 := strings.Replace(string(v2), "bv-skill-version: "+skillVersionHash(embeddedSkillBytes), "bv-skill-version: 222222222222", 1)
	if err := os.WriteFile(skillPath, []byte(driftV2), 0o644); err != nil {
		t.Fatal(err)
	}
	if rc, _, _ := runInstall(t, "claude", "--force"); rc != 0 {
		t.Fatal("force #2 failed")
	}
	bakV2 := mustReadFile(t, skillPath+".bak")
	if !strings.Contains(string(bakV2), "222222222222") {
		t.Errorf("BVS-E-3: .bak should hold drift v2 (immediate-previous), got: %s", bakV2)
	}
	if strings.Contains(string(bakV2), "111111111111") {
		t.Errorf("BVS-E-3: .bak should NOT still hold drift v1 (the original): %s", bakV2)
	}
}

// ---- BVS-U-3: --project ----

func TestInstall_Project(t *testing.T) {
	// Stage two distinct dirs: $HOME and CWD. Run --project; only the
	// CWD-rooted path should be touched.
	home := setupTempHome(t)
	cwd := t.TempDir()
	prevWD, _ := os.Getwd()
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })

	rc, _, _ := runInstall(t, "claude", "--project")
	if rc != 0 {
		t.Fatalf("--project rc: %d", rc)
	}
	projectPath := filepath.Join(cwd, ".claude", "skills", "butverify", "SKILL.md")
	if _, err := os.Stat(projectPath); err != nil {
		t.Errorf("BVS-U-3: project SKILL.md not written at %q: %v", projectPath, err)
	}
	homePath := claudeSkillPath(home)
	if _, err := os.Stat(homePath); !os.IsNotExist(err) {
		t.Errorf("BVS-U-3: --project should NOT touch $HOME path %q (err=%v)", homePath, err)
	}
}

// ---- BVS-E-4: --uninstall ----

func TestUninstall_Installed(t *testing.T) {
	home := setupTempHome(t)
	if rc, _, _ := runInstall(t, "claude"); rc != 0 {
		t.Fatal("setup install failed")
	}
	skillPath := claudeSkillPath(home)
	// Plant a .bak so we exercise the BVS-E-4 fixed-set removal.
	if err := os.WriteFile(skillPath+".bak", []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	rc, _, _ := runInstall(t, "claude", "--uninstall")
	if rc != 0 {
		t.Fatalf("uninstall rc: %d", rc)
	}
	if _, err := os.Stat(skillPath); !os.IsNotExist(err) {
		t.Errorf("SKILL.md should be removed: err=%v", err)
	}
	if _, err := os.Stat(skillPath + ".bak"); !os.IsNotExist(err) {
		t.Errorf(".bak should be removed: err=%v", err)
	}
	// butverify/ dir was empty after removal -> removed.
	if _, err := os.Stat(filepath.Dir(skillPath)); !os.IsNotExist(err) {
		t.Errorf("butverify/ dir should be removed: err=%v", err)
	}
	// Parent (~/.claude/skills/) MUST survive.
	skillsDir := filepath.Dir(filepath.Dir(skillPath))
	if _, err := os.Stat(skillsDir); err != nil {
		t.Errorf("BVS-E-4: ancestor dir %q should NOT be touched: %v", skillsDir, err)
	}
}

func TestUninstall_NotInstalled(t *testing.T) {
	_ = setupTempHome(t)
	rc, stdout, _ := runInstall(t, "claude", "--uninstall")
	if rc != 0 {
		t.Errorf("BVS-SC-5b: uninstall on clean dir should return 0, got %d", rc)
	}
	if !strings.Contains(stdout, "not_installed") {
		t.Errorf("expected not_installed status in JSON: %s", stdout)
	}
	// Idempotent — repeated.
	rc2, _, _ := runInstall(t, "claude", "--uninstall")
	if rc2 != 0 {
		t.Errorf("idempotent uninstall: rc2=%d", rc2)
	}
}

func TestUninstall_PreservesSiblings(t *testing.T) {
	home := setupTempHome(t)
	if rc, _, _ := runInstall(t, "claude"); rc != 0 {
		t.Fatal("setup install failed")
	}
	skillPath := claudeSkillPath(home)
	// Plant an unrelated file inside butverify/.
	planted := filepath.Join(filepath.Dir(skillPath), "unrelated.md")
	if err := os.WriteFile(planted, []byte("user-owned"), 0o644); err != nil {
		t.Fatal(err)
	}

	rc, _, _ := runInstall(t, "claude", "--uninstall")
	if rc != 0 {
		t.Fatalf("uninstall rc: %d", rc)
	}
	if _, err := os.Stat(skillPath); !os.IsNotExist(err) {
		t.Errorf("SKILL.md should be removed")
	}
	if _, err := os.Stat(planted); err != nil {
		t.Errorf("BVS-E-4 fixed-set: planted sibling should be preserved: %v", err)
	}
	// Dir is non-empty -> preserved.
	if _, err := os.Stat(filepath.Dir(skillPath)); err != nil {
		t.Errorf("BVS-E-4: butverify/ dir should be preserved when non-empty: %v", err)
	}
}

func TestUninstall_NeverAncestors(t *testing.T) {
	home := setupTempHome(t)
	// Plant a file in ~/.claude/skills/ (the parent of butverify/).
	skillsDir := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	parentPlant := filepath.Join(skillsDir, "user-other-skill.md")
	if err := os.WriteFile(parentPlant, []byte("user owned"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rc, _, _ := runInstall(t, "claude"); rc != 0 {
		t.Fatal("setup install failed")
	}
	if rc, _, _ := runInstall(t, "claude", "--uninstall"); rc != 0 {
		t.Fatal("uninstall failed")
	}

	if _, err := os.Stat(parentPlant); err != nil {
		t.Errorf("BVS-E-4: ~/.claude/skills/ plant should survive: %v", err)
	}
	if _, err := os.Stat(skillsDir); err != nil {
		t.Errorf("BVS-E-4: ~/.claude/skills/ should survive: %v", err)
	}
}

// ---- BVS-E-5: unsupported agent ----

func TestUnsupportedAgent(t *testing.T) {
	_ = setupTempHome(t)
	rc, stdout, _ := runInstall(t, "cursor")
	if rc == 0 {
		t.Errorf("BVS-E-5: unsupported agent should exit non-zero")
	}
	if !strings.Contains(stdout, "claude") {
		t.Errorf("BVS-E-5: error envelope should list `claude` as supported: %s", stdout)
	}
}

// ---- BVS-E-6: login-free local-first install ----

func TestInstallSkillDoesNotRequireLogin(t *testing.T) {
	home := setupTempHomeUnconfigured(t)
	rc, stdout, _ := runInstall(t, "claude")
	if rc != 0 {
		t.Fatalf("install-skill should work without login in local-first mode, rc=%d stdout=%s", rc, stdout)
	}
	skillPath := claudeSkillPath(home)
	if _, err := os.Stat(skillPath); err != nil {
		t.Errorf("skill should be written without login: %v", err)
	}
}

// ---- BVS-N-3 / BVS-SC-9: symlink containment ----

func TestSymlinkContainment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("symlink test requires admin on Windows; skipping")
	}
	home := setupTempHome(t)
	// Create an out-of-tree dir, then symlink ~/.claude/skills/ to it.
	outOfTree := t.TempDir() // never inside `home`
	dotClaude := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dotClaude, 0o755); err != nil {
		t.Fatal(err)
	}
	skillsDir := filepath.Join(dotClaude, "skills")
	if err := os.Symlink(outOfTree, skillsDir); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	rc, stdout, _ := runInstall(t, "claude")
	if rc == 0 {
		t.Errorf("BVS-N-3: symlink-out-of-home should be rejected")
	}
	if !strings.Contains(strings.ToLower(stdout), "escapes") && !strings.Contains(strings.ToLower(stdout), "containment") && !strings.Contains(strings.ToLower(stdout), "outside") {
		t.Errorf("BVS-N-3: error should describe containment violation: %s", stdout)
	}
}

// ---- Fitness: install time budget ----

func TestInstallTimeBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("perf gate skipped in -short mode")
	}
	_ = setupTempHome(t)
	start := time.Now()
	rc, _, _ := runInstall(t, "claude")
	d := time.Since(start)
	if rc != 0 {
		t.Fatalf("rc: %d", rc)
	}
	// Generous budget: 100ms p95 on the local laptop. The actual
	// install does ~5 syscalls (mkdir, open tmp, write, close,
	// rename), so the bulk of variance is filesystem cache state.
	if d > 100*time.Millisecond {
		t.Errorf("install latency %v exceeds 100ms budget", d)
	}
}

// ---- AC-20: concurrent install race (BVS-S-2) ----

// TestInstall_Concurrent pins BVS-S-2 / AC-20: two (or more) concurrent
// `bv install-skill claude` invocations on the same target path race,
// last-writer-wins, the final SKILL.md byte-equals the canonical
// post-install bytes, and no SKILL.md.tmp.* siblings remain.
//
// Spec §4.3 BVS-S-2; Acceptance Criteria AC-20.
//
// Why this passes: atomicWrite uses an O_EXCL same-dir tmp + os.Rename.
// The OS rename is a single inode swap — whoever renames last wins.
// Earlier renames are clobbered (their bytes go to /dev/null). All N
// goroutines are stamping the SAME embedded hash, so any rename order
// produces a byte-identical final file. The early-out "already_current"
// branch (BVS-E-2) trips for any caller that lands AFTER the first
// successful rename — those callers don't even attempt a second write,
// which strengthens the no-stray-tmp guarantee.
//
// Mutation contract: removing the same-dir tmp + Rename pattern (e.g.
// regressing to a direct-write-to-dst) MUST fail this test by either
// leaving partial bytes in the dst or by splattering tmp siblings.
func TestInstall_Concurrent(t *testing.T) {
	home := setupTempHome(t)
	skillPath := claudeSkillPath(home)
	dir := filepath.Dir(skillPath)

	const n = 8
	var wg sync.WaitGroup
	rcs := make([]int, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each goroutine gets its own writer so they don't race on
			// a shared bytes.Buffer. We don't inspect their outputs —
			// only the final filesystem state matters.
			w, _, _ := newJSONWriter(t)
			rcs[i] = runInstallSkill(context.Background(), globalContext{w: w}, []string{"claude"})
		}()
	}
	wg.Wait()

	// Every invocation must exit 0. The acceptable outcomes are:
	//   - the first writer freshly installs (BVS-E-1) → rc=0
	//   - subsequent writers see the matching hash and short-circuit
	//     to "already_current" (BVS-E-2) → rc=0
	// Any non-zero rc indicates a race-induced corruption (e.g. a
	// reader saw a partial frontmatter mid-rename and mis-detected
	// drift).
	for i, rc := range rcs {
		if rc != 0 {
			t.Errorf("goroutine %d: rc=%d (BVS-S-2: all racing installs of the same hash must succeed)", i, rc)
		}
	}

	// Final SKILL.md must byte-equal the canonical post-install bytes.
	got, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	want := stampVersion(embeddedSkillBytes, skillVersionHash(embeddedSkillBytes))
	if string(got) != string(want) {
		t.Errorf("BVS-S-2: final SKILL.md bytes diverge from canonical post-install bytes\n"+
			"got len=%d, want len=%d", len(got), len(want))
	}

	// No stray .tmp.* siblings. BVS-S-1 + BVS-E-4 require an aborted
	// install to leave nothing behind; the atomic-rename pattern means
	// a SUCCESSFUL race also leaves nothing behind (the loser's tmp is
	// renamed away into the dst, then potentially overwritten by the
	// next winner via another rename — there is never a stray tmp).
	tmps, err := filepath.Glob(filepath.Join(dir, "SKILL.md.tmp.*"))
	if err != nil {
		t.Fatalf("glob tmp siblings: %v", err)
	}
	if len(tmps) != 0 {
		t.Errorf("BVS-S-2: stray tmp siblings remain after concurrent install: %v", tmps)
	}
}

// ---- AC-24: structured log emission per spec §9.4 ----

// runInstallHuman runs runInstallSkill with a HUMAN-mode writer so the
// Status sink (which carries the install-skill.* event lines) is
// written to the captured stderr buffer. The JSON-mode writer used by
// runInstall() suppresses Status by design — see output.Writer.Status.
func runInstallHuman(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	w, stdout, stderr := newHumanWriter(t)
	rc := runInstallSkill(context.Background(), globalContext{w: w}, args)
	return rc, stdout.String(), stderr.String()
}

// TestRunInstallSkill_LogEvents pins spec §9.4: the CLI emits
// install-skill.<event>{...} lines for every state transition. Each
// subtest exercises one path and asserts the expected event strings
// appear in stderr in order.
//
// Spec §9.4 lists `install-skill.uninstalled{agent}` only; the
// implementation also includes a `path` field. See the comment on
// logInstallSkill in cmd_install_skill.go — the spec is treated as a
// strict-subset contract (extra fields permitted).
//
// Mutation contract: removing a logInstallSkill / logInstallSkillError
// call from any of the four code paths below MUST fail this test.
func TestRunInstallSkill_LogEvents(t *testing.T) {
	t.Run("happy_install", func(t *testing.T) {
		home := setupTempHome(t)
		rc, _, stderr := runInstallHuman(t, "claude")
		if rc != 0 {
			t.Fatalf("rc: %d  stderr=%s", rc, stderr)
		}
		skillPath := claudeSkillPath(home)
		assertLogOrder(t, stderr,
			"install-skill.started{agent=claude}",
			"install-skill.completed{agent=claude, path="+skillPath+"}",
		)
	})

	t.Run("drift_refusal", func(t *testing.T) {
		home := setupTempHome(t)
		// Seed an install, then mutate the version line to force drift.
		if rc, _, _ := runInstall(t, "claude"); rc != 0 {
			t.Fatal("setup install failed")
		}
		skillPath := claudeSkillPath(home)
		cur, err := os.ReadFile(skillPath)
		if err != nil {
			t.Fatal(err)
		}
		embeddedHash := skillVersionHash(embeddedSkillBytes)
		mutated := strings.Replace(string(cur), "bv-skill-version: "+embeddedHash, "bv-skill-version: deadbeef0000", 1)
		if err := os.WriteFile(skillPath, []byte(mutated), 0o644); err != nil {
			t.Fatal(err)
		}

		rc, _, stderr := runInstallHuman(t, "claude")
		if rc == 0 {
			t.Fatalf("expected non-zero rc on drift; got 0  stderr=%s", stderr)
		}
		assertLogOrder(t, stderr,
			"install-skill.started{agent=claude}",
			"install-skill.error{agent=claude, code=DRIFT, path="+skillPath+"}",
		)
	})

	t.Run("uninstall", func(t *testing.T) {
		home := setupTempHome(t)
		if rc, _, _ := runInstall(t, "claude"); rc != 0 {
			t.Fatal("setup install failed")
		}
		skillPath := claudeSkillPath(home)
		rc, _, stderr := runInstallHuman(t, "claude", "--uninstall")
		if rc != 0 {
			t.Fatalf("uninstall rc: %d  stderr=%s", rc, stderr)
		}
		// Spec §9.4 says install-skill.uninstalled{agent}; we emit
		// {agent, path} (strict-subset contract — see logInstallSkill).
		assertLogOrder(t, stderr,
			"install-skill.started{agent=claude}",
			"install-skill.uninstalled{agent=claude, path="+skillPath+"}",
		)
	})

	t.Run("unconfigured_install", func(t *testing.T) {
		home := setupTempHomeUnconfigured(t)
		rc, _, stderr := runInstallHuman(t, "claude")
		if rc != 0 {
			t.Fatalf("unconfigured install should succeed; rc=%d stderr=%s", rc, stderr)
		}
		skillPath := claudeSkillPath(home)
		assertLogOrder(t, stderr,
			"install-skill.started{agent=claude}",
			"install-skill.completed{agent=claude, path="+skillPath+"}",
		)
	})
}

// assertLogOrder asserts every wanted substring appears in `out` AND
// they appear in the listed order (each occurrence comes after the
// previous match). Helps surface the exact missing line on failure
// rather than dumping the whole stderr.
func assertLogOrder(t *testing.T, out string, wants ...string) {
	t.Helper()
	cursor := 0
	for _, w := range wants {
		idx := strings.Index(out[cursor:], w)
		if idx < 0 {
			t.Errorf("missing log line %q after offset %d in stderr.\nstderr=%q", w, cursor, out)
			return
		}
		cursor += idx + len(w)
	}
}

// assertLogContains asserts each substring appears in the output (no
// ordering constraint). Use when the spec doesn't pin order.
func assertLogContains(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("missing log line %q in stderr.\nstderr=%q", w, out)
		}
	}
}

// ---- helpers ----

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return b
}
