// Hand-rolled duration humanizer for the EXPIRES column of `bv ls`.
//
// The bucket ladder is ported from github.com/dustin/go-humanize so the
// surface matches what users have seen elsewhere, but the dependency is
// not pulled in (the bv binary has a tight stripped-size budget — see
// cmd/bv/main.go). The phrase returned is the magnitude only — no "in"
// prefix, no "ago" suffix — because direction is conveyed by the
// absolute timestamp the column already shows.

package main

import (
	"fmt"
	"math"
	"time"
)

// humanizeDuration returns a human-readable magnitude phrase for d. The
// sign of d is ignored: a -2h duration produces "2 hours", not
// "2 hours ago". The bucket boundaries match the dustin/go-humanize
// time scale.
func humanizeDuration(d time.Duration) string {
	diff := math.Abs(d.Seconds())
	switch {
	case diff < 30:
		return "less than a minute"
	case diff < 90:
		return "one minute"
	case diff < 2700:
		return pluralizeUnit(int(math.Round(diff/60)), "minute")
	case diff < 5400:
		return "one hour"
	case diff < 79200:
		return pluralizeUnit(int(math.Round(diff/3600)), "hour")
	case diff < 129600:
		return "one day"
	case diff < 2160000:
		return pluralizeUnit(int(math.Round(diff/86400)), "day")
	case diff < 3888000:
		return "one month"
	case diff < 28512000:
		return pluralizeUnit(int(math.Round(diff/2592000)), "month")
	case diff < 46656000:
		return "one year"
	default:
		return pluralizeUnit(int(math.Round(diff/31536000)), "year")
	}
}

// pluralizeUnit renders "one <unit>" for n == 1 (matching the user-given
// example "one hour") and "<n> <unit>s" otherwise.
func pluralizeUnit(n int, unit string) string {
	if n == 1 {
		return "one " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// formatExpires renders the EXPIRES column cell for a SiteSummary's
// ExpiresAt. now is taken as a parameter so tests can pin a reference
// time and avoid wall-clock flakiness.
//
//   - "" (no expiry — e.g. pinned sites): "—"
//   - malformed (non-RFC3339): the raw string verbatim, as a defensive
//     fallback. The server always emits RFC3339, so this should not
//     happen in practice.
//   - otherwise: "YYYY-MM-DD HH:MM AM/PM (humanized)" in local time.
func formatExpires(raw string, now time.Time) string {
	if raw == "" {
		return "—"
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	local := t.Local()
	diff := t.Sub(now)
	return fmt.Sprintf("%s (%s)", local.Format("2006-01-02 03:04 PM"), humanizeDuration(diff))
}
