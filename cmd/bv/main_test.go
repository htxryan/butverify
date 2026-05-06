// End-to-end tests for the bv CLI subcommands. We stand up an httptest
// server that mimics the relevant control-plane endpoints and drive the
// run* functions directly (bypassing main()'s flag parsing — those paths
// are tested separately in flags_test.go).

package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/htxryan/butverify/internal/config"
	"github.com/htxryan/butverify/internal/output"
)

// fakeServer routes a small subset of the control-plane API. Each test
// sets the response handlers it cares about; unsupported routes 404.
type fakeServer struct {
	t             *testing.T
	whoami        func(http.ResponseWriter, *http.Request)
	listSites     func(http.ResponseWriter, *http.Request)
	getSite       func(http.ResponseWriter, *http.Request, string)
	delete        func(http.ResponseWriter, *http.Request, string)
	pin           func(http.ResponseWriter, *http.Request, string)
	unpin         func(http.ResponseWriter, *http.Request, string)
	manifest      func(http.ResponseWriter, *http.Request, string)
	files         func(http.ResponseWriter, *http.Request, string)
	getFile       func(http.ResponseWriter, *http.Request, string, string)
	create        func(http.ResponseWriter, *http.Request)
	finalize      func(http.ResponseWriter, *http.Request, string)
	put           func(http.ResponseWriter, *http.Request)
	listReviews   func(http.ResponseWriter, *http.Request)
	getReview     func(http.ResponseWriter, *http.Request, string)
	ackReview     func(http.ResponseWriter, *http.Request, string)
	requestReview func(http.ResponseWriter, *http.Request, string)
}

func newFakeServer(t *testing.T) *fakeServer {
	return &fakeServer{t: t}
}

func (f *fakeServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Auth gate (very loose — every test passes ghs_test).
		if r.Header.Get("Authorization") != "Bearer ghs_test" && r.URL.Path != "/staging-put" {
			http.Error(w, `{"error":{"code":"UNAUTHENTICATED","message":"missing"}}`, 401)
			return
		}
		switch {
		case r.URL.Path == "/v1/auth/whoami" && r.Method == "GET":
			if f.whoami != nil {
				f.whoami(w, r)
				return
			}
		case r.URL.Path == "/v1/sites" && r.Method == "GET":
			if f.listSites != nil {
				f.listSites(w, r)
				return
			}
		case r.URL.Path == "/v1/sites" && r.Method == "POST":
			if f.create != nil {
				f.create(w, r)
				return
			}
		case strings.HasPrefix(r.URL.Path, "/v1/sites/") && r.URL.Path != "/v1/sites/":
			rest := strings.TrimPrefix(r.URL.Path, "/v1/sites/")
			parts := strings.SplitN(rest, "/", 2)
			siteID := parts[0]
			tail := ""
			if len(parts) == 2 {
				tail = parts[1]
			}
			switch {
			case tail == "" && r.Method == "GET" && f.getSite != nil:
				f.getSite(w, r, siteID)
				return
			case tail == "" && r.Method == "DELETE" && f.delete != nil:
				f.delete(w, r, siteID)
				return
			case tail == "pin" && r.Method == "POST" && f.pin != nil:
				f.pin(w, r, siteID)
				return
			case tail == "unpin" && r.Method == "POST" && f.unpin != nil:
				f.unpin(w, r, siteID)
				return
			case tail == "manifest" && r.Method == "GET" && f.manifest != nil:
				f.manifest(w, r, siteID)
				return
			case tail == "files" && r.Method == "GET" && f.files != nil:
				f.files(w, r, siteID)
				return
			case tail == "finalize" && r.Method == "POST" && f.finalize != nil:
				f.finalize(w, r, siteID)
				return
			case strings.HasPrefix(tail, "files/") && r.Method == "GET" && f.getFile != nil:
				f.getFile(w, r, siteID, strings.TrimPrefix(tail, "files/"))
				return
			}
		case r.URL.Path == "/staging-put" && r.Method == "PUT":
			if f.put != nil {
				f.put(w, r)
				return
			}
		case r.URL.Path == "/v1/reviews" && (r.Method == "GET" || r.Method == "HEAD"):
			if f.listReviews != nil {
				f.listReviews(w, r)
				return
			}
		case strings.HasPrefix(r.URL.Path, "/v1/reviews/"):
			rest := strings.TrimPrefix(r.URL.Path, "/v1/reviews/")
			parts := strings.SplitN(rest, "/", 2)
			reviewID := parts[0]
			tail := ""
			if len(parts) == 2 {
				tail = parts[1]
			}
			switch {
			case tail == "" && r.Method == "GET" && f.getReview != nil:
				f.getReview(w, r, reviewID)
				return
			case tail == "acknowledge" && r.Method == "PATCH" && f.ackReview != nil:
				f.ackReview(w, r, reviewID)
				return
			}
		}
		// Site-scoped review-requests: POST /v1/sites/<id>/review-requests.
		// Handled outside the existing /v1/sites/ switch so a test that
		// only sets `requestReview` doesn't have to also stub the other
		// site routes.
		if strings.HasPrefix(r.URL.Path, "/v1/sites/") && strings.HasSuffix(r.URL.Path, "/review-requests") && r.Method == "POST" {
			rest := strings.TrimPrefix(r.URL.Path, "/v1/sites/")
			rest = strings.TrimSuffix(rest, "/review-requests")
			if f.requestReview != nil {
				f.requestReview(w, r, rest)
				return
			}
		}
		http.Error(w, `{"error":{"code":"NOT_FOUND","message":"no route"}}`, 404)
	})
}

