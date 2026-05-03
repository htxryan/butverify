// Shared helper for building an API client + handling errors uniformly across
// subcommands.

package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/htxryan/butverify/internal/api"
	"github.com/htxryan/butverify/internal/config"
	"github.com/htxryan/butverify/internal/output"
	"github.com/htxryan/butverify/pkg/identity"
)

// loadConfig loads the persisted config and applies any command-line
// overrides for api_url / token. Returns ErrNotInitialized if no config
// exists and the override didn't supply an inline token (login bypasses).
func loadConfig(g globalContext) (*config.Config, error) {
	c, err := config.Load()
	if err != nil {
		if errors.Is(err, config.ErrNotInitialized) {
			// If both override flags are present, synthesize a config so the
			// command can still run (e.g. in CI without `bv login`).
			if g.apiURLOverride != "" && g.tokenOverride != "" {
				return &config.Config{APIURL: g.apiURLOverride, InstallationToken: g.tokenOverride, Mode: config.ModeRemote}, nil
			}
			return nil, err
		}
		return nil, err
	}
	if g.apiURLOverride != "" {
		c.APIURL = g.apiURLOverride
	}
	if g.tokenOverride != "" {
		c.InstallationToken = g.tokenOverride
	}
	return c, nil
}

// newClient builds an API client from the loaded config + global overrides.
// Used by every command except `login` (which builds its own client without
// requiring an existing config).
func newClient(g globalContext) (*api.Client, *config.Config, error) {
	c, err := loadConfig(g)
	if err != nil {
		return nil, nil, err
	}
	if c.APIURL == "" {
		c.APIURL = config.DefaultAPIURL
	}
	if c.InstallationToken == "" {
		return nil, nil, errors.New("no installation token configured; run `bv login` or pass --token")
	}
	return api.New(c.APIURL, c.InstallationToken, Version), c, nil
}

func canAutoRefreshToken(g globalContext, c *config.Config) bool {
	return g.tokenOverride == "" && c != nil && c.InstallationToken != ""
}

func shouldRefreshToken(c *config.Config, now time.Time) bool {
	if c.TokenExpiresAt == "" {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339, c.TokenExpiresAt)
	if err != nil {
		return false
	}
	return identity.ShouldRefresh(identity.InstallationTokenClaims{ExpiresAt: expiresAt}, now)
}

func refreshInstallationToken(ctx context.Context, g globalContext, c *config.Config) (*api.Client, error) {
	ghToken := strings.TrimSpace(resolveAutomaticGHToken(ctx))
	if ghToken == "" {
		return nil, errors.New("installation token expired and no GitHub token is available for automatic refresh; run `bv login`")
	}

	refreshClient := api.New(c.APIURL, ghToken, Version)
	var resp api.LoginResponse
	if err := refreshClient.Do(ctx, "POST", "/v1/auth/login", nil, &resp); err != nil {
		return nil, err
	}

	c.InstallationToken = resp.Token
	c.TenantID = resp.TenantID
	c.AccountLogin = resp.AccountLogin
	c.InstallationID = resp.InstallationID
	c.TokenExpiresAt = resp.ExpiresAt
	if err := config.Save(c); err != nil {
		return nil, err
	}
	g.w.Status("Refreshed auth token for %s", resp.AccountLogin)
	return api.New(c.APIURL, c.InstallationToken, Version), nil
}

// toErrorEnvelope wraps a plain error in the structured envelope shape
// the JSON-output mode emits. Lives here (in the shared utilities)
// rather than in any one command because every subcommand uses it.
func toErrorEnvelope(err error) output.ErrorEnvelope {
	return output.ErrorEnvelope{Message: err.Error(), Code: "BAD_REQUEST"}
}

// reportError prints the error in the writer's mode and returns a process
// exit code. Maps API error classes to deterministic codes so a calling
// shell script can branch:
//
//	0  — success
//	1  — generic error
//	2  — usage error
//	3  — config / not-initialized
//	4  — auth (401)
//	5  — payment_required (402)
//	6  — not_found (404)
//	7  — conflict (409)
//	8  — rate_limited (429)
//	9  — upgrade_required (426)
func reportError(w *output.Writer, err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, config.ErrNotInitialized) {
		w.Error(output.ErrorEnvelope{Code: "NOT_INITIALIZED", Message: err.Error()})
		return 3
	}
	var ae *api.APIError
	if errors.As(err, &ae) {
		w.Error(output.ErrorEnvelope{
			Code:      ae.Code,
			Message:   ae.Message,
			RequestID: ae.RequestID,
			Status:    ae.Status,
		})
		switch ae.Status {
		case http.StatusUnauthorized:
			return 4
		case http.StatusPaymentRequired:
			return 5
		case http.StatusNotFound:
			return 6
		case http.StatusConflict:
			return 7
		case http.StatusTooManyRequests:
			return 8
		case http.StatusUpgradeRequired:
			return 9
		}
		return 1
	}
	w.Error(output.ErrorEnvelope{Code: "ERROR", Message: err.Error()})
	return 1
}
