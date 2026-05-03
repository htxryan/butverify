package main

import (
	"context"
	"errors"

	"github.com/htxryan/butverify/internal/config"
)

func runLogout(_ context.Context, g globalContext, args []string) int {
	if len(args) != 0 {
		g.w.Error(toErrorEnvelope(errors.New("usage: bv logout")))
		return 2
	}
	c, err := loadModeConfig()
	if err != nil {
		return reportError(g.w, err)
	}
	c.InstallationToken = ""
	c.TenantID = ""
	c.AccountLogin = ""
	c.InstallationID = 0
	c.TokenExpiresAt = ""
	c.Mode = config.ModeLocal
	if err := config.Save(c); err != nil {
		return reportError(g.w, err)
	}
	if g.w.IsJSON() {
		_ = g.w.JSON(struct {
			OK   bool   `json:"ok"`
			Mode string `json:"mode"`
		}{true, config.ModeLocal})
		return 0
	}
	g.w.Human("Logged out")
	g.w.Human("Default mode is now local.")
	return 0
}
