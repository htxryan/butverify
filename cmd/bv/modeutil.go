package main

import (
	"errors"

	"github.com/htxryan/butverify/internal/config"
)

func loadModeConfig() (*config.Config, error) {
	c, err := config.Load()
	if err == nil {
		return c, nil
	}
	if errors.Is(err, config.ErrNotInitialized) {
		return &config.Config{Mode: config.ModeLocal}, nil
	}
	return nil, err
}

func resolvedPublishMode(g globalContext, override string) (string, error) {
	if override == "" && g.tokenOverride != "" {
		return config.ModeRemote, nil
	}
	c, err := loadConfig(g)
	if err != nil {
		if errors.Is(err, config.ErrNotInitialized) {
			return config.ResolveMode(nil, override)
		}
		return "", err
	}
	return config.ResolveMode(c, override)
}
