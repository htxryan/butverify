// Package output handles --json vs human output for the bv CLI.
//
// The contract is:
//   - `--json` produces machine-readable structured output on stdout (one
//     JSON object per command invocation; errors share the same envelope
//     as the server-side ApiError).
//   - Default mode produces human-readable text on stdout, with status
//     messages on stderr so a piped consumer (e.g. `bv ls | wc -l`) sees
//     only the data.
//
// Stdlib only — no lipgloss in this layer so the JSON path stays
// dependency-free. The styled human output lives in the caller (cmd/bv).
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Mode is whether the CLI is in --json or human mode.
type Mode int

const (
	ModeHuman Mode = iota
	ModeJSON
)

// Writer is the sink for command output. Bind once at command init via
// New() then pass through the call tree.
type Writer struct {
	mode           Mode
	stdout         io.Writer
	stderr         io.Writer
	statusTTY      bool
	progressActive bool
}

// New constructs a Writer bound to os.Stdout / os.Stderr.
func New(mode Mode) *Writer {
	return &Writer{mode: mode, stdout: os.Stdout, stderr: os.Stderr, statusTTY: isTerminal(os.Stderr)}
}

// NewWith allows tests to inject custom sinks.
func NewWith(mode Mode, stdout, stderr io.Writer) *Writer {
	return &Writer{mode: mode, stdout: stdout, stderr: stderr}
}

// NewWithTTY allows tests to force terminal-style stderr rendering.
func NewWithTTY(mode Mode, stdout, stderr io.Writer) *Writer {
	return &Writer{mode: mode, stdout: stdout, stderr: stderr, statusTTY: true}
}

// IsJSON returns true when the writer is in --json mode.
func (w *Writer) IsJSON() bool { return w.mode == ModeJSON }

// JSON writes a JSON-encoded object to stdout. In human mode this is a no-op
// — the caller is expected to use Human() instead.
func (w *Writer) JSON(v any) error {
	if w.mode != ModeJSON {
		return nil
	}
	enc := json.NewEncoder(w.stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Human writes formatted text to stdout in human mode (no-op in JSON mode).
func (w *Writer) Human(format string, args ...any) {
	if w.mode != ModeHuman {
		return
	}
	w.clearProgressLine()
	fmt.Fprintf(w.stdout, format, args...)
	if len(format) > 0 && format[len(format)-1] != '\n' {
		fmt.Fprintln(w.stdout)
	}
}

// Status writes a newline-delimited status line to stderr in human mode.
func (w *Writer) Status(format string, args ...any) {
	if w.mode != ModeHuman {
		return
	}
	w.clearProgressLine()
	fmt.Fprintf(w.stderr, format, args...)
	if len(format) > 0 && format[len(format)-1] != '\n' {
		fmt.Fprintln(w.stderr)
	}
}

// Progress redraws an in-place status line when stderr is a terminal. When
// stderr is not a terminal it falls back to deterministic newline-delimited
// status output for captured logs and tests.
func (w *Writer) Progress(format string, args ...any) {
	if w.mode != ModeHuman {
		return
	}
	if w.statusTTY {
		fmt.Fprint(w.stderr, "\r\033[2K")
		fmt.Fprintf(w.stderr, format, args...)
		w.progressActive = true
		return
	}
	w.Status(format, args...)
}

// ErrorEnvelope mirrors the server's ApiError shape so error output is
// consistent across the wire and the local CLI.
type ErrorEnvelope struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	// HTTP status code (when known) so a piped consumer can branch on the
	// numeric class without parsing message text.
	Status int `json:"status,omitempty"`
}

// Error writes an error envelope. In JSON mode emits a structured object;
// in human mode prints a formatted error line to stderr.
func (w *Writer) Error(env ErrorEnvelope) {
	w.clearProgressLine()
	if w.mode == ModeJSON {
		_ = json.NewEncoder(w.stdout).Encode(struct {
			Error ErrorEnvelope `json:"error"`
		}{Error: env})
		return
	}
	if env.RequestID != "" {
		fmt.Fprintf(w.stderr, "bv: error %s: %s (request_id=%s)\n", env.Code, env.Message, env.RequestID)
		return
	}
	fmt.Fprintf(w.stderr, "bv: error %s: %s\n", env.Code, env.Message)
}

func (w *Writer) clearProgressLine() {
	if !w.statusTTY || !w.progressActive {
		return
	}
	fmt.Fprint(w.stderr, "\r\033[2K\n")
	w.progressActive = false
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
