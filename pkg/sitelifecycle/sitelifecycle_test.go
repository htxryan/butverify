package sitelifecycle

import (
	"testing"
	"time"
)

func TestSiteIDFormat(t *testing.T) {
	id, err := DeterministicSiteID("t_u42", "upload_abc")
	if err != nil {
		t.Fatal(err)
	}
	if !IsValidSiteID(id) {
		t.Fatalf("DeterministicSiteID returned invalid id: %q", id)
	}
	if len(id) != 8 {
		t.Fatalf("expected length 8, got %d (id=%q)", len(id), id)
	}
	first := id[0]
	if !(first >= 'a' && first <= 'z') {
		t.Fatalf("expected first char to be letter, got %q", first)
	}
}

func TestDeterministicSiteIDIsStable(t *testing.T) {
	a, _ := DeterministicSiteID("t_u42", "upload_abc")
	b, _ := DeterministicSiteID("t_u42", "upload_abc")
	if a != b {
		t.Fatalf("expected stable id, got %q and %q", a, b)
	}
	c, _ := DeterministicSiteID("t_u42", "upload_xyz")
	if a == c {
		t.Fatalf("expected different upload_id to yield different site_id")
	}
}

func TestDeterministicSiteIDValidatesInputs(t *testing.T) {
	if _, err := DeterministicSiteID("", "x"); err == nil {
		t.Fatal("expected error on empty tenant_id")
	}
	if _, err := DeterministicSiteID("t", ""); err == nil {
		t.Fatal("expected error on empty upload_id")
	}
}

func TestSiteIDRegexp(t *testing.T) {
	good := []string{"a23456", "abcdefgh", "abcdefghijkl", "z9z9z9z9"}
	for _, g := range good {
		if !IsValidSiteID(g) {
			t.Fatalf("expected %q to be valid", g)
		}
	}
	bad := []string{"", "1abcdef", "ab", "ABCDEFGH", "ab cdefgh", "abcdefghijklm"}
	for _, b := range bad {
		if IsValidSiteID(b) {
			t.Fatalf("expected %q to be invalid", b)
		}
	}
}

func TestIsReadable(t *testing.T) {
	cases := []struct {
		s    State
		want bool
	}{
		{State{Status: StatusActive, PaymentOK: true}, true},
		{State{Status: StatusPinned, PaymentOK: true}, true},
		{State{Status: StatusSuspended, PaymentOK: false}, true},
		{State{Status: StatusExpired, PaymentOK: true}, false},
		{State{Status: StatusCreating, PaymentOK: true}, false},
	}
	for _, tc := range cases {
		if got := IsReadable(tc.s); got != tc.want {
			t.Fatalf("IsReadable(%v)=%v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestIsWritable(t *testing.T) {
	cases := []struct {
		s    State
		want bool
	}{
		{State{Status: StatusActive, PaymentOK: true}, true},
		{State{Status: StatusActive, PaymentOK: false}, false},
		{State{Status: StatusPinned, PaymentOK: true}, true},
		{State{Status: StatusSuspended, PaymentOK: false}, false},
	}
	for _, tc := range cases {
		if got := IsWritable(tc.s); got != tc.want {
			t.Fatalf("IsWritable(%v)=%v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestIsExpired(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Second)
	future := now.Add(time.Minute)
	if !IsExpired(State{Status: StatusActive, ExpiresAt: &past, PaymentOK: true}, now) {
		t.Fatal("expected expired (past expires_at)")
	}
	if IsExpired(State{Status: StatusActive, ExpiresAt: &future, PaymentOK: true}, now) {
		t.Fatal("expected not expired (future expires_at)")
	}
	if !IsExpired(State{Status: StatusExpired, PaymentOK: true}, now) {
		t.Fatal("expected expired (status)")
	}
	if IsExpired(State{Status: StatusActive, ExpiresAt: nil, PaymentOK: true}, now) {
		t.Fatal("expected not expired (nil expires_at)")
	}
}
