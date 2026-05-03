// Package config loads and persists the bv CLI's user-scoped configuration.
//
// The config is a JSON file at $XDG_CONFIG_HOME/butverify/config.json
// (default: ~/.config/butverify/config.json). It stores:
//
//   - api_url            — control-plane base URL (default api.butverify.dev)
//   - installation_token — the GitHub App installation token used as Bearer
//   - tenant_id          — captured from /v1/auth/whoami after `bv login`
//   - account_login      — captured from /v1/auth/whoami
//   - token_expires_at   — RFC 3339 timestamp; CLI refreshes when within
//     TokenRefreshThreshold of expiry (E-2a).
//   - mode               — default publish mode: local or remote.
//
// File mode is 0600 because the installation token is a credential. We do
// NOT store it in OS keychains at v1 — that's a future hardening pass; v1
// matches the convention of `gh auth login` (file-based) and `aws configure`
// (file-based, ~/.aws/credentials).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config is the persisted shape on disk.
type Config struct {
	APIURL            string `json:"api_url,omitempty"`
	InstallationToken string `json:"installation_token,omitempty"`
	TenantID          string `json:"tenant_id,omitempty"`
	AccountLogin      string `json:"account_login,omitempty"`
	InstallationID    int64  `json:"installation_id,omitempty"`
	TokenExpiresAt    string `json:"token_expires_at,omitempty"`
	Mode              string `json:"mode,omitempty"`
}

// DefaultAPIURL is the public control-plane endpoint.
const DefaultAPIURL = "https://api.butverify.dev"

const (
	ModeLocal  = "local"
	ModeRemote = "remote"
)

// ErrNotInitialized is returned by Load when no config exists. Callers
// surface this as "run `bv login` first."
var ErrNotInitialized = errors.New("config: not initialized; run `bv login`")

// Path returns the absolute path to the config file. Honors $XDG_CONFIG_HOME
// when set, otherwise ~/.config/butverify/config.json.
func Path() (string, error) {
	if explicit := os.Getenv("BV_CONFIG_PATH"); explicit != "" {
		return explicit, nil
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("config: locate home: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "butverify", "config.json"), nil
}

// Load reads the config from disk. Returns ErrNotInitialized if the file
// doesn't exist.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotInitialized
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if c.APIURL == "" {
		c.APIURL = DefaultAPIURL
	}
	if c.Mode == "" {
		if c.InstallationToken != "" {
			c.Mode = ModeRemote
		} else {
			c.Mode = ModeLocal
		}
	}
	if err := ValidateMode(c.Mode); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return &c, nil
}

func ValidateMode(mode string) error {
	switch mode {
	case ModeLocal, ModeRemote:
		return nil
	default:
		return fmt.Errorf("mode must be %q or %q", ModeLocal, ModeRemote)
	}
}

func ResolveMode(c *Config, override string) (string, error) {
	if override != "" {
		if err := ValidateMode(override); err != nil {
			return "", err
		}
		return override, nil
	}
	if c == nil || c.Mode == "" {
		return ModeLocal, nil
	}
	if err := ValidateMode(c.Mode); err != nil {
		return "", err
	}
	return c.Mode, nil
}

// Save writes the config atomically (write to a temp file, then rename).
// File mode is 0600 — the token is a credential.
func Save(c *Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("config: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("config: rename: %w", err)
	}
	return nil
}
