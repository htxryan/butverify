// Tests for `bv login` (pebble-14wf).
//
// Coverage focus:
//
//   - Token source priority across all five resolveGHToken paths. The
//     gh-CLI subprocess case is exercised end-to-end via PATH override
//     (a temp dir holding a fake `gh` script) so we test the real
//     exec.CommandContext code path rather than mocking the wrapper.
//   - 401 install_url branch maps to exit 4 with the install hint.
//   - 401 without install_url (suspended tenant) maps to exit 4 with
//     the server message.
//   - Happy path persists the config and prints the JSON envelope.
//   - The TTY-prompt branch never fires under piped stdin (regression
//     guard: a CI run must not block waiting for human input).
//   - Cohesion: `login` is wired into main.go's switch so dispatch
//     does not silently fall through to the unknown-command branch.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/htxryan/butverify/internal/config"
)

// withCleanGHEnv unsets GH_TOKEN/GITHUB_TOKEN/BV_TOKEN for the duration
// of the test so a developer's ambient env never bleeds into a
// token-source assertion. t.Setenv handles restore-on-cleanup.
func withCleanGHEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("BV_TOKEN", "")
}

// ---------------- resolveGHToken priority ----------------

func TestResolveGHToken_FlagWinsOverEverything(t *testing.T) {
	withCleanGHEnv(t)
	t.Setenv("GH_TOKEN", "env_gh_token")
	t.Setenv("GITHUB_TOKEN", "env_github_token")
	got := resolveGHToken(context.Background(), "flag_token")
	if got != "flag_token" {
		t.Errorf("flag should win, got %q", got)
	}
}

func TestResolveGHToken_GHTokenEnvBeatsGitHubToken(t *testing.T) {
	withCleanGHEnv(t)
	t.Setenv("GH_TOKEN", "env_gh_token")
	t.Setenv("GITHUB_TOKEN", "env_github_token")
	got := resolveGHToken(context.Background(), "")
	if got != "env_gh_token" {
		t.Errorf("GH_TOKEN should win over GITHUB_TOKEN, got %q", got)
	}
}

func TestResolveGHToken_GitHubTokenWhenGHTokenAbsent(t *testing.T) {
	withCleanGHEnv(t)
	t.Setenv("GITHUB_TOKEN", "env_github_token")
	got := resolveGHToken(context.Background(), "")
	if got != "env_github_token" {
		t.Errorf("GITHUB_TOKEN should fall through, got %q", got)
	}
}

// TestResolveGHToken_GHCLIFallback puts a fake `gh` on PATH ahead of the
// real one and verifies we shell out to it when no env / flag wins. The
// test exercises the real exec.CommandContext path — no mock seam.
func TestResolveGHToken_GHCLIFallback(t *testing.T) {
	withCleanGHEnv(t)
	// Stop the TTY prompt from firing if the gh path somehow returns "".
	prev := stdinIsTTYFn
	stdinIsTTYFn = func() bool { return false }
	t.Cleanup(func() { stdinIsTTYFn = prev })

	dir := t.TempDir()
	writeFakeGH(t, dir, "gho_fake_from_gh_cli\n", 0)
	prependPATH(t, dir)

	got := resolveGHToken(context.Background(), "")
	if got != "gho_fake_from_gh_cli" {
		t.Errorf("expected gh-cli token, got %q", got)
	}
}

// TestResolveGHToken_GHCLIFailureFallsThrough simulates `gh auth token`
// returning non-zero (e.g. user not signed in). The token resolver MUST
// degrade to the next source rather than surfacing the gh error.
func TestResolveGHToken_GHCLIFailureFallsThrough(t *testing.T) {
	withCleanGHEnv(t)
	prev := stdinIsTTYFn
	stdinIsTTYFn = func() bool { return false } // suppress prompt
	t.Cleanup(func() { stdinIsTTYFn = prev })

	dir := t.TempDir()
	writeFakeGH(t, dir, "you are not logged in\n", 1) // exit 1
	prependPATH(t, dir)

	got := resolveGHToken(context.Background(), "")
	if got != "" {
		t.Errorf("gh failure should fall through to empty (no TTY), got %q", got)
	}
}

