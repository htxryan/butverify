// Package main is the bv CLI entrypoint — the agent surface for butverify.dev.
//
// Commands at MVP (E7):
//
//	bv login     — resolve and persist a butverify installation token
//	               (exchange a GitHub user token via /v1/auth/login,
//	               or save a pre-minted installation token directly
//	               via --token / BV_TOKEN / piped stdin)
//	bv push      — bundle a directory and upload it as a new site
//	bv ls        — list sites for the authenticated tenant
//	bv rm        — soft-delete a site (DELETE /v1/sites/{id})
//	bv cat       — print a single file from a site to stdout
//	bv get       — download all files in a site to a destination directory
//	bv manifest  — print the site's manifest.json
//	bv pin       — pin a site (paid plan; disables TTL)
//	bv unpin     — unpin a site (re-stamps the default TTL)
//
// Templated artifacts (E8 / E12):
//
//	bv report    — render JSON → static report site (optionally --push)
//	bv dashboard — render CSV → static dashboard site (optionally --push)
//	bv evidence  — render JSON → static evidence gallery (optionally --push;
//	               --schema prints the JSON Schema for the input)
//
// Agent skills (E13):
//
//	bv install-skill <agent> — install the /butverify agent skill into the
//	                           per-agent canonical location (v1: claude only)
//
// Global flags:
//
//	--json       — emit structured output instead of human-readable text
//	--api-url    — override the control-plane endpoint (defaults to config)
//	--token      — override the bearer token (defaults to config)
//
// The CLI uses stdlib `flag` rather than cobra/charmbracelet/fang because
// the command surface is small and the binary-size budget (<15MB stripped)
// is tight at MVP. If the surface grows, swap in cobra here without
// breaking the per-subcommand handlers.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/htxryan/butverify/internal/output"
)

// Version is the build-time version. ldflags-injected at release time;
// "dev" in development.
var Version = "dev"

