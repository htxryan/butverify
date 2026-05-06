// Tests for `bv review {list,get,acknowledge,request}` (pebble-xu6b).

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestReview_NoSubcommand_UsageError(t *testing.T) {
	w, _, _ := newJSONWriter(t)
	rc := runReview(context.Background(), globalContext{w: w}, nil)
	if rc != 2 {
		t.Fatalf("rc: %d", rc)
	}
}

func TestReview_UnknownSubcommand_UsageError(t *testing.T) {
	w, _, _ := newJSONWriter(t)
	rc := runReview(context.Background(), globalContext{w: w}, []string{"nope"})
	if rc != 2 {
		t.Fatalf("rc: %d", rc)
	}
}

func TestReview_List_JSON(t *testing.T) {
	srv := newFakeServer(t)
	srv.listReviews = func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("unacknowledged"); got != "true" {
			t.Errorf("unacknowledged query: %q, want true", got)
		}
		if got := r.URL.Query().Get("site_id"); got != "abcd1234" {
			t.Errorf("site_id query: %q, want abcd1234", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"reviews": []map[string]any{
				{
					"review_id":        "rev_a1",
					"site_id":          "abcd1234",
					"reviewer_login":   "alice",
					"submitted_at":     "2026-05-06T10:00:00Z",
					"acknowledged_at":  nil,
					"annotation_count": 3,
					"status":           "submitted",
				},
				{
					"review_id":        "rev_b2",
					"site_id":          "abcd1234",
					"reviewer_login":   "bob",
					"submitted_at":     "2026-05-06T11:00:00Z",
					"acknowledged_at":  nil,
					"annotation_count": 1,
					"status":           "submitted",
				},
			},
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	w, stdout, _ := newJSONWriter(t)
	rc := runReview(context.Background(), globalContext{w: w}, []string{"list", "--unacknowledged", "--site", "abcd1234"})
	if rc != 0 {
		t.Fatalf("rc: %d stdout=%s", rc, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"review_id": "rev_a1"`) {
		t.Errorf("stdout: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"reviewer_login": "bob"`) {
		t.Errorf("stdout missing 2nd review: %s", stdout.String())
	}
}

// EV2-E-8: --format=ids must produce hook-friendly `<site_id>#<review_id>`
// lines on stdout — no headers, no JSON, deterministic ordering matches
// server response.
func TestReview_List_FormatIDs(t *testing.T) {
	srv := newFakeServer(t)
	srv.listReviews = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"reviews": []map[string]any{
				{"review_id": "rev_a", "site_id": "site1", "reviewer_login": "alice", "submitted_at": "x", "annotation_count": 1, "status": "submitted"},
				{"review_id": "rev_b", "site_id": "site2", "reviewer_login": "bob", "submitted_at": "x", "annotation_count": 1, "status": "submitted"},
			},
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	out := captureStdout(t, func() {
		w, _, _ := newJSONWriter(t)
		rc := runReview(context.Background(), globalContext{w: w}, []string{"list", "--format=ids"})
		if rc != 0 {
			t.Fatalf("rc: %d", rc)
		}
	})
	got := strings.TrimRight(out, "\n")
	want := "site1#rev_a\nsite2#rev_b"
	if got != want {
		t.Fatalf("ids output: %q, want %q", got, want)
	}
}

// Empty result must produce zero output bytes so a Stop hook can branch
// on `[ -s output ]`.
func TestReview_List_FormatIDs_Empty(t *testing.T) {
	srv := newFakeServer(t)
	srv.listReviews = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"reviews": []any{}})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	out := captureStdout(t, func() {
		w, _, _ := newJSONWriter(t)
		rc := runReview(context.Background(), globalContext{w: w}, []string{"list", "--format=ids", "--unacknowledged"})
		if rc != 0 {
			t.Fatalf("rc: %d", rc)
		}
	})
	if out != "" {
		t.Fatalf("expected empty stdout, got %q", out)
	}
}

func TestReview_List_BadFormat(t *testing.T) {
	w, _, _ := newJSONWriter(t)
	rc := runReview(context.Background(), globalContext{w: w}, []string{"list", "--format=yaml"})
	if rc != 2 {
		t.Fatalf("rc: %d", rc)
	}
}

// EV2-E-8 hard timeout: --format=ids enforces a 2s timeout. We assert the
// CLI returns non-zero and does NOT hang when the server stalls.
func TestReview_List_FormatIDs_Timeout(t *testing.T) {
	stall := make(chan struct{})
	t.Cleanup(func() { close(stall) })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until the test ends so the 2s ctx deadline fires first.
		select {
		case <-stall:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	setupConfig(t, srv.URL)

	// Override the constant via a tighter local context — we pass our own
	// ctx so the test runs fast. The CLI applies WithTimeout(ctx, 2s) on
	// top, but a parent ctx already shorter wins.
	parent, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	w, _, _ := newJSONWriter(t)
	rc := runReview(parent, globalContext{w: w}, []string{"list", "--format=ids"})
	if rc == 0 {
		t.Fatalf("expected non-zero rc on timeout")
	}
}

func TestReview_Get_JSON(t *testing.T) {
	srv := newFakeServer(t)
	srv.getReview = func(w http.ResponseWriter, r *http.Request, reviewID string) {
		if reviewID != "rev_xyz" {
			t.Errorf("review_id: %s", reviewID)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"review_id":        "rev_xyz",
			"site_id":          "abcd1234",
			"reviewer_login":   "alice",
			"submitted_at":     "2026-05-06T10:00:00Z",
			"acknowledged_at":  nil,
			"annotation_count": 1,
			"status":           "submitted",
			"annotations": []map[string]any{
				{
					"annotation_id": "ann_1",
					"type":          "site_comment",
					"item_index":    nil,
					"comment":       "looks great",
					"region_shape":  nil,
					"region_x":      nil,
					"region_y":      nil,
					"region_width":  nil,
					"region_height": nil,
					"text_scope":    nil,
					"char_start":    nil,
					"char_end":      nil,
					"text_snippet":  nil,
					"created_at":    "2026-05-06T10:00:00Z",
				},
			},
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	w, stdout, _ := newJSONWriter(t)
	rc := runReview(context.Background(), globalContext{w: w}, []string{"get", "rev_xyz"})
	if rc != 0 {
		t.Fatalf("rc: %d stdout=%s", rc, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"annotation_id": "ann_1"`) {
		t.Errorf("stdout: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"comment": "looks great"`) {
		t.Errorf("stdout: %s", stdout.String())
	}
}

// EV2-U-9 (explicit typed columns): `bv review get` must surface the
// region_*, char_*, text_scope, text_snippet typed columns the server
// emits — not collapse them into a metadata blob. Exercises both
// image_region and text_highlight annotation types.
func TestReview_Get_TypedColumns(t *testing.T) {
	srv := newFakeServer(t)
	srv.getReview = func(w http.ResponseWriter, r *http.Request, reviewID string) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"review_id":        reviewID,
			"site_id":          "abcd1234",
			"reviewer_login":   "alice",
			"submitted_at":     "2026-05-06T10:00:00Z",
			"acknowledged_at":  nil,
			"annotation_count": 2,
			"status":           "submitted",
			"annotations": []map[string]any{
				{
					"annotation_id": "ann_img",
					"type":          "image_region",
					"item_index":    2,
					"comment":       "off-center",
					"region_shape":  "circle",
					"region_x":      0.48,
					"region_y":      0.52,
					"region_width":  0.08,
					"region_height": 0.08,
					"text_scope":    nil,
					"char_start":    nil,
					"char_end":      nil,
					"text_snippet":  nil,
					"created_at":    "2026-05-06T10:00:00Z",
				},
				{
					"annotation_id": "ann_txt",
					"type":          "text_highlight",
					"item_index":    0,
					"comment":       "misleading",
					"region_shape":  nil,
					"region_x":      nil,
					"region_y":      nil,
					"region_width":  nil,
					"region_height": nil,
					"text_scope":    "item",
					"char_start":    44,
					"char_end":      89,
					"text_snippet":  "all states pass automated tests",
					"created_at":    "2026-05-06T10:00:00Z",
				},
			},
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	w, stdout, _ := newJSONWriter(t)
	rc := runReview(context.Background(), globalContext{w: w}, []string{"get", "rev_typed"})
	if rc != 0 {
		t.Fatalf("rc: %d stdout=%s", rc, stdout.String())
	}
	for _, want := range []string{
		`"region_shape": "circle"`,
		`"region_x": 0.48`,
		`"region_height": 0.08`,
		`"text_scope": "item"`,
		`"char_start": 44`,
		`"char_end": 89`,
		`"text_snippet": "all states pass automated tests"`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
}

// EV2-E-4 acknowledged_at round-trip: server-sent `null` for an
// unacknowledged review must round-trip as JSON `null` (not be silently
// dropped via omitempty). A consumer that branches on key presence would
// otherwise miss unacknowledged reviews.
func TestReview_List_AcknowledgedAtNullRoundTrips(t *testing.T) {
	srv := newFakeServer(t)
	srv.listReviews = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"reviews": []map[string]any{
				{
					"review_id":        "rev_a",
					"site_id":          "site1",
					"reviewer_login":   "alice",
					"submitted_at":     "2026-05-06T10:00:00Z",
					"acknowledged_at":  nil,
					"annotation_count": 1,
					"status":           "submitted",
				},
			},
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	w, stdout, _ := newJSONWriter(t)
	rc := runReview(context.Background(), globalContext{w: w}, []string{"list"})
	if rc != 0 {
		t.Fatalf("rc: %d", rc)
	}
	if !strings.Contains(stdout.String(), `"acknowledged_at": null`) {
		t.Errorf("expected `\"acknowledged_at\": null` round-trip, got: %s", stdout.String())
	}
}

func TestReview_Get_RequiresArg(t *testing.T) {
	w, _, _ := newJSONWriter(t)
	rc := runReview(context.Background(), globalContext{w: w}, []string{"get"})
	if rc != 2 {
		t.Fatalf("rc: %d", rc)
	}
}

func TestReview_Get_NotFound_MapsToExitCode(t *testing.T) {
	srv := newFakeServer(t)
	srv.getReview = func(w http.ResponseWriter, r *http.Request, reviewID string) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": "NOT_FOUND", "message": "unknown review"},
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	w, _, _ := newJSONWriter(t)
	rc := runReview(context.Background(), globalContext{w: w}, []string{"get", "rev_missing"})
	if rc != 6 {
		t.Fatalf("rc: %d (want 6 for NOT_FOUND)", rc)
	}
}

func TestReview_Acknowledge_JSON(t *testing.T) {
	srv := newFakeServer(t)
	srv.ackReview = func(w http.ResponseWriter, r *http.Request, reviewID string) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"review_id":            reviewID,
			"status":               "acknowledged",
			"acknowledged_at":      "2026-05-06T12:00:00Z",
			"already_acknowledged": false,
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	w, stdout, _ := newJSONWriter(t)
	rc := runReview(context.Background(), globalContext{w: w}, []string{"acknowledge", "rev_a"})
	if rc != 0 {
		t.Fatalf("rc: %d", rc)
	}
	if !strings.Contains(stdout.String(), `"already_acknowledged": false`) {
		t.Errorf("stdout: %s", stdout.String())
	}
}

// EV2-E-6 idempotency: a second acknowledge call must still exit 0 and
// surface `already_acknowledged: true` so callers can branch on intent
// without parsing message text.
func TestReview_Acknowledge_Idempotent(t *testing.T) {
	srv := newFakeServer(t)
	srv.ackReview = func(w http.ResponseWriter, r *http.Request, reviewID string) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"review_id":            reviewID,
			"status":               "acknowledged",
			"acknowledged_at":      "2026-05-06T12:00:00Z",
			"already_acknowledged": true,
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	w, stdout, _ := newJSONWriter(t)
	rc := runReview(context.Background(), globalContext{w: w}, []string{"acknowledge", "rev_a"})
	if rc != 0 {
		t.Fatalf("rc: %d (idempotent retry must exit 0)", rc)
	}
	if !strings.Contains(stdout.String(), `"already_acknowledged": true`) {
		t.Errorf("stdout: %s", stdout.String())
	}
}

func TestReview_Request_Success(t *testing.T) {
	srv := newFakeServer(t)
	var seenBody map[string]any
	srv.requestReview = func(w http.ResponseWriter, r *http.Request, siteID string) {
		if siteID != "abcd1234" {
			t.Errorf("site_id: %s", siteID)
		}
		_ = json.NewDecoder(r.Body).Decode(&seenBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"requested":      true,
			"reviewer_login": "ryanh",
			"notification":   "email_sent",
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	w, stdout, _ := newJSONWriter(t)
	rc := runReview(context.Background(), globalContext{w: w}, []string{"request", "abcd1234", "--to", "ryanh"})
	if rc != 0 {
		t.Fatalf("rc: %d stdout=%s", rc, stdout.String())
	}
	if seenBody["reviewer_login"] != "ryanh" {
		t.Errorf("body: %+v", seenBody)
	}
	if !strings.Contains(stdout.String(), `"notification": "email_sent"`) {
		t.Errorf("stdout: %s", stdout.String())
	}
}

// EV2-N-7: 422 reviewer_email_unknown — CLI must surface the site URL so
// the agent can share manually. Exit code is non-zero (the request did
// NOT result in a notification), but the JSON body the CLI emits matches
// the spec example shape.
func TestReview_Request_UnresolvableEmail(t *testing.T) {
	srv := newFakeServer(t)
	srv.requestReview = func(w http.ResponseWriter, r *http.Request, siteID string) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"requested": false,
			"error":     "reviewer_email_unknown",
			"site_url":  "https://abcd1234.butverify.dev",
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	w, stdout, _ := newJSONWriter(t)
	rc := runReview(context.Background(), globalContext{w: w}, []string{"request", "abcd1234", "--to", "nobody"})
	if rc != exitReviewerUnresolvable {
		t.Fatalf("rc: %d, want %d (exitReviewerUnresolvable)", rc, exitReviewerUnresolvable)
	}
	if !strings.Contains(stdout.String(), `"error": "reviewer_email_unknown"`) {
		t.Errorf("stdout: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"site_url": "https://abcd1234.butverify.dev"`) {
		t.Errorf("stdout missing site_url: %s", stdout.String())
	}
}

// EV2-N-6: duplicate review request returns 409 review_request_already_sent —
// CLI should map to its 409 exit code (7).
func TestReview_Request_AlreadySent(t *testing.T) {
	srv := newFakeServer(t)
	srv.requestReview = func(w http.ResponseWriter, r *http.Request, siteID string) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "review_request_already_sent",
				"message": "review_request_already_sent",
			},
		})
	}
	server := httptest.NewServer(srv.handler())
	defer server.Close()
	setupConfig(t, server.URL)

	w, _, _ := newJSONWriter(t)
	rc := runReview(context.Background(), globalContext{w: w}, []string{"request", "abcd1234", "--to", "ryanh"})
	if rc != 7 {
		t.Fatalf("rc: %d (want 7 for CONFLICT)", rc)
	}
}

func TestReview_Request_RequiresTo(t *testing.T) {
	w, _, _ := newJSONWriter(t)
	rc := runReview(context.Background(), globalContext{w: w}, []string{"request", "abcd1234"})
	if rc != 2 {
		t.Fatalf("rc: %d", rc)
	}
}

func TestReview_Request_RequiresSite(t *testing.T) {
	w, _, _ := newJSONWriter(t)
	rc := runReview(context.Background(), globalContext{w: w}, []string{"request", "--to", "ryanh"})
	if rc != 2 {
		t.Fatalf("rc: %d", rc)
	}
}

// captureStdout swaps os.Stdout for an os.Pipe for the duration of fn,
// returns the captured bytes. Used for `--format=ids` and the human-mode
// JSON paths that bypass the writer (matching `bv manifest`'s pattern).
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, wp, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = wp
	done := make(chan []byte)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()
	defer func() {
		os.Stdout = old
	}()
	fn()
	_ = wp.Close()
	out := <-done
	_ = r.Close()
	return string(out)
}