// TestResolveGHToken_NoTTYNoPrompt asserts the prompt branch is gated
// on os.Stdin being a character device. With stdinIsTTYFn forced false
// (i.e. piped stdin), the resolver returns "" instead of blocking on
// bufio.ReadString.
func TestResolveGHToken_NoTTYNoPrompt(t *testing.T) {
	withCleanGHEnv(t)
	dir := t.TempDir()
	writeFakeGH(t, dir, "", 1) // gh fails
	prependPATH(t, dir)

	prev := stdinIsTTYFn
	stdinIsTTYFn = func() bool { return false }
	t.Cleanup(func() { stdinIsTTYFn = prev })

	got := resolveGHToken(context.Background(), "")
	if got != "" {
		t.Errorf("piped stdin must not trigger the prompt, got %q", got)
	}
}

// ---------------- runLogin against a fake server ----------------

func TestRunLogin_HappyPathPersistsConfigAndEmitsJSON(t *testing.T) {
	withCleanGHEnv(t)
	t.Setenv("GH_TOKEN", "ghu_user_token")

	var sawAuth string
	var sawMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/login" {
			http.Error(w, `{"error":{"code":"NOT_FOUND","message":"no route"}}`, 404)
			return
		}
		sawMethod = r.Method
		sawAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":           "ghs_minted_installation_token",
			"expires_at":      "2026-05-01T11:00:00Z",
			"tenant_id":       "t_alice",
			"account_login":   "alice",
			"installation_id": 42,
			"account_type":    "User",
		})
	}))
	defer srv.Close()

	cfgPath := isolatedConfigPath(t)

	w, stdout, _ := newJSONWriter(t)
	g := globalContext{w: w, apiURLOverride: srv.URL}
	rc := runLogin(context.Background(), g, nil)
	if rc != 0 {
		t.Fatalf("rc=%d stdout=%s", rc, stdout.String())
	}
	if sawMethod != "POST" {
		t.Errorf("expected POST, got %q", sawMethod)
	}
	if sawAuth != "Bearer ghu_user_token" {
		t.Errorf("server saw Authorization=%q (CLI must forward the GH user token, not a CP token)", sawAuth)
	}

	// JSON envelope on stdout.
	out := stdout.String()
	for _, want := range []string{`"ok": true`, `"tenant_id": "t_alice"`, `"account_login": "alice"`, `"installation_id": 42`, `"expires_at": "2026-05-01T11:00:00Z"`} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q\nfull stdout: %s", want, out)
		}
	}

	// Config persisted with the minted (NOT the GH user) token.
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.InstallationToken != "ghs_minted_installation_token" {
		t.Errorf("config.installation_token=%q, want minted server token", loaded.InstallationToken)
	}
	if loaded.TenantID != "t_alice" || loaded.AccountLogin != "alice" || loaded.InstallationID != 42 {
		t.Errorf("config tenant fields wrong: %+v", loaded)
	}
	if loaded.APIURL != srv.URL {
		t.Errorf("config api_url=%q, want %q", loaded.APIURL, srv.URL)
	}
	if loaded.Mode != config.ModeRemote {
		t.Errorf("config mode=%q, want %q", loaded.Mode, config.ModeRemote)
	}

	// File mode must be 0600 (matches bv init; the token is a credential).
	st, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if mode := st.Mode().Perm(); mode != 0o600 {
		t.Errorf("config mode=%o, want 0600", mode)
	}
}

