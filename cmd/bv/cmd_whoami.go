// `bv whoami` prints the resolved tenant for the configured token.

package main

import (
	"context"

	"github.com/htxryan/butverify/internal/api"
)

func runWhoami(ctx context.Context, g globalContext, args []string) int {
	_ = args
	client, _, err := newClient(g)
	if err != nil {
		return reportError(g.w, err)
	}
	var who api.WhoamiResponse
	if err := client.Do(ctx, "GET", "/v1/auth/whoami", nil, &who); err != nil {
		return reportError(g.w, err)
	}
	if g.w.IsJSON() {
		_ = g.w.JSON(who)
		return 0
	}
	g.w.Human("Tenant:        %s", who.TenantID)
	g.w.Human("Account:       %s (%s)", who.AccountLogin, who.AccountType)
	g.w.Human("Installation:  %d", who.InstallationID)
	g.w.Human("Token expires: %s", who.ExpiresAt)
	return 0
}
