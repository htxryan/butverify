// Tests for `bv evidence`. These cover EARS-E surface (E12-T5) plus the
// EV-E-8 distinctive 400 envelope and the EV-N-7 cross-device
// surfacing. Render-pipeline behavior (containment, MIME, atomic
// rename) is covered in templates/evidence_test.go; we don't re-test
// it here.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/htxryan/butverify/internal/output"
	"github.com/htxryan/butverify/pkg/templates"
)

// makeTestPNG returns the bytes of a tiny 1x1 PNG. http.DetectContentType
// reads up to 512 bytes and matches the standard PNG signature in the
// first 8, so the fixture passes T3's MIME double-gate.
func makeTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test PNG: %v", err)
	}
	return buf.Bytes()
}

// stageEvidenceFixture writes a manifest JSON file plus two PNGs into
// dir and returns the path of the JSON file. The manifest references
// `./shot.png` (so the containment root is dir).
func stageEvidenceFixture(t *testing.T, dir string) string {
	t.Helper()
	pngBytes := makeTestPNG(t)
	if err := os.WriteFile(filepath.Join(dir, "shot.png"), pngBytes, 0o644); err != nil {
		t.Fatalf("write png: %v", err)
	}
	jsonPath := filepath.Join(dir, "evidence.json")
	manifest := `{
		"title": "Evidence test",
		"subtitle": "Subtest",
		"metadata": {
			"issue_url": "https://jira.example.com/browse/EV-1",
			"issue_id": "EV-1",
			"issue_title": "Evidence test issue"
		},
		"items": [
			{
				"src": "./shot.png",
				"title": "First",
				"description": "shot 1",
				"sequence": 1,
				"metadata": {
					"issue_url": "https://jira.example.com/browse/EV-1",
					"issue_id": "EV-1",
					"issue_title": "Evidence test issue"
				}
			}
		]
	}`
	if err := os.WriteFile(jsonPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}
	return jsonPath
}

func TestEvidence_Schema(t *testing.T) {
	// --schema goes to real os.Stdout (so consumers can pipe to jq
	// without --json mangling). Capture os.Stdout to verify.
	r, wp, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = wp
	defer func() { os.Stdout = old }()

	w, _, _ := newJSONWriter(t)
	rc := runEvidence(context.Background(), globalContext{w: w}, []string{"--schema"})
	_ = wp.Close()

	if rc != 0 {
		t.Fatalf("--schema rc: %d", rc)
	}
	out, _ := io.ReadAll(r)
	if string(out) != templates.EvidenceSchema {
		t.Errorf("schema output mismatch (len got=%d want=%d)", len(out), len(templates.EvidenceSchema))
	}
}

func TestEvidence_NoFlagsExit2(t *testing.T) {
	w, _, stderr := newJSONWriter(t)
	rc := runEvidence(context.Background(), globalContext{w: w}, []string{})
	if rc != 2 {
		t.Errorf("expected rc=2 for missing flags, got %d", rc)
	}
	// JSON mode writes the error envelope to stdout, not stderr; we
	// just need the exit code to match the EV-E-4 contract. Sanity-
	// check no panic by reading stderr.
	_ = stderr.String()
}

func TestEvidence_StdinTTYRejected(t *testing.T) {
	// Swap out the TTY probe to simulate a TTY stdin. EV-E-4 stdin
	// guard: when --from - is supplied with a TTY stdin, exit 2.
	orig := stdinIsTTY
	stdinIsTTY = func() bool { return true }
	defer func() { stdinIsTTY = orig }()

	w, _, _ := newJSONWriter(t)
	rc := runEvidence(context.Background(), globalContext{w: w}, []string{"--from", "-"})
	if rc != 2 {
		t.Errorf("expected rc=2 for TTY stdin, got %d", rc)
	}
}

