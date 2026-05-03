package main

import (
	"context"
	"strings"
	"testing"

	"github.com/htxryan/butverify/internal/config"
)

func TestModeDefaultsLocalWithoutConfig(t *testing.T) {
	isolatedConfigPath(t)
	w, stdout, _ := newJSONWriter(t)
	rc := runMode(context.Background(), globalContext{w: w}, nil)
	if rc != 0 {
		t.Fatalf("rc=%d stdout=%s", rc, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"mode": "local"`) {
		t.Fatalf("stdout missing local mode: %s", stdout.String())
	}
}

func TestModeSetsDefault(t *testing.T) {
	isolatedConfigPath(t)
	w, stdout, _ := newJSONWriter(t)
	rc := runMode(context.Background(), globalContext{w: w}, []string{config.ModeRemote})
	if rc != 0 {
		t.Fatalf("rc=%d stdout=%s", rc, stdout.String())
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Mode != config.ModeRemote {
		t.Fatalf("mode=%q, want %q", loaded.Mode, config.ModeRemote)
	}
	if !strings.Contains(stdout.String(), `"changed": true`) {
		t.Fatalf("stdout missing changed=true: %s", stdout.String())
	}
}

func TestModeRejectsInvalid(t *testing.T) {
	isolatedConfigPath(t)
	w, _, _ := newJSONWriter(t)
	rc := runMode(context.Background(), globalContext{w: w}, []string{"bogus"})
	if rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

func TestLogoutClearsAuthAndSetsLocal(t *testing.T) {
	setupConfig(t, "https://api.example.test")
	w, stdout, _ := newJSONWriter(t)
	rc := runLogout(context.Background(), globalContext{w: w}, nil)
	if rc != 0 {
		t.Fatalf("rc=%d stdout=%s", rc, stdout.String())
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Mode != config.ModeLocal {
		t.Fatalf("mode=%q, want %q", loaded.Mode, config.ModeLocal)
	}
	if loaded.InstallationToken != "" || loaded.TenantID != "" || loaded.AccountLogin != "" || loaded.InstallationID != 0 || loaded.TokenExpiresAt != "" {
		t.Fatalf("auth fields not cleared: %+v", loaded)
	}
}
