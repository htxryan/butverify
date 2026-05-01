// `bv init` captures an installation token and persists it to the config
// file, plus runs `whoami` against the API to verify the token resolves to
// a tenant. The token is sourced (in order) from --token, BV_TOKEN env, or
// stdin (so a shell script can `gh auth token | bv init`).

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"strings"

	"github.com/htxryan/butverify/internal/api"
	"github.com/htxryan/butverify/internal/config"
	"github.com/htxryan/butverify/internal/output"
)

func runInit(ctx context.Context, g globalContext, args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	apiURL := fs.String("api-url", "", "control-plane base URL (defaults to https://api.butverify.dev)")
	tokenFlag := fs.String("token", "", "installation token (otherwise read from BV_TOKEN or stdin)")
	if err := fs.Parse(args); err != nil {
		g.w.Status("bv init: %v", err)
		return 2
	}

	// Token source priority: --token > globalContext.tokenOverride > env > stdin.
	token := *tokenFlag
	if token == "" {
		token = g.tokenOverride
	}
	if token == "" {
		token = os.Getenv("BV_TOKEN")
	}
	if token == "" {
		token = readTokenFromStdin()
	}
	token = strings.TrimSpace(token)
	if token == "" {
		g.w.Error(toErrorEnvelope(errors.New("no installation token provided (use --token, BV_TOKEN env, or pipe to stdin)")))
		return 2
	}

	url := *apiURL
	if url == "" {
		url = g.apiURLOverride
	}
	if url == "" {
		url = config.DefaultAPIURL
	}

	client := api.New(url, token, Version)
	var who api.WhoamiResponse
	if err := client.Do(ctx, "GET", "/v1/auth/whoami", nil, &who); err != nil {
		return reportError(g.w, err)
	}

	cfg := &config.Config{
		APIURL:            url,
		InstallationToken: token,
		TenantID:          who.TenantID,
		AccountLogin:      who.AccountLogin,
		InstallationID:    who.InstallationID,
		TokenExpiresAt:    who.ExpiresAt,
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
			TokenExpiresAt string `json:"token_expires_at"`
		}{true, who.TenantID, who.AccountLogin, who.InstallationID, who.ExpiresAt})
		return 0
	}
	path, _ := config.Path()
	g.w.Human("Initialized as %s (tenant=%s)", who.AccountLogin, who.TenantID)
	g.w.Status("Config written to %s", path)
	return 0
}

func readTokenFromStdin() string {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return ""
	}
	// Only consume stdin if it's piped — interactive callers should use --token.
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return ""
	}
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}

func toErrorEnvelope(err error) output.ErrorEnvelope {
	return output.ErrorEnvelope{Message: err.Error(), Code: "BAD_REQUEST"}
}
