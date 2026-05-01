// Package identity defines Principal types shared between the bv CLI and the
// control-plane Worker. The TS twin lives at packages/identity.
package identity

import (
	"errors"
	"time"
)

const (
	AccountTypeUser         = "User"
	AccountTypeOrganization = "Organization"
)

// TokenRefreshThreshold matches the TS shared-kernel threshold (E-2a).
const TokenRefreshThreshold = 10 * time.Minute

// Principal is the discriminated union of who/what is making a request.
type Principal struct {
	Kind  string // "human" | "agent_installation"
	Human *HumanIdentity
	Agent *AgentInstallation
}

type HumanIdentity struct {
	GitHubUserID int64
	GitHubLogin  string
	Email        string // optional
}

type AgentInstallation struct {
	InstallationID int64
	AccountLogin   string
	AccountID      int64
	AccountType    string // AccountTypeUser | AccountTypeOrganization
	TokenExpiresAt time.Time
}

type InstallationTokenClaims struct {
	InstallationID int64
	ExpiresAt      time.Time
	AccountID      int64
	AccountLogin   string
	AccountType    string
}

var ErrInvalidToken = errors.New("invalid installation token")

// ValidateClaims performs structural validation of token claims. Signature
// verification is the caller's responsibility; this only enforces the contract
// the rest of the codebase relies on.
func ValidateClaims(c InstallationTokenClaims) error {
	if c.InstallationID <= 0 {
		return errors.Join(ErrInvalidToken, errors.New("installation_id missing or invalid"))
	}
	if c.ExpiresAt.IsZero() {
		return errors.Join(ErrInvalidToken, errors.New("expires_at missing"))
	}
	if c.AccountID <= 0 {
		return errors.Join(ErrInvalidToken, errors.New("account_id missing or invalid"))
	}
	if c.AccountLogin == "" {
		return errors.Join(ErrInvalidToken, errors.New("account_login missing"))
	}
	if c.AccountType != AccountTypeUser && c.AccountType != AccountTypeOrganization {
		return errors.Join(ErrInvalidToken, errors.New("account_type must be User or Organization"))
	}
	return nil
}

// IsTokenExpired returns true if expires_at <= now.
func IsTokenExpired(c InstallationTokenClaims, now time.Time) bool {
	return !c.ExpiresAt.After(now)
}

// ShouldRefresh implements E-2a: refresh if the remaining lifetime is at or
// below TokenRefreshThreshold.
func ShouldRefresh(c InstallationTokenClaims, now time.Time) bool {
	return c.ExpiresAt.Sub(now) <= TokenRefreshThreshold
}

// ToAgentInstallation produces the public Principal-shaped struct from a set
// of validated claims.
func ToAgentInstallation(c InstallationTokenClaims) AgentInstallation {
	return AgentInstallation{
		InstallationID: c.InstallationID,
		AccountLogin:   c.AccountLogin,
		AccountID:      c.AccountID,
		AccountType:    c.AccountType,
		TokenExpiresAt: c.ExpiresAt,
	}
}