func TestEvidence_RenderOnly_EmitsV2BundleReferences(t *testing.T) {
	// V2 (CDN-bundle) is the default render path per spec
	// docs/specs/evidence-v2.md §3.1 (EV2-U-2). The CLI's
	// generated index.html must reference the versioned CDN
	// bundle assets, inline the manifest as a JSON script tag,
	// and provide a noscript fallback. Layout/theme controls are
	// rendered client-side by the bundle, NOT baked in by the CLI.
	dir := t.TempDir()
	jsonPath := stageEvidenceFixture(t, dir)
	outDir := filepath.Join(dir, "out")

	w, stdout, _ := newJSONWriter(t)
	rc := runEvidence(context.Background(), globalContext{w: w},
		[]string{"--from", jsonPath, "--out", outDir})
	if rc != 0 {
		t.Fatalf("rc: %d  stdout=%s", rc, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"template": "evidence"`) {
		t.Errorf("expected template field in stdout: %s", stdout.String())
	}
	idx, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(idx)
	if strings.Contains(html, `data-layout=`) {
		t.Errorf("rendered html should not hard-code a publish-time layout (first 300): %s", html[:min(300, len(html))])
	}
	for _, want := range []string{
		// CDN bundle references — must use the templates.EvidenceBundleVersion path.
		`/assets/evidence/v` + templates.EvidenceBundleVersion + `/_astro/main.js`,
		`/assets/evidence/v` + templates.EvidenceBundleVersion + `/_astro/main.css`,
		// Manifest mount point for the Svelte gallery.
		`<div id="bv-gallery-root"></div>`,
		// Inlined manifest payload.
		`<script type="application/json" id="evidence-manifest">`,
		// Noscript fallback present (assistive-tech / JS-disabled clients).
		`<noscript>`,
		// Title round-trips.
		`Evidence test`,
		// Per-item issue metadata still surfaces in the manifest payload
		// (the test fixture sets EV-1 / jira URL).
		`EV-1`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered html missing v2 marker %q (first 300): %s", want, html[:min(300, len(html))])
		}
	}
	// V2 must NOT emit the legacy in-binary bundle files. They live on the CDN.
	if _, err := os.Stat(filepath.Join(outDir, "styles.css")); err == nil {
		t.Errorf("v2 must not write styles.css (CDN-served); file exists at %s/styles.css", outDir)
	}
	if _, err := os.Stat(filepath.Join(outDir, "evidence.js")); err == nil {
		t.Errorf("v2 must not write evidence.js (CDN-served); file exists at %s/evidence.js", outDir)
	}
	// Asset has the deterministic `001-shot.png` prefix from
	// templates.SafeAssetName.
	assetEntries, _ := os.ReadDir(filepath.Join(outDir, "assets"))
	if len(assetEntries) != 1 {
		t.Errorf("expected 1 asset, got %d", len(assetEntries))
	}
}

func TestEvidence_LayoutFlagRemoved(t *testing.T) {
	dir := t.TempDir()
	jsonPath := stageEvidenceFixture(t, dir)

	wHuman, _, hStderr := newHumanWriter(t)
	rc := runEvidence(context.Background(), globalContext{w: wHuman},
		[]string{"--from", jsonPath, "--layout", "carousel", "--out", filepath.Join(dir, "out")})
	if rc != 2 {
		t.Errorf("expected rc=2 for removed --layout flag, got %d", rc)
	}
	msg := hStderr.String()
	if !strings.Contains(msg, "flag provided but not defined") || !strings.Contains(msg, "layout") {
		t.Errorf("error message should reject removed --layout flag: %s", msg)
	}
}

func TestEvidence_FromStdin(t *testing.T) {
	// stdin path resolves item.src against CWD. Stage assets in a
	// dedicated dir, chdir into it, then feed JSON via os.Stdin pipe.
	dir := t.TempDir()
	pngBytes := makeTestPNG(t)
	if err := os.WriteFile(filepath.Join(dir, "shot.png"), pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	prevWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(prevWD) }()

	manifest := `{"title":"From stdin","items":[{"src":"./shot.png","title":"x"}]}`
	r, wp, _ := os.Pipe()
	_, _ = wp.WriteString(manifest)
	_ = wp.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	// stdinIsTTY would otherwise stat os.Stdin (a pipe → not a TTY),
	// but be explicit so the test is deterministic on every host.
	orig := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	defer func() { stdinIsTTY = orig }()

	outDir := filepath.Join(dir, "out")
	w, _, _ := newJSONWriter(t)
	rc := runEvidence(context.Background(), globalContext{w: w},
		[]string{"--from", "-", "--out", outDir})
	if rc != 0 {
		t.Fatalf("rc: %d", rc)
	}
	if _, err := os.Stat(filepath.Join(outDir, "index.html")); err != nil {
		t.Errorf("index.html missing: %v", err)
	}
}

func TestEvidence_PushEVE8DistinctiveError(t *testing.T) {
	srv := newFakeServer(t)
	dir := t.TempDir()
	jsonPath := stageEvidenceFixture(t, dir)

	srv.create = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "BadRequest",
				"message": "template must be one of report, dashboard",
			},
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	w, stdout, _ := newJSONWriter(t)
	rc := runEvidence(context.Background(), globalContext{w: w},
		[]string{"--from", jsonPath, "--push"})
	if rc == 0 {
		t.Fatalf("expected non-zero rc; stdout=%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "evidence template not yet enabled") {
		t.Errorf("expected EV-E-8 distinctive message in error envelope: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "EV-E-8") {
		t.Errorf("expected EV-E-8 marker in error envelope: %s", stdout.String())
	}
	// Ephemeral temp dir cleanup: there's no easy way to assert the
	// exact path without exporting it, but RenderEvidence puts ephemeral
	// dirs under os.TempDir() with the .evidence- prefix. Walk that
	// directory and confirm no leftover dir contains our manifest's
	// title.
	tmp := os.TempDir()
	entries, _ := os.ReadDir(tmp)
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), ".evidence-") {
			continue
		}
		idx, err := os.ReadFile(filepath.Join(tmp, e.Name(), "index.html"))
		if err == nil && strings.Contains(string(idx), "Evidence test") {
			t.Errorf("ephemeral evidence tmpdir leaked: %s", filepath.Join(tmp, e.Name()))
			_ = os.RemoveAll(filepath.Join(tmp, e.Name()))
		}
	}
}

func TestEvidence_PushHappyPath(t *testing.T) {
	srv := newFakeServer(t)
	withClientHostname(t, "cli-host.test")
	withPublishInvocationMetadata(t, "bv push ./dist", "/workspace/project")
	dir := t.TempDir()
	jsonPath := stageEvidenceFixture(t, dir)

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
			"site_id":               "ev123abc",
			"url":                   "https://ev123abc.butverify.dev",
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
			"url":            "https://ev123abc.butverify.dev",
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
	rc := runEvidence(context.Background(), globalContext{w: w},
		[]string{"--from", jsonPath, "--push"})
	if rc != 0 {
		t.Fatalf("rc: %d  stdout=%s", rc, stdout.String())
	}
	if seenTemplate != "evidence" {
		t.Errorf("expected template=evidence on POST /v1/sites, got %q", seenTemplate)
	}
	assertPublishMetadata(t, createBody, publishSourcePath(jsonPath), "cli-host.test")
	assertPublishMetadata(t, finalizeBody, publishSourcePath(jsonPath), "cli-host.test")
	if got := finalizeBody["upload_id"]; got != createBody["upload_id"] {
		t.Errorf("finalize upload_id=%v, want create upload_id %v", got, createBody["upload_id"])
	}
	if !strings.Contains(stdout.String(), `"template": "evidence"`) {
		t.Errorf("response should echo template: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"url": "https://ev123abc.butverify.dev"`) {
		t.Errorf("expected URL in response: %s", stdout.String())
	}
	// Ephemeral cleanup verification: walk os.TempDir() for leftover
	// evidence dirs that contain our title. (We allow other tests'
	// ephemerals to be present; we only assert OUR run's title isn't
	// hanging around.)
	tmp := os.TempDir()
	entries, _ := os.ReadDir(tmp)
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), ".evidence-") {
			continue
		}
		idx, err := os.ReadFile(filepath.Join(tmp, e.Name(), "index.html"))
		if err == nil && strings.Contains(string(idx), "Evidence test") {
			t.Errorf("ephemeral evidence tmpdir leaked after happy-path: %s", filepath.Join(tmp, e.Name()))
			_ = os.RemoveAll(filepath.Join(tmp, e.Name()))
		}
	}
}

