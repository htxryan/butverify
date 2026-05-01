// Package sitelifecycle is the Go twin of @butverify/sitelifecycle. The CLI
// only needs the read predicates (isReadable / isWritable) for client-side
// hints; authoritative state transitions live in the control-plane Worker.
package sitelifecycle

import (
	"crypto/sha256"
	"errors"
	"regexp"
	"time"
)

// Crockford base32 alphabet (i, l, o, u removed). First-char subset omits
// digits so the resulting ID is always a valid DNS label; positions 2..N
// draw from the full alphabet so the regex (U-7) and generator agree on
// what characters can appear at each position.
const (
	siteIDAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"
	siteIDLetters  = "abcdefghjkmnpqrstvwxyz"
	siteIDLen      = 8
)

// SiteIDRegexp mirrors the TS twin (^[a-z][a-z0-9]{5,11}$, U-7).
var SiteIDRegexp = regexp.MustCompile(`^[a-z][a-z0-9]{5,11}$`)

func IsValidSiteID(s string) bool { return SiteIDRegexp.MatchString(s) }

// DeterministicSiteID is the SHA-256-based derivation of a site_id from the
// (tenant_id, upload_id) pair. The CLI uses this only for client-side
// progress display (e.g. "your site will be at <id>.butverify.dev"); the
// control plane is the authoritative source.
func DeterministicSiteID(tenantID, uploadID string) (string, error) {
	if tenantID == "" {
		return "", errors.New("tenant_id required")
	}
	if uploadID == "" {
		return "", errors.New("upload_id required")
	}
	sum := sha256.Sum256([]byte(tenantID + ":" + uploadID))
	out := make([]byte, siteIDLen)
	out[0] = siteIDLetters[int(sum[0])%len(siteIDLetters)]
	for i := 1; i < siteIDLen; i++ {
		out[i] = siteIDAlphabet[int(sum[i])%len(siteIDAlphabet)]
	}
	return string(out), nil
}

type Status string

const (
	StatusCreating  Status = "creating"
	StatusUploading Status = "uploading"
	StatusActive    Status = "active"
	StatusPinned    Status = "pinned"
	StatusExpiring  Status = "expiring"
	StatusExpired   Status = "expired"
	StatusSuspended Status = "suspended"
	StatusFailed    Status = "failed"
)

type State struct {
	Status    Status
	ExpiresAt *time.Time
	PaymentOK bool
}

func IsReadable(s State) bool {
	switch s.Status {
	case StatusActive, StatusPinned, StatusSuspended:
		return true
	default:
		return false
	}
}

func IsWritable(s State) bool {
	if !s.PaymentOK {
		return false
	}
	switch s.Status {
	case StatusActive, StatusPinned:
		return true
	default:
		return false
	}
}

func IsExpired(s State, now time.Time) bool {
	if s.Status == StatusExpired {
		return true
	}
	if s.ExpiresAt == nil {
		return false
	}
	return !s.ExpiresAt.After(now)
}
