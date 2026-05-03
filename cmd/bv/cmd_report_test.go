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

func TestReport_RenderOnly(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "out.json")
	if err := os.WriteFile(jsonPath, []byte(`{
	  "title": "Test report",
	  "sections": [{"type":"headline","text":"hi","tone":"success"}]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")

	w, stdout, _ := newJSONWriter(t)
	rc := runReport(context.Background(), globalContext{w: w},
		[]string{"--from", jsonPath, "--out", outDir})
	if rc != 0 {
		t.Fatalf("rc: %d  stdout=%s", rc, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"template": "report"`) {
		t.Errorf("missing template field: %s", stdout.String())
	}
	idx, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatalf("index.html: %v", err)
	}
	if !strings.Contains(string(idx), "Test report") {
		t.Errorf("rendered index missing title: %s", idx[:200])
	}
}

func TestReport_RejectsBadJSON(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(jsonPath, []byte(`{"sections":[]}`), 0o644)

	w, _, _ := newJSONWriter(t)
	rc := runReport(context.Background(), globalContext{w: w},
		[]string{"--from", jsonPath, "--out", filepath.Join(dir, "out")})
	if rc == 0 {
		t.Fatal("expected non-zero on missing title")
	}
}

func TestReport_RejectsMissingFlag(t *testing.T) {
	w, _, _ := newJSONWriter(t)
	rc := runReport(context.Background(), globalContext{w: w}, []string{})
	if rc != 2 {
		t.Errorf("usage error should return 2, got %d", rc)
	}
}

func TestReport_PushSendsTemplateField(t *testing.T) {
	// End-to-end: render → push, server sees template=report on POST /v1/sites.
	srv := newFakeServer(t)
	withClientHostname(t, "cli-host.test")
	withPublishInvocationMetadata(t, "bv push ./dist", "/workspace/project")
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "out.json")
	_ = os.WriteFile(jsonPath, []byte(`{
	  "title": "T",
	  "sections": [{"type":"text","body":"hi"}]
	}`), 0o644)

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
	rc := runReport(context.Background(), globalContext{w: w},
		[]string{"--from", jsonPath, "--push"})
	if rc != 0 {
		t.Fatalf("rc: %d stdout=%s", rc, stdout.String())
	}
	if seenTemplate != "report" {
		t.Errorf("expected template=report on POST /v1/sites, got %q", seenTemplate)
	}
	assertPublishMetadata(t, createBody, publishSourcePath(jsonPath), "cli-host.test")
	assertPublishMetadata(t, finalizeBody, publishSourcePath(jsonPath), "cli-host.test")
	if got := finalizeBody["upload_id"]; got != createBody["upload_id"] {
		t.Errorf("finalize upload_id=%v, want create upload_id %v", got, createBody["upload_id"])
	}
	if !strings.Contains(stdout.String(), `"template": "report"`) {
		t.Errorf("response body should echo template: %s", stdout.String())
	}
}

func TestReport_PushModeLocalServesRenderedOutput(t *testing.T) {
	localServer := withFakeLocalServer(t)
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "out.json")
	_ = os.WriteFile(jsonPath, []byte(`{
	  "title": "Local report",
	  "sections": [{"type":"text","body":"hi"}]
	}`), 0o644)
	isolatedConfigPath(t)
	w, stdout, _ := newJSONWriter(t)
	rc := runReport(context.Background(), globalContext{w: w}, []string{"--from", jsonPath, "--push", "--mode", "local"})
	if rc != 0 {
		t.Fatalf("rc=%d stdout=%s", rc, stdout.String())
	}
	if !strings.Contains(localServer.IndexHTML, "Local report") {
		t.Fatalf("local server did not receive rendered report: %s", localServer.IndexHTML[:min(200, len(localServer.IndexHTML))])
	}
}