func TestEvidence_PushModeLocalServesRenderedOutput(t *testing.T) {
	localServer := withFakeLocalServer(t)
	dir := t.TempDir()
	jsonPath := stageEvidenceFixture(t, dir)
	isolatedConfigPath(t)
	w, stdout, _ := newJSONWriter(t)
	rc := runEvidence(context.Background(), globalContext{w: w}, []string{"--from", jsonPath, "--push", "--mode", "local"})
	if rc != 0 {
		t.Fatalf("rc=%d stdout=%s", rc, stdout.String())
	}
	if !strings.Contains(localServer.IndexHTML, "Evidence test") {
		t.Fatalf("local server did not receive rendered evidence: %s", localServer.IndexHTML[:min(200, len(localServer.IndexHTML))])
	}
}

// TestEvidence_NonRolloutBadRequestPassesThrough makes sure the EV-E-8
// classifier doesn't rewrite OTHER 400 conditions (like a malformed
// upload_id). Spec: "Do NOT rewrite the message for ANY other 400
// condition."
func TestEvidence_NonRolloutBadRequestPassesThrough(t *testing.T) {
	srv := newFakeServer(t)
	dir := t.TempDir()
	jsonPath := stageEvidenceFixture(t, dir)

	srv.create = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "BadRequest",
				"message": "upload_id must match [A-Za-z0-9_-]{1,128}",
			},
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	w, stdout, _ := newJSONWriter(t)
	rc := runEvidence(context.Background(), globalContext{w: w},
		[]string{"--from", jsonPath, "--push"})
	if rc == 0 {
		t.Fatal("expected non-zero rc on 400")
	}
	if strings.Contains(stdout.String(), "evidence template not yet enabled") {
		t.Errorf("EV-E-8 wording leaked into a non-rollout 400: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "upload_id must match") {
		t.Errorf("original 400 message should pass through: %s", stdout.String())
	}
}