func setupConfig(t *testing.T, apiURL string) string {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	t.Setenv("BV_CONFIG_PATH", cfgPath)
	if err := config.Save(&config.Config{
		APIURL:            apiURL,
		InstallationToken: "ghs_test",
		TenantID:          "t_u42",
		AccountLogin:      "alice",
		Mode:              config.ModeRemote,
	}); err != nil {
		t.Fatalf("setup config: %v", err)
	}
	return cfgPath
}

func newJSONWriter(t *testing.T) (*output.Writer, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	return output.NewWith(output.ModeJSON, &stdout, &stderr), &stdout, &stderr
}

func newTTYHumanWriter(t *testing.T) (*output.Writer, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	return output.NewWithTTY(output.ModeHuman, &stdout, &stderr), &stdout, &stderr
}

func withClientHostname(t *testing.T, hostname string) {
	t.Helper()
	old := collectClientHostname
	collectClientHostname = func() (string, error) { return hostname, nil }
	t.Cleanup(func() { collectClientHostname = old })
}

func withPublishInvocationMetadata(t *testing.T, command, cwd string) {
	t.Helper()
	old := collectPublishInvocationMetadata
	collectPublishInvocationMetadata = func() (string, string) { return command, cwd }
	t.Cleanup(func() { collectPublishInvocationMetadata = old })
}

func assertPublishMetadata(t *testing.T, body map[string]any, wantSourcePath, wantHostname string) {
	t.Helper()
	if got := body["source_path"]; got != wantSourcePath {
		t.Errorf("source_path=%v, want %q", got, wantSourcePath)
	}
	if got := body["client_hostname"]; got != wantHostname {
		t.Errorf("client_hostname=%v, want %q", got, wantHostname)
	}
	if got := body["cli_version"]; got != Version {
		t.Errorf("cli_version=%v, want %q", got, Version)
	}
	if got := body["publish_command"]; got != "bv push ./dist" {
		t.Errorf("publish_command=%v, want %q", got, "bv push ./dist")
	}
	if got := body["publish_cwd"]; got != "/workspace/project" {
		t.Errorf("publish_cwd=%v, want %q", got, "/workspace/project")
	}
}

func writeCLIJPEGFixture(t *testing.T, path string, quality int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 3), G: uint8(y * 3), B: uint8((x + y) * 2), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	data := buf.Bytes()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write jpeg: %v", err)
	}
	return data
}

func tarEntryBytes(t *testing.T, bundle []byte, name string) []byte {
	t.Helper()
	r := tar.NewReader(bytes.NewReader(bundle))
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar: %v", err)
		}
		if h.Name != name {
			continue
		}
		body, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("read tar entry %s: %v", name, err)
		}
		return body
	}
	t.Fatalf("tar entry %s not found", name)
	return nil
}

