package output

import (
	"fmt"
	"os"
	"strings"
)

// Styler turns plain text into ANSI-styled text when the output sink is a
// terminal that supports color. When color is disabled (piped output, NO_COLOR
// set, dumb terminal), every method returns its argument unchanged so the
// caller never has to branch on TTY state at the call site.
type Styler struct {
	color   bool
	unicode bool
}

// ANSI escape codes for color/style. Reset is appended after every styled
// fragment so a single color never bleeds into the next line.
const (
	ansiReset    = "\x1b[0m"
	ansiBold     = "\x1b[1m"
	ansiDim      = "\x1b[2m"
	ansiRed      = "\x1b[31m"
	ansiGreen    = "\x1b[32m"
	ansiYellow   = "\x1b[33m"
	ansiBlue     = "\x1b[34m"
	ansiMagenta  = "\x1b[35m"
	ansiCyan     = "\x1b[36m"
	ansiBoldCyan = "\x1b[1;36m"
)

// newStyler resolves the effective color/unicode capabilities. Honors the
// NO_COLOR convention (no-color.org) and the BV_NO_UNICODE escape hatch for
// environments that mis-render UTF-8 glyphs.
func newStyler(isTTY bool) *Styler {
	color := isTTY
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		color = false
	}
	if t := os.Getenv("TERM"); t == "dumb" {
		color = false
	}
	unicode := true
	if _, ok := os.LookupEnv("BV_NO_UNICODE"); ok {
		unicode = false
	}
	return &Styler{color: color, unicode: unicode}
}

// wrap is the central styling primitive. When color is off we return the
// original string; otherwise we wrap it in the supplied ANSI code and append
// a reset so the styling is scoped to this fragment alone.
func (s *Styler) wrap(code, text string) string {
	if !s.color || code == "" {
		return text
	}
	return code + text + ansiReset
}

func (s *Styler) Bold(text string) string    { return s.wrap(ansiBold, text) }
func (s *Styler) Dim(text string) string     { return s.wrap(ansiDim, text) }
func (s *Styler) Red(text string) string     { return s.wrap(ansiRed, text) }
func (s *Styler) Green(text string) string   { return s.wrap(ansiGreen, text) }
func (s *Styler) Yellow(text string) string  { return s.wrap(ansiYellow, text) }
func (s *Styler) Blue(text string) string    { return s.wrap(ansiBlue, text) }
func (s *Styler) Magenta(text string) string { return s.wrap(ansiMagenta, text) }
func (s *Styler) Cyan(text string) string    { return s.wrap(ansiCyan, text) }

// Heading renders a section title in bold cyan. Used by Section() to give
// the eye a stable anchor at the top of each output block.
func (s *Styler) Heading(text string) string { return s.wrap(ansiBoldCyan, text) }

// IconCheck returns the canonical green check glyph for "succeeded" state.
// The non-color/non-unicode fallbacks are pure ASCII so a captured log
// file (or BV_NO_UNICODE=1 invocation) still parses cleanly. Each Icon*
// returns an EMPTY string when color is disabled — callers concatenate
// with a space prefix only when the icon is non-empty.
func (s *Styler) IconCheck() string {
	if !s.color {
		return ""
	}
	if s.unicode {
		return s.Green("✓")
	}
	return s.Green("[ok]")
}

func (s *Styler) IconCross() string {
	if !s.color {
		return ""
	}
	if s.unicode {
		return s.Red("✗")
	}
	return s.Red("[x]")
}

func (s *Styler) IconWarn() string {
	if !s.color {
		return ""
	}
	if s.unicode {
		return s.Yellow("!")
	}
	return s.Yellow("[!]")
}

func (s *Styler) IconInfo() string {
	if !s.color {
		return ""
	}
	if s.unicode {
		return s.Blue("i")
	}
	return s.Blue("[i]")
}

func (s *Styler) IconArrow() string {
	if !s.color {
		return ""
	}
	if s.unicode {
		return s.Cyan("→")
	}
	return s.Cyan("->")
}

func (s *Styler) IconBullet() string {
	if !s.color {
		return ""
	}
	if s.unicode {
		return s.Dim("•")
	}
	return s.Dim("-")
}

// PrefixWithIcon glues a (possibly empty) icon onto the front of msg. We do
// the gluing here so callers don't have to think about TTY state — when no
// icon is rendered (non-TTY/NO_COLOR) the original message is returned
// unchanged, preserving backwards compatibility with tests that match
// exact strings like "Published site".
func (s *Styler) PrefixWithIcon(icon, msg string) string {
	if icon == "" {
		return msg
	}
	return icon + " " + msg
}

// Sprintf is a convenience for callers that already have a format string.
func (s *Styler) Sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

// stripANSI removes ANSI SGR (Select Graphic Rendition) escape sequences
// from a string while preserving cursor-control codes such as "\x1b[2K"
// that the progress redraw path emits. SGR sequences are CSI sequences
// whose final byte is 'm' and whose parameters are decimal digits or ';'
// — anything else (e.g. 'K' for line-clear) is left untouched. Used by
// tests that need to assert content independent of styling.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if i+1 < len(s) && s[i] == '\x1b' && s[i+1] == '[' {
			j := i + 2
			for j < len(s) {
				c := s[j]
				if (c >= '0' && c <= '9') || c == ';' {
					j++
					continue
				}
				break
			}
			if j < len(s) && s[j] == 'm' {
				i = j + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
