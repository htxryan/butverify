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
		"items": [
			{"src": "./shot.png", "title": "First", "description": "shot 1", "sequence": 1}
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

func TestEvidence_RenderOnly_StackedDefault(t *testing.T) {
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
	if !strings.Contains(stdout.String(), `"layout": "stacked"`) {
		t.Errorf("expected default layout=stacked in stdout: %s", stdout.String())
	}
	idx, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if !strings.Contains(string(idx), `data-layout="stacked"`) {
		t.Errorf("rendered html missing stacked layout marker (first 300): %s", idx[:min(300, len(idx))])
	}
	if !strings.Contains(string(idx), "Evidence test") {
		t.Errorf("title missing from index: %s", idx[:min(300, len(idx))])
	}
	if _, err := os.Stat(filepath.Join(outDir, "styles.css")); err != nil {
		t.Errorf("styles.css missing: %v", err)
	}
	// Asset has the deterministic `001-shot.png` prefix from
	// templates.SafeAssetName.
	assetEntries, _ := os.ReadDir(filepath.Join(outDir, "assets"))
	if len(assetEntries) != 1 {
		t.Errorf("expected 1 asset, got %d", len(assetEntries))
	}
}

func TestEvidence_BogusLayoutExit2(t *testing.T) {
	dir := t.TempDir()
	jsonPath := stageEvidenceFixture(t, dir)

	// Use the human writer so the assertion can grep stderr text
	// directly (in JSON mode, the envelope lands on stdout as a
	// quoted string and is harder to read at a glance).
	wHuman, _, hStderr := newHumanWriter(t)
	rc := runEvidence(context.Background(), globalContext{w: wHuman},
		[]string{"--from", jsonPath, "--layout", "bogus", "--out", filepath.Join(dir, "out")})
	if rc != 2 {
		t.Errorf("expected rc=2 for bogus layout, got %d", rc)
	}
	msg := hStderr.String()
	if !strings.Contains(msg, "stacked") || !strings.Contains(msg, "carousel") {
		t.Errorf("error message should list supported layouts: %s", msg)
	}
}

func TestEvidence_CarouselLayout(t *testing.T) {
	dir := t.TempDir()
	jsonPath := stageEvidenceFixture(t, dir)
	outDir := filepath.Join(dir, "out")

	w, _, _ := newJSONWriter(t)
	rc := runEvidence(context.Background(), globalContext{w: w},
		[]string{"--from", jsonPath, "--layout", "carousel", "--out", outDir})
	if rc != 0 {
		t.Fatalf("carousel rc: %d", rc)
	}
	idx, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(idx), `data-layout="carousel"`) {
		t.Errorf("expected data-layout=\"carousel\" in rendered html (first 300): %s", idx[:min(300, len(idx))])
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
// suites use newJSONWriter; we add this only for layout-message
// assertions where the JSON envelope is harder to grep.
func newHumanWriter(t *testing.T) (*output.Writer, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	return output.NewWith(output.ModeHuman, &stdout, &stderr), &stdout, &stderr
}
