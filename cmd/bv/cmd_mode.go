package main

import (
	"context"
	"errors"

	"github.com/htxryan/butverify/internal/config"
)

func runMode(_ context.Context, g globalContext, args []string) int {
	if len(args) > 1 {
		g.w.Error(toErrorEnvelope(errors.New("usage: bv mode [local|remote]")))
		return 2
	}
	c, err := loadModeConfig()
	if err != nil {
		return reportError(g.w, err)
	}
	if len(args) == 0 {
		mode, err := config.ResolveMode(c, "")
		if err != nil {
			return reportError(g.w, err)
		}
		writeModeResult(g, mode, false)
		return 0
	}
	mode := args[0]
	if err := config.ValidateMode(mode); err != nil {
		g.w.Error(toErrorEnvelope(err))
		return 2
	}
	c.Mode = mode
	if err := config.Save(c); err != nil {
		return reportError(g.w, err)
	}
	writeModeResult(g, mode, true)
	return 0
}

func writeModeResult(g globalContext, mode string, changed bool) {
	if g.w.IsJSON() {
		_ = g.w.JSON(struct {
			Mode    string `json:"mode"`
			Changed bool   `json:"changed"`
		}{mode, changed})
		return
	}
	if changed {
		g.w.Human("Default mode set to %s", mode)
		return
	}
	g.w.Human("%s", mode)
}
