// `bv login` exchanges a GitHub user token for a butverify installation
// token via POST /v1/auth/login, then persists the resulting config so
// subsequent commands authenticate as the resolved tenant.
//
// Token sources, tried in order, are:
//
//  1. --gh-token flag
//  2. GH_TOKEN env
//  3. GITHUB_TOKEN env
//  4. `gh auth token` subprocess (degrades silently if gh is missing
//     or not signed in)
//  5. Interactive TTY prompt (only when os.Stdin is a character device,
//     so a piped stdin never blocks waiting for input)
//
// The flag-or-env path lets CI shovel a token in without a TTY; the gh
// fallback matches what most contributors already have configured; the
// TTY prompt is the last resort for someone running `bv login` cold.

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/htxryan/butverify/internal/api"
	"github.com/htxryan/butverify/internal/config"
)

func runLogin(ctx context.Context, g globalContext, args []string) int {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	apiURL := fs.String("api-url", "", "control-plane base URL (defaults to https://api.butverify.dev)")
	ghTokenFlag := fs.String("gh-token", "", "GitHub user token (otherwise read from GH_TOKEN/GITHUB_TOKEN, gh CLI, or TTY prompt)")
	if err := fs.Parse(args); err != nil {
		g.w.Status("bv login: %v", err)
		return 2
	}

	ghToken := strings.TrimSpace(resolveGHToken(ctx, *ghTokenFlag))
	if ghToken == "" {
		g.w.Error(toErrorEnvelope(errors.New("no GitHub token available (use --gh-token, set GH_TOKEN/GITHUB_TOKEN, sign in with the gh CLI, or run interactively)")))
		return 2
	}

	url := *apiURL
	if url == "" {
		url = g.apiURLOverride
	}
	if url == "" {
		url = config.DefaultAPIURL
	}

	// IMPORTANT: send the GH user token as the bearer here — the server
	// route uses it to call api.github.com/user, then mints a fresh
	// installation token in the response body. Do NOT mix this up with
	// the bv installation token that other commands use.
	client := api.New(url, ghToken, Version)
	var resp api.LoginResponse
	if err := client.Do(ctx, "POST", "/v1/auth/login", nil, &resp); err != nil {
		return reportLoginError(g, err)
	}

	cfg := &config.Config{
		APIURL:            url,
		InstallationToken: resp.Token,
		TenantID:          resp.TenantID,
		AccountLogin:      resp.AccountLogin,
		InstallationID:    resp.InstallationID,
		TokenExpiresAt:    resp.ExpiresAt,
	}
	if err := config.Save(cfg); err != nil {
		return reportError(g.w, err)
	}

	if g.w.IsJSON() {
		_ = g.w.JSON(struct {
			OK             bool   `json:"ok"`
			TenantID       string `json:"tenant_id"`
			AccountLogin   string `json:"account_login"`
			InstallationID int64  `json:"installation_id"`
			ExpiresAt      string `json:"expires_at"`
		}{true, resp.TenantID, resp.AccountLogin, resp.InstallationID, resp.ExpiresAt})
		return 0
	}
	path, _ := config.Path()
	g.w.Human("Logged in as %s (tenant=%s)", resp.AccountLogin, resp.TenantID)
	g.w.Status("Config written to %s", path)
	return 0
}

// resolveGHToken walks the documented source priority and returns the
// first non-empty value. Each source is independent: a failure to read
// one (e.g. gh not installed) silently falls through to the next.
func resolveGHToken(ctx context.Context, flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := os.Getenv("GH_TOKEN"); v != "" {
		return v
	}
	if v := os.Getenv("GITHUB_TOKEN"); v != "" {
		return v
	}
	if v, err := ghAuthToken(ctx); err == nil && v != "" {
		return v
	}
	if isStdinTTY() {
		return promptForGHToken()
	}
	return ""
}

// ghAuthTokenCmd is a hook for tests that want to swap in a fake
// command. Production callers leave it nil so the real `gh auth token`
// subprocess runs.
var ghAuthTokenCmd func(ctx context.Context) *exec.Cmd

// ghAuthToken runs `gh auth token` and returns the trimmed stdout. Any
// error (gh missing, gh not signed in, non-zero exit) returns ("", err)
// so the caller can fall through to the next source.
func ghAuthToken(ctx context.Context) (string, error) {
	var cmd *exec.Cmd
	if ghAuthTokenCmd != nil {
		cmd = ghAuthTokenCmd(ctx)
	} else {
		cmd = exec.CommandContext(ctx, "gh", "auth", "token")
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// stdinIsTTYFn is overridable by tests; defaults to a real isatty probe.
var stdinIsTTYFn = realStdinIsTTY

func isStdinTTY() bool { return stdinIsTTYFn() }

func realStdinIsTTY() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func promptForGHToken() string {
	fmt.Fprint(os.Stderr, "GitHub token: ")
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}

// reportLoginError handles the bv login-specific 401 surface — the
// install_url branch needs a friendlier hint than the generic envelope —
// before falling back to the shared exit-code mapping.
func reportLoginError(g globalContext, err error) int {
	var ae *api.APIError
	if errors.As(err, &ae) && ae.Status == http.StatusUnauthorized && !g.w.IsJSON() {
		if installURL, ok := ae.Details["install_url"].(string); ok && installURL != "" {
			g.w.Status("Install the butverify GitHub App: %s", installURL)
			return 4
		}
		g.w.Status("%s", ae.Message)
		return 4
	}
	return reportError(g.w, err)
}
