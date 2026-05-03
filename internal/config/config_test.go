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
		ImageQuality:      82,
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

func TestResolveImageQuality(t *testing.T) {
	quality, err := ResolveImageQuality(nil, 0, 0)
	if err != nil {
		t.Fatalf("ResolveImageQuality default: %v", err)
	}
	if quality != DefaultImageQuality {
		t.Errorf("default quality=%d, want %d", quality, DefaultImageQuality)
	}
	quality, err = ResolveImageQuality(&Config{ImageQuality: 84}, 0, 0)
	if err != nil {
		t.Fatalf("ResolveImageQuality config: %v", err)
	}
	if quality != 84 {
		t.Errorf("config quality=%d, want 84", quality)
	}
	quality, err = ResolveImageQuality(&Config{ImageQuality: 84}, 62, 0)
	if err != nil {
		t.Fatalf("ResolveImageQuality override: %v", err)
	}
	if quality != 62 {
		t.Errorf("override quality=%d, want 62", quality)
	}
	quality, err = ResolveImageQuality(&Config{ImageQuality: 84}, 62, 50)
	if err != nil {
		t.Fatalf("ResolveImageQuality cap: %v", err)
	}
	if quality != 50 {
		t.Errorf("capped quality=%d, want 50", quality)
	}
	if _, err := ResolveImageQuality(nil, 101, 0); err == nil {
		t.Fatal("expected invalid override quality error")
	}
}

func TestLoadRejectsInvalidImageQuality(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("BV_CONFIG_PATH", path)
	if err := os.WriteFile(path, []byte(`{"image_quality":101}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid image_quality parse error")
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
