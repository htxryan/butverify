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
// Stdlib only — the JSON path stays dependency-free and the styled human
// output uses bare ANSI escape codes via the Styler. No third-party color
// library so the binary stays tight.
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
	humanTTY       bool // true when stdout is an interactive terminal
	statusTTY      bool // true when stderr is an interactive terminal
	progressActive bool
	stdoutStyle    *Styler // styling applied to stdout (Human/Section/KV/Success)
	statusStyle    *Styler // styling applied to stderr (Status/Hint/WarnStatus)
}

// New constructs a Writer bound to os.Stdout / os.Stderr.
func New(mode Mode) *Writer {
	stdoutTTY := isTerminal(os.Stdout)
	stderrTTY := isTerminal(os.Stderr)
	return &Writer{
		mode:        mode,
		stdout:      os.Stdout,
		stderr:      os.Stderr,
		humanTTY:    stdoutTTY,
		statusTTY:   stderrTTY,
		stdoutStyle: newStyler(stdoutTTY),
		statusStyle: newStyler(stderrTTY),
	}
}

// NewWith allows tests to inject custom sinks.
func NewWith(mode Mode, stdout, stderr io.Writer) *Writer {
	return &Writer{
		mode:        mode,
		stdout:      stdout,
		stderr:      stderr,
		stdoutStyle: newStyler(false),
		statusStyle: newStyler(false),
	}
}

// NewWithTTY allows tests to force terminal-style stderr rendering.
// stdoutTTY remains false so existing assertions of plain stdout content
// keep matching; the dedicated push-progress redraw test only inspects
// stderr.
func NewWithTTY(mode Mode, stdout, stderr io.Writer) *Writer {
	return &Writer{
		mode:        mode,
		stdout:      stdout,
		stderr:      stderr,
		statusTTY:   true,
		stdoutStyle: newStyler(false),
		statusStyle: newStyler(true),
	}
}

// NewStyled is a test helper that forces both stdout and stderr to be
// treated as terminals so styling appears in captured buffers.
func NewStyled(mode Mode, stdout, stderr io.Writer) *Writer {
	return &Writer{
		mode:        mode,
		stdout:      stdout,
		stderr:      stderr,
		humanTTY:    true,
		statusTTY:   true,
		stdoutStyle: newStyler(true),
		statusStyle: newStyler(true),
	}
}

// IsJSON returns true when the writer is in --json mode.
func (w *Writer) IsJSON() bool { return w.mode == ModeJSON }

// IsHumanTTY reports whether the writer is in human mode and the stderr
// sink is an interactive terminal. Used to decide whether to issue
// interactive prompts (the install-skill hook consent flow uses this).
func (w *Writer) IsHumanTTY() bool { return w.mode == ModeHuman && w.statusTTY }

// StdoutStyler returns the Styler attached to stdout. Callers that need
// inline styling for table cells / structured human output reach through
// this rather than calling the writer methods directly.
func (w *Writer) StdoutStyler() *Styler { return w.stdoutStyle }

// StderrStyler returns the Styler attached to stderr.
func (w *Writer) StderrStyler() *Styler { return w.statusStyle }

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

// Success prints a single-line "ok" message to stdout. In a TTY it prepends
// a green check icon; in non-TTY/NO_COLOR contexts it prints the bare
// message so existing string-match tests keep passing.
func (w *Writer) Success(format string, args ...any) {
	if w.mode != ModeHuman {
		return
	}
	w.clearProgressLine()
	msg := fmt.Sprintf(format, args...)
	line := w.stdoutStyle.PrefixWithIcon(w.stdoutStyle.IconCheck(), msg)
	fmt.Fprintln(w.stdout, line)
}

// Section prints a bold heading on stdout, separated from prior output by
// a blank line. Mirrors the visual block-grouping used in `git status` /
// `gh pr view` so multiple groups in one command (e.g. publish summary +
// metadata) read at a glance.
func (w *Writer) Section(title string) {
	if w.mode != ModeHuman {
		return
	}
	w.clearProgressLine()
	fmt.Fprintln(w.stdout)
	fmt.Fprintln(w.stdout, w.stdoutStyle.Heading(title))
}

// kvKeyWidth is the fixed-width label column used by KV — keys plus the
// trailing colon are left-padded to this many characters so successive KV
// calls stack vertically. The width matches the historical hand-rolled
// publish/whoami layouts (e.g. "Open URL:   value", "Site ID:    value")
// so log-grepping integrations keep matching after the styling refactor.
const kvKeyWidth = 12

// KV prints an aligned "  Key:    value" line on stdout. The key column
// (key + ":" left-padded to kvKeyWidth) is dimmed in TTY mode so the eye
// lands on the value first; in non-TTY mode the line is byte-identical
// to the previous hand-rolled output.
func (w *Writer) KV(key, value string) {
	if w.mode != ModeHuman {
		return
	}
	w.clearProgressLine()
	col := fmt.Sprintf("%-*s", kvKeyWidth, key+":")
	keyStyled := w.stdoutStyle.Dim(col)
	fmt.Fprintf(w.stdout, "  %s%s\n", keyStyled, value)
}

// KVf is sprintf sugar over KV so callers don't have to wrap fmt.Sprintf
// at the call site.
func (w *Writer) KVf(key, format string, args ...any) {
	w.KV(key, fmt.Sprintf(format, args...))
}

// Hint prints a dimmed informational line on stderr. Used for next-step
// pointers like "Run `bv push <dir>` to publish" that are useful but not
// part of the primary stdout payload.
func (w *Writer) Hint(format string, args ...any) {
	if w.mode != ModeHuman {
		return
	}
	w.clearProgressLine()
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(w.stderr, w.statusStyle.Dim(msg))
}

// Note prints a dimmed informational line on stdout. Use sparingly —
// most informational output should go through Hint() so a piped consumer
// of stdout sees only data.
func (w *Writer) Note(format string, args ...any) {
	if w.mode != ModeHuman {
		return
	}
	w.clearProgressLine()
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(w.stdout, w.stdoutStyle.Dim(msg))
}

// WarnStatus prints a yellow-styled warning line on stderr.
func (w *Writer) WarnStatus(format string, args ...any) {
	if w.mode != ModeHuman {
		return
	}
	w.clearProgressLine()
	msg := fmt.Sprintf(format, args...)
	icon := w.statusStyle.IconWarn()
	line := w.statusStyle.PrefixWithIcon(icon, msg)
	fmt.Fprintln(w.stderr, line)
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
// in human mode prints a formatted error line to stderr. The literal text
// of the human line is identical to the pre-styling format ("bv: error
// CODE: message") so existing log-grepping integrations keep working; in
// TTY mode the prefix is colored red so the eye lands on it instantly.
func (w *Writer) Error(env ErrorEnvelope) {
	w.clearProgressLine()
	if w.mode == ModeJSON {
		_ = json.NewEncoder(w.stdout).Encode(struct {
			Error ErrorEnvelope `json:"error"`
		}{Error: env})
		return
	}
	prefix := w.statusStyle.Red("bv: error")
	if env.RequestID != "" {
		fmt.Fprintf(w.stderr, "%s %s: %s (request_id=%s)\n",
			prefix, env.Code, env.Message, env.RequestID)
		return
	}
	fmt.Fprintf(w.stderr, "%s %s: %s\n", prefix, env.Code, env.Message)
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