func TestPublishInvocationMetadataShellQuotesArgs(t *testing.T) {
	got := publishShellJoin([]string{"bv", "push", "my dir", "--title", "Bob's report"})
	want := `bv push 'my dir' --title 'Bob'\''s report'`
	if got != want {
		t.Fatalf("shellJoin=%q, want %q", got, want)
	}
}

func TestWhoami(t *testing.T) {
	srv := newFakeServer(t)
	srv.whoami = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tenant_id":       "t_u42",
			"installation_id": 99,
			"account_login":   "alice",
			"account_type":    "User",
			"expires_at":      "2026-04-27T11:00:00Z",
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)
	w, stdout, _ := newJSONWriter(t)
	g := globalContext{w: w}
	rc := runWhoami(context.Background(), g, nil)
	if rc != 0 {
		t.Errorf("rc: %d", rc)
	}
	if !strings.Contains(stdout.String(), `"tenant_id": "t_u42"`) {
		t.Errorf("stdout: %s", stdout.String())
	}
}

func TestListSites(t *testing.T) {
	srv := newFakeServer(t)
	srv.listSites = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sites": []map[string]any{
				{"site_id": "abcd1234", "tenant_id": "t_u42", "status": "active", "url": "https://abcd1234.butverify.dev", "manifest_url": "x", "bytes_used": 1024, "created_at": "x", "updated_at": "x"},
			},
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)
	w, stdout, _ := newJSONWriter(t)
	rc := runList(context.Background(), globalContext{w: w}, nil)
	if rc != 0 {
		t.Fatalf("rc: %d", rc)
	}
	if !strings.Contains(stdout.String(), `"site_id": "abcd1234"`) {
		t.Errorf("stdout: %s", stdout.String())
	}
}

func TestRemove(t *testing.T) {
	srv := newFakeServer(t)
	srv.delete = func(w http.ResponseWriter, r *http.Request, siteID string) {
		if siteID != "abcd1234" {
			t.Errorf("site_id: %s", siteID)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"site_id": siteID, "status": "expired"})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)
	w, stdout, _ := newJSONWriter(t)
	rc := runRemove(context.Background(), globalContext{w: w}, []string{"abcd1234"})
	if rc != 0 {
		t.Fatalf("rc: %d", rc)
	}
	if !strings.Contains(stdout.String(), `"status": "expired"`) {
		t.Errorf("stdout: %s", stdout.String())
	}
}

func TestPinUnpin(t *testing.T) {
	srv := newFakeServer(t)
	srv.pin = func(w http.ResponseWriter, r *http.Request, siteID string) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"site_id":    siteID,
			"status":     "pinned",
			"pinned_at":  "2026-04-27T10:00:00Z",
			"expires_at": nil,
		})
	}
	srv.unpin = func(w http.ResponseWriter, r *http.Request, siteID string) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"site_id":    siteID,
			"status":     "active",
			"pinned_at":  nil,
			"expires_at": "2026-05-04T10:00:00Z",
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	w, stdout, _ := newJSONWriter(t)
	if rc := runPin(context.Background(), globalContext{w: w}, []string{"abcd1234"}); rc != 0 {
		t.Fatalf("pin rc: %d", rc)
	}
	if !strings.Contains(stdout.String(), `"status": "pinned"`) {
		t.Errorf("pin stdout: %s", stdout.String())
	}

	w2, stdout2, _ := newJSONWriter(t)
	if rc := runUnpin(context.Background(), globalContext{w: w2}, []string{"abcd1234"}); rc != 0 {
		t.Fatalf("unpin rc: %d", rc)
	}
	if !strings.Contains(stdout2.String(), `"status": "active"`) {
		t.Errorf("unpin stdout: %s", stdout2.String())
	}
}

