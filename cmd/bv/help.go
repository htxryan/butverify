package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/htxryan/butverify/internal/cliref"
	"github.com/htxryan/butverify/internal/output"
)

func newCLIFlagSet(name string) (*flag.FlagSet, cliref.FlagValues) {
	fs, values, ok := cliref.NewFlagSet(name, io.Discard)
	if !ok {
		panic("unknown CLI command metadata: " + name)
	}
	return fs, values
}

func handleFlagParseError(g globalContext, name string, err error) int {
	if errors.Is(err, flag.ErrHelp) {
		printCommandHelp(name)
		return 0
	}
	g.w.Error(toErrorEnvelope(err))
	return 2
}

func printCommandHelp(name string) {
	fmt.Print(styleHelpText(cliref.CommandHelp(name), helpStdoutStyler()))
}

func usageError(name string) error {
	return errors.New(cliref.UsageError(name))
}

// helpStdoutStyler returns a Styler that applies styling only when stdout
// is an interactive terminal. Help is printed via fmt.Print so the writer
// is not in scope; we re-derive a Styler here based on os.Stdout.
func helpStdoutStyler() *output.Styler {
	return helpStylerFor(os.Stdout)
}

// helpStderrStyler is the stderr counterpart used when the CLI prints
// usage to stderr (e.g. on missing command, after an unknown command).
func helpStderrStyler() *output.Styler {
	return helpStylerFor(os.Stderr)
}

func helpStylerFor(f *os.File) *output.Styler {
	tty := false
	if info, err := f.Stat(); err == nil {
		tty = info.Mode()&os.ModeCharDevice != 0
	}
	if tty {
		return output.NewStyled(output.ModeHuman, f, f).StdoutStyler()
	}
	return output.NewWith(output.ModeHuman, f, f).StdoutStyler()
}

// styleHelpText post-processes the help text returned by cliref so the
// section headings ("Usage:", "Commands:", "Flags:", "Examples:") read
// as bold cyan and inline `code spans` read as cyan. The literal text is
// preserved 1:1 in non-TTY mode (unit tests + scrapers untouched). When a
// styler returns its argument unchanged (color disabled) the function
// reduces to identity over the input.
func styleHelpText(s string, st *output.Styler) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		switch trimmed {
		case "Usage:", "Commands:", "Flags:", "Examples:":
			lines[i] = st.Heading(trimmed)
			continue
		}
		if strings.HasPrefix(line, "Usage: ") {
			lines[i] = st.Heading("Usage: ") + line[len("Usage: "):]
			continue
		}
		// Inline backtick code spans: turn `bv push` into a cyan-colored
		// chunk so flag/example tables read like Markdown.
		if strings.Contains(line, "`") {
			lines[i] = styleBacktickCode(line, st)
		}
	}
	return strings.Join(lines, "\n")
}

// styleBacktickCode wraps `…` spans with the styler's Cyan color while
// keeping the surrounding backticks intact (so non-TTY output is byte-
// identical and the test that asserts e.g. "## `bv push …`" still
// matches the markdown generator output).
func styleBacktickCode(line string, st *output.Styler) string {
	var b strings.Builder
	b.Grow(len(line))
	for {
		i := strings.IndexByte(line, '`')
		if i < 0 {
			b.WriteString(line)
			return b.String()
		}
		j := strings.IndexByte(line[i+1:], '`')
		if j < 0 {
			b.WriteString(line)
			return b.String()
		}
		j += i + 1
		b.WriteString(line[:i+1])
		b.WriteString(st.Cyan(line[i+1 : j]))
		b.WriteString(line[j : j+1])
		line = line[j+1:]
	}
}