func TestRunLogin_401WithInstallURLPrintsHintExit4(t *testing.T) {
	withCleanGHEnv(t)
	t.Setenv("GH_TOKEN", "ghu_user_token")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "UNAUTHENTICATED",
				"message": "no tenant for this GitHub account; install the butverify GitHub App first",
				"details": map[string]any{
					"install_url": "https://github.com/apps/butverify/installations/new",
				},
			},
		})
	}))
	defer srv.Close()
	isolatedConfigPath(t)

	// Human mode so we can assert the friendly hint on stderr.
	w, _, stderr := newHumanWriter(t)
	g := globalContext{w: w, apiURLOverride: srv.URL}
	rc := runLogin(context.Background(), g, nil)
	if rc != 4 {
		t.Errorf("rc=%d, want 4", rc)
	}
	if !strings.Contains(stderr.String(), "Install the butverify GitHub App: https://github.com/apps/butverify/installations/new") {
		t.Errorf("stderr missing install hint:\n%s", stderr.String())
	}
}

func TestRunLogin_401WithoutInstallURLPrintsServerMessageExit4(t *testing.T) {
	withCleanGHEnv(t)
	t.Setenv("GH_TOKEN", "ghu_user_token")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "UNAUTHENTICATED",
				"message": "tenant suspended or uninstalled",
			},
		})
	}))
	defer srv.Close()
	isolatedConfigPath(t)

	w, _, stderr := newHumanWriter(t)
	g := globalContext{w: w, apiURLOverride: srv.URL}
	rc := runLogin(context.Background(), g, nil)
	if rc != 4 {
		t.Errorf("rc=%d, want 4", rc)
	}
	if !strings.Contains(stderr.String(), "tenant suspended or uninstalled") {
		t.Errorf("stderr missing server message:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "Install the butverify GitHub App") {
		t.Errorf("stderr should NOT contain install hint when install_url is absent")
	}
}

func TestRunLogin_NoTokenSourceFails(t *testing.T) {
	withCleanGHEnv(t)
	prev := stdinIsTTYFn
	stdinIsTTYFn = func() bool { return false }
	t.Cleanup(func() { stdinIsTTYFn = prev })

	dir := t.TempDir()
	writeFakeGH(t, dir, "", 1) // gh fails
	prependPATH(t, dir)

	isolatedConfigPath(t)
	w, stderr, _ := newJSONWriter(t)
	g := globalContext{w: w}
	rc := runLogin(context.Background(), g, nil)
	if rc != 2 {
		t.Errorf("rc=%d, want 2 (usage)", rc)
	}
	_ = stderr
}

// ---------------- direct-token path (replaces `bv init`) ----------------

// directTokenServer responds to GET /v1/auth/whoami with a fixed tenant
// payload, asserting the bearer is the installation token the caller
// supplied (NOT a GH user token). Returns the server + a pointer that
// will hold the bearer the server saw.
func directTokenServer(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/whoami" {
			http.Error(w, `{"error":{"code":"NOT_FOUND","message":"no route"}}`, 404)
			return
		}
		sawAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tenant_id":       "t_alice",
			"account_login":   "alice",
			"installation_id": 42,
			"account_type":    "User",
			"expires_at":      "2026-05-01T11:00:00Z",
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &sawAuth
}