// newHumanWriter is a small helper for tests that need to assert
// human-readable output (stderr-formatted error lines). The other
// suites use newJSONWriter; we add this for parse-message assertions
// where the JSON envelope is harder to grep.
func newHumanWriter(t *testing.T) (*output.Writer, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	return output.NewWith(output.ModeHuman, &stdout, &stderr), &stdout, &stderr
}

// stageEvidenceFixtureMulti writes a manifest with N PNG items into dir and
// returns the path of the JSON file. Each item references a PNG named
// shotN.png where N is 1-based.
func stageEvidenceFixtureMulti(t *testing.T, dir string, count int) string {
	t.Helper()
	pngBytes := makeTestPNG(t)
	items := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		fname := fmt.Sprintf("shot%d.png", i)
		if err := os.WriteFile(filepath.Join(dir, fname), pngBytes, 0o644); err != nil {
			t.Fatalf("write png %d: %v", i, err)
		}
		items = append(items, fmt.Sprintf(`{
			"src": "./%s",
			"title": "Step %d",
			"description": "Description for step %d",
			"sequence": %d
		}`, fname, i, i, i))
	}
	manifest := fmt.Sprintf(`{
		"title": "Multi-item evidence",
		"items": [%s]
	}`, strings.Join(items, ",\n"))
	jsonPath := filepath.Join(dir, "evidence.json")
	if err := os.WriteFile(jsonPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}
	return jsonPath
}

// chdirTemp changes CWD to a new temp dir for the duration of the test
// and returns the dir path.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	return dir
}

