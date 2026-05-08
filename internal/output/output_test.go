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

	// In TTY mode the "bv: error" prefix is wrapped in red ANSI codes so a
	// human reader spots failure at a glance; the rest of the line stays
	// byte-identical to the non-TTY format so log scrapers keep working.
	const wantStripped = "\r\x1b[2K[1/4] first\r\x1b[2K\nbv: error BAD_REQUEST: missing field\n"
	got := stderr.String()
	if !strings.Contains(got, "\x1b[31mbv: error\x1b[0m") {
		t.Fatalf("expected red 'bv: error' in TTY mode: %q", got)
	}
	if stripped := stripANSI(got); stripped != wantStripped {
		t.Fatalf("stderr stripped: %q", stripped)
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

func TestSuccessHumanPlainNoTTY(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWith(ModeHuman, &stdout, &stderr)
	w.Success("Published %s", "site")
	// Without a TTY the icon is suppressed so existing grep-based tests
	// keep matching the bare message.
	if got := stdout.String(); got != "Published site\n" {
		t.Fatalf("stdout: %q", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr leaked: %q", stderr.String())
	}
}

func TestSuccessHumanTTYAddsCheckIcon(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewStyled(ModeHuman, &stdout, &stderr)
	w.Success("Published site")
	got := stdout.String()
	if !strings.Contains(got, "✓") {
		t.Fatalf("expected check icon in TTY mode: %q", got)
	}
	if !strings.Contains(got, "Published site") {
		t.Fatalf("expected message in TTY mode: %q", got)
	}
	if stripped := stripANSI(got); stripped != "✓ Published site\n" {
		t.Fatalf("stripped: %q", stripped)
	}
}

func TestSectionAndKVNoTTYAlignsAndStripsStyles(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWith(ModeHuman, &stdout, &stderr)
	w.Section("Metadata")
	w.KV("Site ID", "abcd1234")
	w.KV("Status", "active")
	want := "\nMetadata\n  Site ID:    abcd1234\n  Status:     active\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout mismatch:\nwant=%q\ngot =%q", want, got)
	}
	_ = stderr
}

func TestKVWithTTYDimsKey(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewStyled(ModeHuman, &stdout, &stderr)
	w.KV("Site ID", "abcd1234")
	got := stdout.String()
	if !strings.Contains(got, "\x1b[2m") {
		t.Fatalf("expected dim ANSI in TTY mode: %q", got)
	}
	if !strings.Contains(stripANSI(got), "Site ID") {
		t.Fatalf("expected key in stripped output: %q", got)
	}
	if !strings.Contains(stripANSI(got), "abcd1234") {
		t.Fatalf("expected value in stripped output: %q", got)
	}
	_ = stderr
}

func TestHintGoesToStderrAndIsDimInTTY(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewStyled(ModeHuman, &stdout, &stderr)
	w.Hint("To publish:  bv push %s", "./out")
	if stdout.Len() != 0 {
		t.Fatalf("hints must not write to stdout: %q", stdout.String())
	}
	got := stderr.String()
	if !strings.Contains(got, "\x1b[2m") {
		t.Fatalf("expected dim ANSI on hint in TTY: %q", got)
	}
	if stripped := stripANSI(got); stripped != "To publish:  bv push ./out\n" {
		t.Fatalf("stripped hint: %q", stripped)
	}
}

func TestSuppressedInJSONMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWith(ModeJSON, &stdout, &stderr)
	w.Success("ignored")
	w.Section("ignored")
	w.KV("ignored", "ignored")
	w.Hint("ignored")
	w.Note("ignored")
	w.WarnStatus("ignored")
	if stdout.Len() != 0 {
		t.Fatalf("stdout leaked in JSON mode: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr leaked in JSON mode: %q", stderr.String())
	}
}

func TestStripANSI(t *testing.T) {
	in := "\x1b[31mred\x1b[0m \x1b[1;36mbold-cyan\x1b[0m plain"
	if got := stripANSI(in); got != "red bold-cyan plain" {
		t.Fatalf("strip: %q", got)
	}
}