func TestRunLogin_DirectTokenViaFlag_HitsWhoamiAndPersistsConfig(t *testing.T) {
	withCleanGHEnv(t)
	srv, sawAuth := directTokenServer(t)
	cfgPath := isolatedConfigPath(t)

	w, stdout, _ := newJSONWriter(t)
	g := globalContext{w: w, apiURLOverride: srv.URL}
	rc := runLogin(context.Background(), g, []string{"--token", "ghs_existing_installation_token"})
	if rc != 0 {
		t.Fatalf("rc=%d stdout=%s", rc, stdout.String())
	}
	if *sawAuth != "Bearer ghs_existing_installation_token" {
		t.Errorf("server saw Authorization=%q, want Bearer + the supplied installation token", *sawAuth)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.InstallationToken != "ghs_existing_installation_token" {
		t.Errorf("config installation_token=%q, want the token we passed (no exchange)", loaded.InstallationToken)
	}
	if loaded.TenantID != "t_alice" || loaded.AccountLogin != "alice" || loaded.InstallationID != 42 {
		t.Errorf("tenant fields wrong after whoami: %+v", loaded)
	}
	if loaded.Mode != config.ModeRemote {
		t.Errorf("config mode=%q, want %q", loaded.Mode, config.ModeRemote)
	}
	st, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if mode := st.Mode().Perm(); mode != 0o600 {
		t.Errorf("config mode=%o, want 0600", mode)
	}
}

func TestRunLogin_HumanOutputMentionsRemoteDefault(t *testing.T) {
	withCleanGHEnv(t)
	srv, _ := directTokenServer(t)
	isolatedConfigPath(t)
	w, stdout, _ := newHumanWriter(t)
	g := globalContext{w: w, apiURLOverride: srv.URL}
	rc := runLogin(context.Background(), g, []string{"--token", "ghs_existing_installation_token"})
	if rc != 0 {
		t.Fatalf("rc=%d stdout=%s", rc, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Default mode is now remote") {
		t.Fatalf("stdout missing mode change callout: %s", stdout.String())
	}
}

func TestRunLogin_DirectTokenViaGlobalOverride(t *testing.T) {
	withCleanGHEnv(t)
	srv, sawAuth := directTokenServer(t)
	isolatedConfigPath(t)

	w, _, _ := newJSONWriter(t)
	// Mirrors `bv --token X login`: main.go pre-parses --token into
	// globalContext.tokenOverride before dispatch.
	g := globalContext{w: w, apiURLOverride: srv.URL, tokenOverride: "ghs_global_override"}
	if rc := runLogin(context.Background(), g, nil); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if *sawAuth != "Bearer ghs_global_override" {
		t.Errorf("server saw %q, want Bearer ghs_global_override", *sawAuth)
	}
}

func TestRunLogin_DirectTokenViaBVTokenEnv(t *testing.T) {
	withCleanGHEnv(t)
	t.Setenv("BV_TOKEN", "ghs_from_env")
	srv, sawAuth := directTokenServer(t)
	isolatedConfigPath(t)

	w, _, _ := newJSONWriter(t)
	g := globalContext{w: w, apiURLOverride: srv.URL}
	if rc := runLogin(context.Background(), g, nil); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if *sawAuth != "Bearer ghs_from_env" {
		t.Errorf("server saw %q, want Bearer ghs_from_env", *sawAuth)
	}
}

func TestRunLogin_DirectTokenFlagWinsOverGHToken(t *testing.T) {
	// When BOTH --token and a GH source are present, the installation
	// token wins (skip the exchange). This is the "you already have a
	// fresh installation token, don't bother re-minting" case.
	withCleanGHEnv(t)
	t.Setenv("GH_TOKEN", "ghu_user_token") // would normally exchange
	srv, sawAuth := directTokenServer(t)
	isolatedConfigPath(t)

	w, _, _ := newJSONWriter(t)
	g := globalContext{w: w, apiURLOverride: srv.URL}
	rc := runLogin(context.Background(), g, []string{"--token", "ghs_direct"})
	if rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if *sawAuth != "Bearer ghs_direct" {
		t.Errorf("server saw %q, want Bearer ghs_direct (direct-token path must skip exchange)", *sawAuth)
	}
}

// TestRunLogin_PreservesExistingAPIURL asserts that running `bv login`
// without --api-url does NOT reset a previously configured non-default
// api_url back to the production default. This is the regression test for
// the "re-auth after token expiry overwrites dev api_url" bug.
func TestRunLogin_PreservesExistingAPIURL(t *testing.T) {
	withCleanGHEnv(t)

	// Pre-seed a config file with a non-default API URL (simulating a dev
	// installation). The test server will stand in for dev-api.butverify.dev.
	cfgPath := isolatedConfigPath(t)

	// We need to write the config AFTER isolatedConfigPath redirects the env
	// var, so config.Save lands at the temp path.
	preSeedCfg := &config.Config{
		APIURL:            "PLACEHOLDER", // filled in once srv is up
		InstallationToken: "ghs_old_expired_token",
		TenantID:          "t_alice",
		AccountLogin:      "alice",
		InstallationID:    42,
		Mode:              config.ModeRemote,
	}
	// Write a minimal JSON blob directly so we can set a fake URL before
	// the test server is running — we'll overwrite with the real srv.URL
	// after it starts.

	// Spin up the fake server that represents dev-api.butverify.dev.
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/login" {
			http.Error(w, `{"error":{"code":"NOT_FOUND","message":"no route"}}`, 404)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":           "ghs_fresh_token",
			"expires_at":      "2026-06-01T00:00:00Z",
			"tenant_id":       "t_alice",
			"account_login":   "alice",
			"installation_id": 42,
			"account_type":    "User",
		})
	}))
	defer srv.Close()
	srvURL = srv.URL

	// Write the pre-seeded config pointing at the test server URL.
	preSeedCfg.APIURL = srvURL
	if err := config.Save(preSeedCfg); err != nil {
		t.Fatalf("pre-seed config: %v", err)
	}

	// Verify the config file was written where we expect it.
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("pre-seeded config not at expected path %s: %v", cfgPath, err)
	}

	// Run login with GH_TOKEN but NO --api-url and NO apiURLOverride.
	t.Setenv("GH_TOKEN", "ghu_user_token")
	w, _, _ := newJSONWriter(t)
	g := globalContext{w: w} // no apiURLOverride — key to this test
	rc := runLogin(context.Background(), g, nil)
	if rc != 0 {
		t.Fatalf("rc=%d (login failed)", rc)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config after re-login: %v", err)
	}
	if loaded.APIURL != srvURL {
		t.Errorf("api_url after re-login = %q, want existing %q — bv login must not overwrite api_url when --api-url is not given", loaded.APIURL, srvURL)
	}
	if loaded.InstallationToken != "ghs_fresh_token" {
		t.Errorf("expected fresh token, got %q", loaded.InstallationToken)
	}
}

