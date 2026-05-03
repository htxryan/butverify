package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWith(ModeJSON, &stdout, &stderr)
	if !w.IsJSON() {
		t.Error("IsJSON should be true in JSON mode")
	}
	if err := w.JSON(map[string]string{"hello": "world"}); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var out map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["hello"] != "world" {
		t.Errorf("unexpected: %+v", out)
	}
	// Human(), Status(), and Progress() should be no-ops in JSON mode.
	w.Human("ignored")
	w.Status("ignored")
	w.Progress("ignored")
	if strings.Contains(stdout.String(), "ignored") {
		t.Error("Human should not write in JSON mode")
	}
	if stderr.Len() != 0 {
		t.Error("Status should not write in JSON mode")
	}
}

func TestHumanMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWith(ModeHuman, &stdout, &stderr)
	if w.IsJSON() {
		t.Error("IsJSON should be false")
	}
	w.Human("hello %s", "world")
	if !strings.HasPrefix(stdout.String(), "hello world") {
		t.Errorf("stdout: %q", stdout.String())
	}
	w.Status("uploading...")
	if !strings.Contains(stderr.String(), "uploading") {
		t.Errorf("stderr: %q", stderr.String())
	}
	stderr.Reset()
	w.Progress("bundling...")
	if got := stderr.String(); got != "bundling...\n" {
		t.Errorf("progress fallback stderr: %q", got)
	}
	// JSON() is a no-op in human mode.
	stdout.Reset()
	if err := w.JSON(map[string]string{"x": "y"}); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if stdout.Len() != 0 {
		t.Error("JSON should be no-op in human mode")
	}
}

func TestTTYModeRedrawsProgressAndClearsBeforeHumanOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWithTTY(ModeHuman, &stdout, &stderr)
	w.Progress("[1/4] first")
	w.Progress("[2/4] second")
	w.Human("done")

	if got := stderr.String(); got != "\r\033[2K[1/4] first\r\033[2K[2/4] second\r\033[2K\n" {
		t.Fatalf("stderr: %q", got)
	}
	if got := stdout.String(); got != "done\n" {
		t.Fatalf("stdout: %q", got)
	}
}

func TestTTYModeKeepsStatusLinesNewlineDelimited(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWithTTY(ModeHuman, &stdout, &stderr)
	w.Status("first")
	w.Status("second")

	if got := stderr.String(); got != "first\nsecond\n" {
		t.Fatalf("stderr: %q", got)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout: %q", stdout.String())
	}
}

func TestTTYModeClearsBeforeErrorOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWithTTY(ModeHuman, &stdout, &stderr)
	w.Progress("[1/4] first")
	w.Error(ErrorEnvelope{Code: "BAD_REQUEST", Message: "missing field"})

	if got := stderr.String(); got != "\r\033[2K[1/4] first\r\033[2K\nbv: error BAD_REQUEST: missing field\n" {
		t.Fatalf("stderr: %q", got)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout: %q", stdout.String())
	}
}

func TestErrorEnvelopeJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWith(ModeJSON, &stdout, &stderr)
	w.Error(ErrorEnvelope{Code: "PAYMENT_REQUIRED", Message: "quota", RequestID: "req_x"})
	var out struct {
		Error ErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "PAYMENT_REQUIRED" {
		t.Errorf("code: %s", out.Error.Code)
	}
}

func TestErrorEnvelopeHuman(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWith(ModeHuman, &stdout, &stderr)
	w.Error(ErrorEnvelope{Code: "BAD_REQUEST", Message: "missing field"})
	if stdout.Len() != 0 {
		t.Error("human error goes to stderr, not stdout")
	}
	if !strings.Contains(stderr.String(), "BAD_REQUEST") {
		t.Errorf("stderr: %q", stderr.String())
	}
}
