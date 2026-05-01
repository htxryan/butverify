package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ghs_test" {
			t.Errorf("missing or wrong auth header: %q", r.Header.Get("Authorization"))
		}
		ua := r.Header.Get("User-Agent")
		if !strings.HasPrefix(ua, "bv/test ") {
			t.Errorf("User-Agent should start with bv/test: %q", ua)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "echo": r.URL.Path})
	}))
	defer server.Close()

	c := New(server.URL, "ghs_test", "test")
	var out map[string]any
	if err := c.Do(context.Background(), "GET", "/v1/auth/whoami", nil, &out); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if out["ok"] != true {
		t.Errorf("unexpected response: %+v", out)
	}
}

func TestDoApiError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":       "PAYMENT_REQUIRED",
				"message":    "quota exceeded",
				"request_id": "req_abc",
			},
		})
	}))
	defer server.Close()

	c := New(server.URL, "tok", "test")
	err := c.Do(context.Background(), "POST", "/v1/sites", map[string]any{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if ae.Status != 402 || ae.Code != "PAYMENT_REQUIRED" {
		t.Errorf("unexpected: %+v", ae)
	}
	if ae.RequestID != "req_abc" {
		t.Errorf("request_id missing: %+v", ae)
	}
	if !IsPaymentRequired(err) {
		t.Error("IsPaymentRequired should be true")
	}
}

func TestDoUpgradeRequired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUpgradeRequired)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "UPGRADE_REQUIRED",
				"message": "upgrade",
				"details": map[string]any{
					"min_version":     "1.0.0",
					"current_version": "0.1.0",
					"download_url":    "https://dl.butverify.dev/cli/stable/manifest.json",
				},
			},
		})
	}))
	defer server.Close()
	c := New(server.URL, "tok", "test")
	err := c.Do(context.Background(), "GET", "/v1/auth/whoami", nil, nil)
	if !IsUpgradeRequired(err) {
		t.Fatalf("expected UpgradeRequired, got %v", err)
	}
	var ae *APIError
	_ = errors.As(err, &ae)
	if ae.Details["min_version"] != "1.0.0" {
		t.Errorf("min_version missing: %+v", ae.Details)
	}
}

func TestDoNonJSONError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream timeout"))
	}))
	defer server.Close()
	c := New(server.URL, "tok", "test")
	err := c.Do(context.Background(), "GET", "/foo", nil, nil)
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if ae.Status != 502 {
		t.Errorf("status: %d", ae.Status)
	}
	if !strings.Contains(ae.Message, "upstream timeout") {
		t.Errorf("message: %q", ae.Message)
	}
}

func TestDoRawSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"manifest_version":1}`))
	}))
	defer server.Close()
	c := New(server.URL, "tok", "test")
	resp, err := c.DoRaw(context.Background(), "GET", "/v1/sites/abc/manifest", nil)
	if err != nil {
		t.Fatalf("DoRaw: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status: %d", resp.StatusCode)
	}
}