const usageText = `bv — the butverify.dev agent CLI

Usage:
  bv [--json] [--api-url URL] [--token TOK] <command> [args]

Commands:
  login             Resolve and persist a butverify installation token.
                    Default flow: exchange a GitHub user token via
                    POST /v1/auth/login (uses --gh-token, GH_TOKEN,
                    GITHUB_TOKEN, the gh CLI, or a TTY prompt).
                    Direct flow: pass an installation token via
                    --token, BV_TOKEN env, or piped stdin to skip
                    the exchange (CI / scripted setups).
  push <dir>        Bundle <dir> and upload it as a new site.
  ls                List sites for the authenticated tenant.
  rm <site-id>      Soft-delete a site.
  cat <site-id> <path>
                    Print a single file from a site to stdout.
  get <site-id> <dest>
                    Download a site's files into <dest>.
  manifest <site-id>
                    Print the site's manifest.json.
  pin <site-id>     Pin a site (paid plan; disables TTL).
  unpin <site-id>   Unpin a site (re-stamps the default TTL).
  report --from <out.json> [--out DIR] [--push]
                    Render a static report site from JSON.
  dashboard --from <data.csv> [--out DIR] [--push]
                    Render a static dashboard site from CSV.
  evidence --from <evidence.json> [--out DIR] [--push] [--layout stacked|carousel]
                    Render a static evidence/gallery site from JSON.
                    Use --schema to print the JSON Schema for the input.
  install-skill <agent> [--force|--uninstall] [--project]
                    Install the /butverify agent skill (v1: claude only).
  whoami            Print the resolved tenant for the configured token.
  version           Print the CLI version.

Run 'bv <command> --help' for command-specific help.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}
	// Parse leading global flags. We hand-roll instead of using flag.Parse
	// at top-level because Go's stdlib flag package doesn't let
	// subcommands intermix flags with the parent parser cleanly.
	args := os.Args[1:]
	jsonMode := false
	apiURLOverride := ""
	tokenOverride := ""
	cmd := ""
	cmdArgs := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			jsonMode = true
		case a == "--api-url":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "bv: --api-url requires a value")
				os.Exit(2)
			}
			i++
			apiURLOverride = args[i]
		case strings.HasPrefix(a, "--api-url="):
			apiURLOverride = strings.TrimPrefix(a, "--api-url=")
		case a == "--token":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "bv: --token requires a value")
				os.Exit(2)
			}
			i++
			tokenOverride = args[i]
		case strings.HasPrefix(a, "--token="):
			tokenOverride = strings.TrimPrefix(a, "--token=")
		case a == "--help" || a == "-h":
			fmt.Print(usageText)
			os.Exit(0)
		case a == "--version" || a == "-v":
			cmd = "version"
		default:
			cmd = a
			cmdArgs = args[i+1:]
			i = len(args)
		}
	}
	if cmd == "" {
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}

	mode := output.ModeHuman
	if jsonMode {
		mode = output.ModeJSON
	}
	w := output.New(mode)

	// Cancel running operations on SIGINT/SIGTERM so a long-running push
	// doesn't ignore Ctrl-C. The subcommand handler that holds long-lived
	// resources (e.g. an open HTTP body during `bv get`) listens on the
	// returned context.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	gctx := globalContext{
		w:              w,
		apiURLOverride: apiURLOverride,
		tokenOverride:  tokenOverride,
	}

	var exitCode int
	switch cmd {
	case "version":
		exitCode = runVersion(w)
	case "init":
		// `bv init` was folded into `bv login` (which now accepts
		// --token / BV_TOKEN / piped stdin to skip the GitHub
		// exchange). Keep the dispatch here for one release with a
		// clear migration message instead of a generic "unknown
		// command" so existing users — and any CI scripts still
		// running `gh auth token | bv init` — get a precise pointer.
		fmt.Fprintln(os.Stderr, "bv: 'bv init' has been replaced by 'bv login'.")
		fmt.Fprintln(os.Stderr, "    Equivalent invocations:")
		fmt.Fprintln(os.Stderr, "      bv init --token <tok>     →  bv login --token <tok>")
		fmt.Fprintln(os.Stderr, "      cmd | bv init             →  cmd | bv login")
		fmt.Fprintln(os.Stderr, "      BV_TOKEN=<tok> bv init    →  BV_TOKEN=<tok> bv login")
		fmt.Fprintln(os.Stderr, "    Or to mint a fresh token from GitHub: just run 'bv login'.")
		exitCode = 2
	case "login":
		exitCode = runLogin(ctx, gctx, cmdArgs)
	case "whoami":
		exitCode = runWhoami(ctx, gctx, cmdArgs)
	case "push":
		exitCode = runPush(ctx, gctx, cmdArgs)
	case "ls":
		exitCode = runList(ctx, gctx, cmdArgs)
	case "rm":
		exitCode = runRemove(ctx, gctx, cmdArgs)
	case "cat":
		exitCode = runCat(ctx, gctx, cmdArgs)
	case "get":
		exitCode = runGet(ctx, gctx, cmdArgs)
	case "manifest":
		exitCode = runManifest(ctx, gctx, cmdArgs)
	case "pin":
		exitCode = runPin(ctx, gctx, cmdArgs)
	case "unpin":
		exitCode = runUnpin(ctx, gctx, cmdArgs)
	case "report":
		exitCode = runReport(ctx, gctx, cmdArgs)
	case "dashboard":
		exitCode = runDashboard(ctx, gctx, cmdArgs)
	case "evidence":
		exitCode = runEvidence(ctx, gctx, cmdArgs)
	case "install-skill":
		exitCode = runInstallSkill(ctx, gctx, cmdArgs)
	default:
		fmt.Fprintf(os.Stderr, "bv: unknown command %q\n\n", cmd)
		fmt.Fprint(os.Stderr, usageText)
		exitCode = 2
	}
	os.Exit(exitCode)
}

// globalContext bundles process-wide state passed to every subcommand. The
// alternative — package globals — would make per-test isolation harder.
type globalContext struct {
	w              *output.Writer
	apiURLOverride string
	tokenOverride  string
}

func runVersion(w *output.Writer) int {
	if w.IsJSON() {
		_ = w.JSON(struct {
			Version string `json:"version"`
		}{Version})
		return 0
	}
	w.Human("bv %s", Version)
	return 0
}