// TestEvidence_ValidateWorkflow_HappyPath tests the full validation loop:
// start session with a 2-item manifest, validate item 1, validate item 2,
// expect push call.
func TestEvidence_ValidateWorkflow_HappyPath(t *testing.T) {
	srv := newFakeServer(t)
	stagingURL := ""
	srv.create = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"site_id":               "ev-val-123",
			"url":                   "https://ev-val-123.butverify.dev",
			"expires_at":            "2026-05-10T10:00:00Z",
			"upload_token":          "use_installation_token",
			"manifest_url":          "x",
			"status":                "creating",
			"idempotent":            false,
			"upload_url":            stagingURL,
			"upload_max_bytes":      100 * 1024 * 1024,
			"upload_url_expires_at": "2026-05-10T10:15:00Z",
		})
	}
	srv.finalize = func(w http.ResponseWriter, r *http.Request, siteID string) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"site_id":        siteID,
			"status":         "active",
			"url":            "https://ev-val-123.butverify.dev",
			"manifest_url":   "x",
			"expires_at":     "2026-05-10T10:00:00Z",
			"manifest_sha":   strings.Repeat("b", 64),
			"last_pushed_at": "2026-05-10T10:00:00Z",
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

	// Stage everything in CWD (the session uses CWD-relative paths).
	dir := chdirTemp(t)
	setupConfig(t, server.URL)
	jsonPath := stageEvidenceFixtureMulti(t, dir, 2)

	ctx := context.Background()

	// Step 1: start the validation workflow.
	w1, stdout1, _ := newJSONWriter(t)
	rc1 := runEvidence(ctx, globalContext{w: w1}, []string{"--from", jsonPath})
	if rc1 != 0 {
		t.Fatalf("step 1 rc=%d stdout=%s", rc1, stdout1.String())
	}
	out1 := stdout1.String()
	if !strings.Contains(out1, `"action": "validate_item"`) {
		t.Errorf("expected validate_item action in stdout: %s", out1)
	}
	if !strings.Contains(out1, `"item_index": 1`) {
		t.Errorf("expected item_index 1 in stdout: %s", out1)
	}
	if !strings.Contains(out1, `"item_count": 2`) {
		t.Errorf("expected item_count 2 in stdout: %s", out1)
	}
	// Session file should exist.
	if _, err := os.Stat(evidenceSessionPath); err != nil {
		t.Fatalf("session file missing after step 1: %v", err)
	}

	// Step 2: validate item 1 → should prompt for item 2.
	w2, stdout2, _ := newJSONWriter(t)
	rc2 := runEvidence(ctx, globalContext{w: w2}, []string{"validate", "--item", "1"})
	if rc2 != 0 {
		t.Fatalf("step 2 rc=%d stdout=%s", rc2, stdout2.String())
	}
	out2 := stdout2.String()
	if !strings.Contains(out2, `"item_index": 2`) {
		t.Errorf("expected item_index 2 after validating item 1: %s", out2)
	}
	// Session should still exist (not all items validated yet).
	if _, err := os.Stat(evidenceSessionPath); err != nil {
		t.Fatalf("session file missing after step 2: %v", err)
	}

	// Step 3: validate item 2 → should publish.
	w3, stdout3, _ := newJSONWriter(t)
	rc3 := runEvidence(ctx, globalContext{w: w3}, []string{"validate", "--item", "2"})
	if rc3 != 0 {
		t.Fatalf("step 3 rc=%d stdout=%s", rc3, stdout3.String())
	}
	out3 := stdout3.String()
	if !strings.Contains(out3, `"url": "https://ev-val-123.butverify.dev"`) {
		t.Errorf("expected URL in publish output: %s", out3)
	}
	// Session file should be deleted after successful publish.
	if _, err := os.Stat(evidenceSessionPath); !os.IsNotExist(err) {
		t.Errorf("session file should be deleted after publish; stat err=%v", err)
	}
}

// TestEvidence_ValidateWorkflow_NoSession verifies that `bv evidence validate`
// with no active session returns a non-zero exit code.
func TestEvidence_ValidateWorkflow_NoSession(t *testing.T) {
	chdirTemp(t)
	w, _, _ := newJSONWriter(t)
	rc := runEvidence(context.Background(), globalContext{w: w}, []string{"validate", "--item", "1"})
	if rc == 0 {
		t.Errorf("expected non-zero rc when no session exists, got %d", rc)
	}
}

// TestEvidence_ValidateWorkflow_BadIndex verifies that out-of-range --item
// values and zero/negative indices return a non-zero exit code.
func TestEvidence_ValidateWorkflow_BadIndex(t *testing.T) {
	dir := chdirTemp(t)
	jsonPath := stageEvidenceFixtureMulti(t, dir, 2)
	ctx := context.Background()
	isolatedConfigPath(t)

	// Start a session first.
	w0, _, _ := newJSONWriter(t)
	if rc := runEvidence(ctx, globalContext{w: w0}, []string{"--from", jsonPath}); rc != 0 {
		t.Fatalf("setup session rc=%d", rc)
	}

	cases := []struct {
		name string
		item string
	}{
		{"zero", "0"},
		{"out_of_range", "99"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w, _, _ := newJSONWriter(t)
			rc := runEvidence(ctx, globalContext{w: w}, []string{"validate", "--item", tc.item})
			if rc == 0 {
				t.Errorf("expected non-zero rc for --item %s, got 0", tc.item)
			}
		})
	}
}
