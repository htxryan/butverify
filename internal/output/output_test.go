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
	// Human() and Status() should be no-ops in JSON mode.
	w.Human("ignored")
	w.Status("ignored")
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
	// JSON() is a no-op in human mode.
	stdout.Reset()
	if err := w.JSON(map[string]string{"x": "y"}); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if stdout.Len() != 0 {
		t.Error("JSON should be no-op in human mode")
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
