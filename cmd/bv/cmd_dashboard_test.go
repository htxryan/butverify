package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDashboard_RenderOnly(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "data.csv")
	_ = os.WriteFile(csvPath, []byte("date,users\n2026-04-01,10\n2026-04-02,20\n"), 0o644)
	outDir := filepath.Join(dir, "out")

	w, stdout, _ := newJSONWriter(t)
	rc := runDashboard(context.Background(), globalContext{w: w},
		[]string{"--from", csvPath, "--out", outDir, "--title", "My Dashboard"})
	if rc != 0 {
		t.Fatalf("rc: %d stdout=%s", rc, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"row_count": 2`) {
		t.Errorf("row_count: %s", stdout.String())
	}
	idx, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(idx), "My Dashboard") {
		t.Errorf("custom title missing: %s", idx[:300])
	}
	if _, err := os.Stat(filepath.Join(outDir, "data.csv")); err != nil {
		t.Errorf("data.csv missing: %v", err)
	}
}

func TestDashboard_RejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "empty.csv")
	_ = os.WriteFile(csvPath, []byte(""), 0o644)
	w, _, _ := newJSONWriter(t)
	rc := runDashboard(context.Background(), globalContext{w: w},
		[]string{"--from", csvPath, "--out", filepath.Join(dir, "out")})
	if rc == 0 {
		t.Fatal("expected non-zero rc on empty CSV")
	}
}

func TestDashboard_PushSendsTemplateField(t *testing.T) {
	srv := newFakeServer(t)
	withClientHostname(t, "cli-host.test")
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "data.csv")
	_ = os.WriteFile(csvPath, []byte("date,n\n2026-04-01,1\n2026-04-02,2\n"), 0o644)

	var (
		seenTemplate string
		createBody   map[string]any
		finalizeBody map[string]any
	)
	stagingURL := ""
	srv.create = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&createBody)
		if v, ok := createBody["template"].(string); ok {
			seenTemplate = v
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"site_id":               "abcd1234",
			"url":                   "https://abcd1234.butverify.dev",
			"expires_at":            "2026-05-04T10:00:00Z",
			"upload_token":          "use_installation_token",
			"manifest_url":          "x",
			"status":                "creating",
			"idempotent":            false,
			"upload_url":            stagingURL,
			"upload_max_bytes":      100 * 1024 * 1024,
			"upload_url_expires_at": "2026-04-27T10:15:00Z",
		})
	}
	srv.finalize = func(w http.ResponseWriter, r *http.Request, siteID string) {
		_ = json.NewDecoder(r.Body).Decode(&finalizeBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"site_id":        siteID,
			"status":         "active",
			"url":            "https://abcd1234.butverify.dev",
			"manifest_url":   "x",
			"expires_at":     "2026-05-04T10:00:00Z",
			"manifest_sha":   strings.Repeat("a", 64),
			"last_pushed_at": "2026-04-27T10:00:00Z",
			"idempotent":     false,
		})
	}
	srv.put = func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			t.Error("PUT body empty")
		}
		w.WriteHeader(200)
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	stagingURL = server.URL + "/staging-put"
	setupConfig(t, server.URL)

	w, stdout, _ := newJSONWriter(t)
	rc := runDashboard(context.Background(), globalContext{w: w},
		[]string{"--from", csvPath, "--push"})
	if rc != 0 {
		t.Fatalf("rc: %d stdout=%s", rc, stdout.String())
	}
	if seenTemplate != "dashboard" {
		t.Errorf("expected template=dashboard, got %q", seenTemplate)
	}
	assertPublishMetadata(t, createBody, publishSourcePath(csvPath), "cli-host.test")
	assertPublishMetadata(t, finalizeBody, publishSourcePath(csvPath), "cli-host.test")
	if got := finalizeBody["upload_id"]; got != createBody["upload_id"] {
		t.Errorf("finalize upload_id=%v, want create upload_id %v", got, createBody["upload_id"])
	}
}

func TestDashboard_TemplateQuotaExceeded(t *testing.T) {
	// Server returns 402 with PAYMENT_REQUIRED (the actual production
	// envelope; the templated-fairness counter shares the same error code as
	// active-quota / byte-cap exhaustion — the message differs but the code
	// does not). CLI must map to exit code 5.
	srv := newFakeServer(t)
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "data.csv")
	_ = os.WriteFile(csvPath, []byte("a,b\n1,2\n"), 0o644)
	srv.create = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "PAYMENT_REQUIRED",
				"message": "templated-site monthly cap reached (30/30 for template=dashboard)",
			},
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)
	w, _, _ := newJSONWriter(t)
	rc := runDashboard(context.Background(), globalContext{w: w},
		[]string{"--from", csvPath, "--push"})
	if rc != 5 {
		t.Errorf("402 should map to exit 5, got %d", rc)
	}
}