func TestManifest(t *testing.T) {
	srv := newFakeServer(t)
	srv.manifest = func(w http.ResponseWriter, r *http.Request, siteID string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"manifest_version":1,"site_id":"abcd1234","files":[]}`))
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	// Manifest writes raw bytes to os.Stdout, so capture it.
	r, wp, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = wp
	defer func() { os.Stdout = old }()

	w, _, _ := newJSONWriter(t)
	rc := runManifest(context.Background(), globalContext{w: w}, []string{"abcd1234"})
	_ = wp.Close()
	if rc != 0 {
		t.Fatalf("rc: %d", rc)
	}
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), `"manifest_version":1`) {
		t.Errorf("output: %s", out)
	}
}

func TestRemoveErrorMapsToExitCode(t *testing.T) {
	srv := newFakeServer(t)
	srv.delete = func(w http.ResponseWriter, r *http.Request, siteID string) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": "NOT_FOUND", "message": "unknown site"},
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)
	w, _, _ := newJSONWriter(t)
	rc := runRemove(context.Background(), globalContext{w: w}, []string{"unknown1"})
	if rc != 6 {
		t.Errorf("404 should map to exit 6, got %d", rc)
	}
}

func TestPushHappyPath(t *testing.T) {
	srv := newFakeServer(t)
	withClientHostname(t, "cli-host.test")
	withPublishInvocationMetadata(t, "bv push ./dist", "/workspace/project")
	// Stage a temp dir with one file to bundle.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hello</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	var (
		gotPut       bool
		gotFinalize  bool
		uploadIDSeen string
		createBody   map[string]any
		finalizeBody map[string]any
	)
	stagingURL := ""
	srv.create = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&createBody)
		uploadIDSeen = createBody["upload_id"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"site_id":               "abcd1234",
			"url":                   "https://abcd1234.butverify.dev",
			"expires_at":            "2026-05-04T10:00:00Z",
			"upload_token":          "use_installation_token",
			"manifest_url":          "https://api.example.test/v1/sites/abcd1234/manifest",
			"status":                "creating",
			"idempotent":            false,
			"upload_url":            stagingURL,
			"upload_max_bytes":      100 * 1024 * 1024,
			"upload_url_expires_at": "2026-04-27T10:15:00Z",
		})
	}
	srv.finalize = func(w http.ResponseWriter, r *http.Request, siteID string) {
		gotFinalize = true
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
		gotPut = true
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

	w, stdout, stderr := newJSONWriter(t)
	rc := runPush(context.Background(), globalContext{w: w}, []string{dir})
	if rc != 0 {
		t.Fatalf("push rc: %d, stdout=%s", rc, stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("json push should not write progress to stderr: %s", stderr.String())
	}
	if !gotPut {
		t.Error("PUT not seen")
	}
	if !gotFinalize {
		t.Error("finalize not seen")
	}
	if !strings.HasPrefix(uploadIDSeen, "u-") {
		t.Errorf("upload_id should start with u-: %s", uploadIDSeen)
	}
	assertPublishMetadata(t, createBody, publishSourcePath(dir), "cli-host.test")
	assertPublishMetadata(t, finalizeBody, publishSourcePath(dir), "cli-host.test")
	if finalizeBody["upload_id"] != uploadIDSeen {
		t.Errorf("finalize upload_id=%v, want create upload_id %q", finalizeBody["upload_id"], uploadIDSeen)
	}
	if !strings.Contains(stdout.String(), `"manifest_sha"`) {
		t.Errorf("stdout missing manifest_sha: %s", stdout.String())
	}
}

func TestPushBlocksGitleaksFindingBeforeNetwork(t *testing.T) {
	srv := newFakeServer(t)
	var createCalls int
	srv.create = func(w http.ResponseWriter, r *http.Request) {
		createCalls++
		w.WriteHeader(http.StatusInternalServerError)
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	dir := t.TempDir()
	secret := privateKeyGitleaksFixture()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(secret), 0o644); err != nil {
		t.Fatal(err)
	}

	w, stdout, _ := newJSONWriter(t)
	rc := runPush(context.Background(), globalContext{w: w}, []string{dir})
	if rc == 0 {
		t.Fatalf("push should fail on gitleaks finding, stdout=%s", stdout.String())
	}
	if createCalls != 0 {
		t.Fatalf("push should not create a site before blocking, createCalls=%d", createCalls)
	}
	out := stdout.String()
	for _, want := range []string{"gitleaks detected", "blocked before creating or uploading", "--skip-gitleaks-check", "index.html"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, secret) {
		t.Fatalf("stdout should not leak the detected secret:\n%s", out)
	}
}

func TestPushSkipGitleaksCheckAllowsFinding(t *testing.T) {
	srv := newFakeServer(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(privateKeyGitleaksFixture()), 0o644); err != nil {
		t.Fatal(err)
	}

	var gotPut bool
	stagingURL := ""
	srv.create = func(w http.ResponseWriter, r *http.Request) {
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
		gotPut = true
		w.WriteHeader(200)
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	stagingURL = server.URL + "/staging-put"
	setupConfig(t, server.URL)

	w, stdout, _ := newJSONWriter(t)
	rc := runPush(context.Background(), globalContext{w: w}, []string{"--skip-gitleaks-check", dir})
	if rc != 0 {
		t.Fatalf("push rc: %d, stdout=%s", rc, stdout.String())
	}
	if !gotPut {
		t.Fatal("PUT not seen with explicit skip flag")
	}
}

func privateKeyGitleaksFixture() string {
	return strings.Join([]string{
		"-----BEGIN OPENSSH" + " PRIVATE KEY-----",
		"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW",
		"-----END OPENSSH" + " PRIVATE KEY-----",
	}, "\n")
}

func TestPushOptimizesImagesBeforeUpload(t *testing.T) {
	srv := newFakeServer(t)
	dir := t.TempDir()
	original := writeCLIJPEGFixture(t, filepath.Join(dir, "photo.jpg"), 100)
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<img src=photo.jpg>"), 0o644); err != nil {
		t.Fatal(err)
	}

	var uploaded []byte
	stagingURL := ""
	srv.create = func(w http.ResponseWriter, r *http.Request) {
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
		uploaded, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	stagingURL = server.URL + "/staging-put"
	setupConfig(t, server.URL)

	w, stdout, _ := newJSONWriter(t)
	rc := runPush(context.Background(), globalContext{w: w}, []string{"--image-quality", "40", dir})
	if rc != 0 {
		t.Fatalf("push rc: %d, stdout=%s", rc, stdout.String())
	}
	optimized := tarEntryBytes(t, uploaded, "photo.jpg")
	if len(optimized) >= len(original) {
		t.Fatalf("uploaded photo.jpg was not optimized: got %d original %d", len(optimized), len(original))
	}
	if _, err := jpeg.Decode(bytes.NewReader(optimized)); err != nil {
		t.Fatalf("optimized upload should decode as jpeg: %v", err)
	}
}

func TestPushHumanOutputShowsProgressAndStructuredResult(t *testing.T) {
	srv := newFakeServer(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hello</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	stagingURL := ""
	srv.create = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"site_id":               "abcd1234",
			"url":                   "https://abcd1234.butverify.dev",
			"expires_at":            "2026-05-04T10:00:00Z",
			"upload_token":          "use_installation_token",
			"manifest_url":          "https://api.example.test/v1/sites/abcd1234/manifest",
			"status":                "creating",
			"idempotent":            false,
			"upload_url":            stagingURL,
			"upload_max_bytes":      100 * 1024 * 1024,
			"upload_url_expires_at": "2026-04-27T10:15:00Z",
		})
	}
	srv.finalize = func(w http.ResponseWriter, r *http.Request, siteID string) {
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

	w, stdout, stderr := newHumanWriter(t)
	rc := runPush(context.Background(), globalContext{w: w}, []string{dir})
	if rc != 0 {
		t.Fatalf("push rc: %d, stdout=%s stderr=%s", rc, stdout.String(), stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"Published site",
		"Open URL:   https://abcd1234.butverify.dev",
		"Metadata",
		"Site ID:    abcd1234",
		"Status:     active",
		"Manifest:   " + strings.Repeat("a", 64),
		"Files:      1",
		"Size:",
		"Expires:    2026-05-04T10:00:00Z",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("human stdout missing %q:\n%s", want, out)
		}
	}

	errOut := stderr.String()
	for _, want := range []string{
		"[1/4] [#####---------------] Bundled: 1 files",
		"[2/4] [##########----------] Provisioned: abcd1234 ready for upload",
		"[3/4] [###############-----] Uploaded: bundle staged",
		"[4/4] [####################] Published: https://abcd1234.butverify.dev",
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("human stderr missing %q:\n%s", want, errOut)
		}
	}
}

func TestPushHumanOutputTTYRedrawsProgressInPlace(t *testing.T) {
	srv := newFakeServer(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hello</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	stagingURL := ""
	srv.create = func(w http.ResponseWriter, r *http.Request) {
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
		w.WriteHeader(200)
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	stagingURL = server.URL + "/staging-put"
	setupConfig(t, server.URL)

	w, stdout, stderr := newTTYHumanWriter(t)
	rc := runPush(context.Background(), globalContext{w: w}, []string{dir})
	if rc != 0 {
		t.Fatalf("push rc: %d, stdout=%s stderr=%s", rc, stdout.String(), stderr.String())
	}

	errOut := stderr.String()
	if strings.Count(errOut, "\r\033[2K") != 5 {
		t.Fatalf("stderr should redraw in place and clear before result: %q", errOut)
	}
	if strings.Contains(errOut, "\n[2/4]") || strings.Contains(errOut, "\n[3/4]") || strings.Contains(errOut, "\n[4/4]") {
		t.Fatalf("stderr should not contain snapshot progress lines: %q", errOut)
	}
	if !strings.HasSuffix(errOut, "\r\033[2K\n") {
		t.Fatalf("stderr should terminate the active progress line before human output: %q", errOut)
	}
	if !strings.Contains(stdout.String(), "Published site") {
		t.Fatalf("stdout: %s", stdout.String())
	}
}

func TestPushJSONOutputStaysMachineOnly(t *testing.T) {
	srv := newFakeServer(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hello</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	stagingURL := ""
	srv.create = func(w http.ResponseWriter, r *http.Request) {
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
		w.WriteHeader(200)
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	stagingURL = server.URL + "/staging-put"
	setupConfig(t, server.URL)

	w, stdout, stderr := newJSONWriter(t)
	rc := runPush(context.Background(), globalContext{w: w}, []string{dir})
	if rc != 0 {
		t.Fatalf("push rc: %d, stdout=%s stderr=%s", rc, stdout.String(), stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("json mode stderr should stay empty, got: %s", stderr.String())
	}
	if strings.Contains(stdout.String(), "Published site") || strings.Contains(stdout.String(), "[1/4]") {
		t.Fatalf("json stdout polluted with human output: %s", stdout.String())
	}
	var got pushResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json stdout is not valid pushResult JSON: %v\n%s", err, stdout.String())
	}
	if got.URL != "https://abcd1234.butverify.dev" || got.SiteID != "abcd1234" || got.Status != "active" || got.ManifestSHA != strings.Repeat("a", 64) || got.ExpiresAt != "2026-05-04T10:00:00Z" {
		t.Fatalf("json push result changed unexpectedly: %+v", got)
	}
}

func TestPushDefaultsToLocalWithoutConfig(t *testing.T) {
	localServer := withFakeLocalServer(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>local</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	isolatedConfigPath(t)
	w, stdout, _ := newJSONWriter(t)
	rc := runPush(context.Background(), globalContext{w: w}, []string{dir})
	if rc != 0 {
		t.Fatalf("push rc=%d stdout=%s", rc, stdout.String())
	}
	if localServer.Root == "" || localServer.Root == dir {
		t.Fatalf("served root should be filtered staging dir, got %q from source %q", localServer.Root, dir)
	}
	if !strings.Contains(localServer.IndexHTML, "local") {
		t.Fatalf("staged local index missing source content: %s", localServer.IndexHTML)
	}
	if !strings.Contains(stdout.String(), `"mode": "local"`) || !strings.Contains(stdout.String(), `"url": "http://127.0.0.1:12345/"`) {
		t.Fatalf("stdout missing local result: %s", stdout.String())
	}
}

func TestPushModeRemoteWithoutConfigRequiresLogin(t *testing.T) {
	localServer := withFakeLocalServer(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>remote</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	isolatedConfigPath(t)
	w, _, _ := newJSONWriter(t)
	rc := runPush(context.Background(), globalContext{w: w}, []string{"--mode", "remote", dir})
	if rc != 3 {
		t.Fatalf("push rc=%d, want 3", rc)
	}
	if localServer.Root != "" {
		t.Fatalf("local server should not start in remote mode, root=%q", localServer.Root)
	}
}

func TestPushInvalidModeExit2(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>bad mode</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	w, _, _ := newJSONWriter(t)
	rc := runPush(context.Background(), globalContext{w: w}, []string{"--mode", "bogus", dir})
	if rc != 2 {
		t.Fatalf("push rc=%d, want 2", rc)
	}
}

func TestPushRefreshesExpiredTokenBeforeCreate(t *testing.T) {
	withCleanGHEnv(t)
	t.Setenv("GH_TOKEN", "ghu_refresh")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hello</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	var (
		stagingURL   string
		loginCalls   int
		createAuth   string
		finalizeAuth string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/auth/login" && r.Method == "POST":
			loginCalls++
			if got := r.Header.Get("Authorization"); got != "Bearer ghu_refresh" {
				t.Errorf("refresh Authorization=%q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":           "ghs_new",
				"expires_at":      time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
				"tenant_id":       "t_alice",
				"account_login":   "alice",
				"installation_id": 42,
				"account_type":    "User",
			})
		case r.URL.Path == "/v1/sites" && r.Method == "POST":
			createAuth = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"site_id":               "abcd1234",
				"url":                   "https://abcd1234.butverify.dev",
				"expires_at":            "2026-05-04T10:00:00Z",
				"upload_token":          "use_installation_token",
				"manifest_url":          "https://api.example.test/v1/sites/abcd1234/manifest",
				"status":                "creating",
				"idempotent":            false,
				"upload_url":            stagingURL,
				"upload_max_bytes":      100 * 1024 * 1024,
				"upload_url_expires_at": "2026-04-27T10:15:00Z",
			})
		case r.URL.Path == "/staging-put" && r.Method == "PUT":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/v1/sites/abcd1234/finalize" && r.Method == "POST":
			finalizeAuth = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"site_id":        "abcd1234",
				"status":         "active",
				"url":            "https://abcd1234.butverify.dev",
				"manifest_url":   "x",
				"expires_at":     "2026-05-04T10:00:00Z",
				"manifest_sha":   strings.Repeat("a", 64),
				"last_pushed_at": "2026-04-27T10:00:00Z",
				"idempotent":     false,
			})
		default:
			http.Error(w, `{"error":{"code":"NOT_FOUND","message":"no route"}}`, http.StatusNotFound)
		}
	}))
	defer srv.Close()
	stagingURL = srv.URL + "/staging-put"

	isolatedConfigPath(t)
	if err := config.Save(&config.Config{
		APIURL:            srv.URL,
		InstallationToken: "ghs_old",
		TenantID:          "t_alice",
		AccountLogin:      "alice",
		InstallationID:    42,
		TokenExpiresAt:    time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		Mode:              config.ModeRemote,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	w, stdout, _ := newJSONWriter(t)
	rc := runPush(context.Background(), globalContext{w: w}, []string{dir})
	if rc != 0 {
		t.Fatalf("push rc: %d, stdout=%s", rc, stdout.String())
	}
	if loginCalls != 1 {
		t.Fatalf("loginCalls=%d, want 1", loginCalls)
	}
	if createAuth != "Bearer ghs_new" {
		t.Errorf("create Authorization=%q, want refreshed token", createAuth)
	}
	if finalizeAuth != "Bearer ghs_new" {
		t.Errorf("finalize Authorization=%q, want refreshed token", finalizeAuth)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.InstallationToken != "ghs_new" {
		t.Errorf("persisted token=%q, want refreshed token", loaded.InstallationToken)
	}
}

func TestPushRefreshesAndRetriesCreateAfter401(t *testing.T) {
	withCleanGHEnv(t)
	t.Setenv("GH_TOKEN", "ghu_refresh")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hello</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	var (
		stagingURL string
		loginCalls int
		createAuth []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/auth/login" && r.Method == "POST":
			loginCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":           "ghs_new",
				"expires_at":      time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
				"tenant_id":       "t_alice",
				"account_login":   "alice",
				"installation_id": 42,
				"account_type":    "User",
			})
		case r.URL.Path == "/v1/sites" && r.Method == "POST":
			createAuth = append(createAuth, r.Header.Get("Authorization"))
			if len(createAuth) == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "UNAUTHENTICATED", "message": "expired"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"site_id":               "abcd1234",
				"url":                   "https://abcd1234.butverify.dev",
				"expires_at":            "2026-05-04T10:00:00Z",
				"upload_token":          "use_installation_token",
				"manifest_url":          "https://api.example.test/v1/sites/abcd1234/manifest",
				"status":                "creating",
				"idempotent":            false,
				"upload_url":            stagingURL,
				"upload_max_bytes":      100 * 1024 * 1024,
				"upload_url_expires_at": "2026-04-27T10:15:00Z",
			})
		case r.URL.Path == "/staging-put" && r.Method == "PUT":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/v1/sites/abcd1234/finalize" && r.Method == "POST":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"site_id":        "abcd1234",
				"status":         "active",
				"url":            "https://abcd1234.butverify.dev",
				"manifest_url":   "x",
				"expires_at":     "2026-05-04T10:00:00Z",
				"manifest_sha":   strings.Repeat("a", 64),
				"last_pushed_at": "2026-04-27T10:00:00Z",
				"idempotent":     false,
			})
		default:
			http.Error(w, `{"error":{"code":"NOT_FOUND","message":"no route"}}`, http.StatusNotFound)
		}
	}))
	defer srv.Close()
	stagingURL = srv.URL + "/staging-put"

	isolatedConfigPath(t)
	if err := config.Save(&config.Config{
		APIURL:            srv.URL,
		InstallationToken: "ghs_old",
		TenantID:          "t_alice",
		AccountLogin:      "alice",
		InstallationID:    42,
		TokenExpiresAt:    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		Mode:              config.ModeRemote,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	w, stdout, _ := newJSONWriter(t)
	rc := runPush(context.Background(), globalContext{w: w}, []string{dir})
	if rc != 0 {
		t.Fatalf("push rc: %d, stdout=%s", rc, stdout.String())
	}
	if loginCalls != 1 {
		t.Fatalf("loginCalls=%d, want 1", loginCalls)
	}
	if got, want := strings.Join(createAuth, ","), "Bearer ghs_old,Bearer ghs_new"; got != want {
		t.Errorf("create auth sequence=%q, want %q", got, want)
	}
}

func TestPushTokenOverrideDoesNotAutoRefresh(t *testing.T) {
	withCleanGHEnv(t)
	t.Setenv("GH_TOKEN", "ghu_refresh")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hello</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	loginCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/auth/login":
			loginCalls++
			http.Error(w, `{"error":{"code":"UNEXPECTED","message":"should not refresh"}}`, http.StatusInternalServerError)
		case r.URL.Path == "/v1/sites" && r.Method == "POST":
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "UNAUTHENTICATED", "message": "expired"}})
		default:
			http.Error(w, `{"error":{"code":"NOT_FOUND","message":"no route"}}`, http.StatusNotFound)
		}
	}))
	defer srv.Close()
	isolatedConfigPath(t)

	w, _, _ := newJSONWriter(t)
	rc := runPush(context.Background(), globalContext{w: w, apiURLOverride: srv.URL, tokenOverride: "ghs_override"}, []string{dir})
	if rc != 4 {
		t.Fatalf("push rc=%d, want 4", rc)
	}
	if loginCalls != 0 {
		t.Fatalf("loginCalls=%d, want 0", loginCalls)
	}
}

// Ensure 404 maps to exit code 6, 401 to 4, etc.
func TestErrorExitCodes(t *testing.T) {
	cases := []struct {
		status int
		want   int
		code   string
	}{
		{401, 4, "UNAUTHENTICATED"},
		{402, 5, "PAYMENT_REQUIRED"},
		{404, 6, "NOT_FOUND"},
		{409, 7, "CONFLICT"},
		{426, 9, "UPGRADE_REQUIRED"},
		{429, 8, "RATE_LIMITED"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%d-%s", tc.status, tc.code), func(t *testing.T) {
			srv := newFakeServer(t)
			srv.listSites = func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{"code": tc.code, "message": "mock"},
				})
			}
			server := httptest.NewServer(srv.handler())
			defer server.Close()
			setupConfig(t, server.URL)
			w, _, _ := newJSONWriter(t)
			rc := runList(context.Background(), globalContext{w: w}, nil)
			if rc != tc.want {
				t.Errorf("status=%d want exit %d, got %d", tc.status, tc.want, rc)
			}
		})
	}
}
