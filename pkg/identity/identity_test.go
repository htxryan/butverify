package identity

import (
	"errors"
	"testing"
	"time"
)

func validClaims() InstallationTokenClaims {
	return InstallationTokenClaims{
		InstallationID: 99,
		ExpiresAt:      time.Now().Add(30 * time.Minute),
		AccountID:      42,
		AccountLogin:   "acme",
		AccountType:    AccountTypeOrganization,
	}
}

func TestValidateClaims_Valid(t *testing.T) {
	if err := ValidateClaims(validClaims()); err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
}

func TestValidateClaims_BadInstallationID(t *testing.T) {
	c := validClaims()
	c.InstallationID = 0
	err := ValidateClaims(c)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestValidateClaims_BadAccountType(t *testing.T) {
	c := validClaims()
	c.AccountType = "Robot"
	if err := ValidateClaims(c); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestValidateClaims_MissingExpiresAt(t *testing.T) {
	c := validClaims()
	c.ExpiresAt = time.Time{}
	if err := ValidateClaims(c); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestIsTokenExpired(t *testing.T) {
	now := time.Now()
	c := InstallationTokenClaims{ExpiresAt: now.Add(-time.Second)}
	if !IsTokenExpired(c, now) {
		t.Fatalf("expected expired")
	}
	c.ExpiresAt = now.Add(time.Minute)
	if IsTokenExpired(c, now) {
		t.Fatalf("expected not expired")
	}
}

func TestShouldRefresh(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name    string
		expires time.Duration
		want    bool
	}{
		{"5m left → refresh", 5 * time.Minute, true},
		{"10m left → refresh (boundary)", 10 * time.Minute, true},
		{"30m left → no refresh", 30 * time.Minute, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := InstallationTokenClaims{ExpiresAt: now.Add(tc.expires)}
			if got := ShouldRefresh(c, now); got != tc.want {
				t.Fatalf("ShouldRefresh = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestToAgentInstallation(t *testing.T) {
	c := validClaims()
	a := ToAgentInstallation(c)
	if a.InstallationID != c.InstallationID {
		t.Fatalf("installation_id mismatch")
	}
	if a.AccountID != c.AccountID || a.AccountLogin != c.AccountLogin {
		t.Fatalf("account fields mismatch")
	}
}
