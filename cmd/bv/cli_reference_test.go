package main

import (
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/htxryan/butverify/internal/cliref"
)

func TestCommandHelpExitsZero(t *testing.T) {
	for _, command := range []string{"push", "report", "dashboard", "evidence", "login", "agent-init", "install-skill", "mode"} {
		command := command
		t.Run(command, func(t *testing.T) {
			cmd := exec.Command("go", "run", ".", command, "--help")
			cmd.Env = append(os.Environ(), "CI=true", "GIT_TERMINAL_PROMPT=0")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("bv %s --help failed: %v\n%s", command, err, out)
			}
			text := string(out)
			want := "Usage:\n  bv " + command
			if !strings.Contains(text, want) {
				t.Fatalf("help for %s missing %q:\n%s", command, want, text)
			}
		})
	}
}

func TestDispatchCasesAreDocumentedOrHidden(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(`case "([^"]+)":`).FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatal("no dispatch cases found in main.go")
	}
	for _, match := range matches {
		name := match[1]
		command, ok := cliref.Lookup(name)
		if !ok {
			t.Fatalf("dispatch case %q is neither documented nor intentionally hidden", name)
		}
		if command.Hidden && name != "init" {
			t.Fatalf("dispatch case %q is hidden without an explicit policy test", name)
		}
	}
	initCommand, ok := cliref.Lookup("init")
	if !ok || !initCommand.Hidden || !initCommand.Deprecated {
		t.Fatalf("init must be represented as hidden and deprecated; got %#v, %v", initCommand, ok)
	}
}

func TestVisibleCommandsAppearInTopLevelHelp(t *testing.T) {
	for _, command := range cliref.DefaultReference().Commands {
		if command.Hidden {
			if regexp.MustCompile(`(?m)^\s+`+regexp.QuoteMeta(command.Name)+`\b`).FindString(usageText) != "" {
				t.Fatalf("hidden command %q appeared in top-level usage", command.Name)
			}
			continue
		}
		if !strings.Contains(usageText, command.Name) {
			t.Fatalf("visible command %q missing from top-level usage", command.Name)
		}
	}
}

func TestUsageErrorUsesCommandMetadata(t *testing.T) {
	for _, name := range []string{"push", "report", "dashboard", "evidence", "cat", "get", "manifest", "mode"} {
		command, ok := cliref.Lookup(name)
		if !ok {
			t.Fatalf("missing metadata for %s", name)
		}
		if got, want := usageError(name).Error(), "usage: "+command.Usage; got != want {
			t.Fatalf("usageError(%q)=%q, want %q", name, got, want)
		}
	}
}

func TestCommandMetadataFlagSetsRegisterDocumentedFlags(t *testing.T) {
	for _, command := range cliref.DefaultReference().Commands {
		if command.Hidden || len(command.Flags) == 0 {
			continue
		}
		fs, _, ok := cliref.NewFlagSet(command.Name, io.Discard)
		if !ok {
			t.Fatalf("missing flag set for %s", command.Name)
		}
		for _, documented := range command.Flags {
			flagName := strings.TrimLeft(strings.Split(documented.Name, ",")[0], "-")
			if fs.Lookup(flagName) == nil {
				t.Fatalf("%s documents %s but runtime flag set did not register it", command.Name, documented.Name)
			}
		}
	}
}