// ---------------- main.go cohesion ----------------

func TestLoginWiredIntoMainSwitch(t *testing.T) {
	mainPath := mainGoPath(t)
	raw, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	src := string(raw)
	if !strings.Contains(src, `case "login":`) {
		t.Error("main.go missing dispatch case for `bv login` — would fall through to unknown-command branch")
	}
	if !strings.Contains(src, "runLogin") {
		t.Error("main.go does not reference runLogin handler")
	}
}

func TestLoginListedInUsageText(t *testing.T) {
	if !strings.Contains(usageText, "login") {
		t.Error("usageText does not list `login` — `bv --help` will not surface it")
	}
}

// ---------------- helpers ----------------

// isolatedConfigPath redirects config.Path() to a temp file for the
// lifetime of the test. Returns the redirected path so callers can stat
// it (e.g. assert file mode).
func isolatedConfigPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	t.Setenv("BV_CONFIG_PATH", cfgPath)
	return cfgPath
}

// writeFakeGH writes a `gh` shell script to dir that prints `out` to
// stdout and exits with `code`. Used to exercise the gh-CLI subprocess
// branch via a PATH override — no mock seam in the production code.
func writeFakeGH(t *testing.T, dir, out string, code int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PATH-override fake gh script not supported on Windows")
	}
	// %q escapes embedded quotes/newlines safely for the shell.
	body := "#!/bin/sh\nprintf %s " + shellQuote(out) + "\nexit " + itoaInt(code) + "\n"
	path := filepath.Join(dir, "gh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
}

func prependPATH(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// shellQuote single-quotes a string for /bin/sh, escaping embedded
// single quotes by closing/opening the quoted region. Avoids importing
// strconv just for %q.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// itoaInt mirrors the cohesion_test helper but for arbitrary signed
// ints (the existing one only handles non-negative).
func itoaInt(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
