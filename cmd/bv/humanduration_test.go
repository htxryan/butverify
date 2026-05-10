package main

import (
	"testing"
	"time"
)

// TestHumanizeDuration walks every bucket boundary in the ladder, just
// below and at the boundary, plus a few mid-bucket samples and a
// negative-duration symmetry case.
func TestHumanizeDuration(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
		want string
	}{
		// less than a minute
		{"zero", 0, "less than a minute"},
		{"29s", 29 * time.Second, "less than a minute"},
		// one minute
		{"30s", 30 * time.Second, "one minute"},
		{"60s", 60 * time.Second, "one minute"},
		{"89s", 89 * time.Second, "one minute"},
		// N minutes
		{"90s", 90 * time.Second, "2 minutes"},
		{"5m", 5 * time.Minute, "5 minutes"},
		{"44m59s", 44*time.Minute + 59*time.Second, "45 minutes"},
		// one hour
		{"45m", 45 * time.Minute, "one hour"},
		{"1h", time.Hour, "one hour"},
		{"89m59s", 89*time.Minute + 59*time.Second, "one hour"},
		// N hours
		{"90m", 90 * time.Minute, "2 hours"},
		{"2h", 2 * time.Hour, "2 hours"},
		{"21h59m", 21*time.Hour + 59*time.Minute, "22 hours"},
		// one day
		{"22h", 22 * time.Hour, "one day"},
		{"35h59m", 35*time.Hour + 59*time.Minute, "one day"},
		// N days
		{"36h", 36 * time.Hour, "2 days"},
		{"5d", 5 * 24 * time.Hour, "5 days"},
		{"24d23h", 24*24*time.Hour + 23*time.Hour, "25 days"},
		// one month
		{"25d", 25 * 24 * time.Hour, "one month"},
		{"44d", 44 * 24 * time.Hour, "one month"},
		// N months
		{"45d", 45 * 24 * time.Hour, "2 months"},
		{"6mo", 6 * 30 * 24 * time.Hour, "6 months"},
		// one year
		{"11mo", 11 * 30 * 24 * time.Hour, "one year"},
		// N years (note: 18mo rounds to 1 year via the year divisor;
		// stepping past it with 19mo lands at 2 years)
		{"19mo", 19 * 30 * 24 * time.Hour, "2 years"},
		{"5y", 5 * 365 * 24 * time.Hour, "5 years"},
		// negative-duration symmetry
		{"-2h", -2 * time.Hour, "2 hours"},
		{"-30s", -30 * time.Second, "one minute"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := humanizeDuration(c.d)
			if got != c.want {
				t.Fatalf("humanizeDuration(%s) = %q, want %q", c.d, got, c.want)
			}
		})
	}
}

func TestFormatExpiresEmpty(t *testing.T) {
	now := time.Date(2026, 5, 9, 9, 38, 0, 0, time.UTC)
	if got := formatExpires("", now); got != "—" {
		t.Fatalf("formatExpires(\"\") = %q, want %q", got, "—")
	}
}

func TestFormatExpiresMalformed(t *testing.T) {
	now := time.Date(2026, 5, 9, 9, 38, 0, 0, time.UTC)
	raw := "not-a-date"
	if got := formatExpires(raw, now); got != raw {
		t.Fatalf("formatExpires(%q) = %q, want %q", raw, got, raw)
	}
}

// TestFormatExpiresFuture pins now and an expires_at one hour in the
// future and asserts the magnitude phrase is "one hour". The exact
// timestamp prefix depends on the runner's local timezone, so this
// test asserts on the magnitude phrase only.
func TestFormatExpiresFuture(t *testing.T) {
	now := time.Date(2026, 5, 9, 9, 38, 0, 0, time.UTC)
	expires := now.Add(time.Hour).Format(time.RFC3339)
	got := formatExpires(expires, now)
	if !contains(got, "(one hour)") {
		t.Fatalf("formatExpires future expected substring %q, got %q", "(one hour)", got)
	}
}

// TestFormatExpiresPast confirms a past expiry produces a magnitude
// phrase WITHOUT an "ago" suffix; direction is conveyed by the
// absolute timestamp the column already shows.
func TestFormatExpiresPast(t *testing.T) {
	now := time.Date(2026, 5, 9, 9, 38, 0, 0, time.UTC)
	expires := now.Add(-2 * time.Hour).Format(time.RFC3339)
	got := formatExpires(expires, now)
	if !contains(got, "(2 hours)") {
		t.Fatalf("formatExpires past expected substring %q, got %q", "(2 hours)", got)
	}
	if contains(got, "ago") {
		t.Fatalf("formatExpires past must not include 'ago', got %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
