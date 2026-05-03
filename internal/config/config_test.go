package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("BV_CONFIG_PATH", path)

	in := &Config{
		APIURL:            "https://api.example.test",
		InstallationToken: "ghs_test",
		TenantID:          "t_u42",
		AccountLogin:      "alice",
		InstallationID:    99,
		TokenExpiresAt:    "2026-04-27T10:00:00Z",
		Mode:              ModeRemote,
	}
	if err := Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("file mode should be 0600, got %o", st.Mode().Perm())
	}
	out, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if *out != *in {
		t.Errorf("roundtrip mismatch: %+v vs %+v", out, in)
	}
}

func TestLoadDefaultsModeLocal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("BV_CONFIG_PATH", path)
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Mode != ModeLocal {
		t.Errorf("Mode should default to %s, got %s", ModeLocal, c.Mode)
	}
}

func TestLoadDefaultsAuthenticatedLegacyConfigRemote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("BV_CONFIG_PATH", path)
	if err := os.WriteFile(path, []byte(`{"installation_token":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Mode != ModeRemote {
		t.Errorf("legacy authenticated config should default to %s, got %s", ModeRemote, c.Mode)
	}
}

func TestResolveMode(t *testing.T) {
	mode, err := ResolveMode(nil, "")
	if err != nil {
		t.Fatalf("ResolveMode: %v", err)
	}
	if mode != ModeLocal {
		t.Errorf("nil config mode=%q, want %q", mode, ModeLocal)
	}
	mode, err = ResolveMode(&Config{Mode: ModeRemote}, ModeLocal)
	if err != nil {
		t.Fatalf("ResolveMode override: %v", err)
	}
	if mode != ModeLocal {
		t.Errorf("override mode=%q, want %q", mode, ModeLocal)
	}
	if _, err := ResolveMode(nil, "bogus"); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestLoadNotInitialized(t *testing.T) {
	t.Setenv("BV_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))
	_, err := Load()
	if !errors.Is(err, ErrNotInitialized) {
		t.Errorf("expected ErrNotInitialized, got %v", err)
	}
}

func TestLoadDefaultsAPIURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("BV_CONFIG_PATH", path)
	if err := os.WriteFile(path, []byte(`{"installation_token":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.APIURL != DefaultAPIURL {
		t.Errorf("APIURL should default to %s, got %s", DefaultAPIURL, c.APIURL)
	}
}

func TestPathRespectsExplicitOverride(t *testing.T) {
	t.Setenv("BV_CONFIG_PATH", "/tmp/explicit/config.json")
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if p != "/tmp/explicit/config.json" {
		t.Errorf("Path: %s", p)
	}
}
