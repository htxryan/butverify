// Package cliref defines shared bv CLI command metadata for help and docs.
package cliref

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const DocsPath = "marketing-site/src/content/docs/docs/reference/cli.md"

type FlagType string

const (
	FlagString FlagType = "string"
	FlagBool   FlagType = "bool"
	FlagInt    FlagType = "int"
	FlagInt64  FlagType = "int64"
)

type Flag struct {
	Name        string
	Value       string
	Description string
	Type        FlagType
	Default     string
	RuntimeHelp string
}

type Command struct {
	Name        string
	Summary     string
	Usage       string
	Description string
	Details     []string
	Flags       []Flag
	Examples    []string
	Hidden      bool
	Deprecated  bool
}

type ExitCode struct {
	Code    string
	Meaning string
}

type Reference struct {
	Commands    []Command
	GlobalFlags []Flag
	ExitCodes   []ExitCode
}

type FlagValues struct {
	strings map[string]*string
	bools   map[string]*bool
	ints    map[string]*int
	int64s  map[string]*int64
}

func (v FlagValues) String(name string) *string { return v.strings[name] }
func (v FlagValues) Bool(name string) *bool     { return v.bools[name] }
func (v FlagValues) Int(name string) *int       { return v.ints[name] }
func (v FlagValues) Int64(name string) *int64   { return v.int64s[name] }

func DefaultReference() Reference {
	return Reference{
		GlobalFlags: []Flag{
			{Name: "--json", Description: "emit a single JSON document on stdout instead of human-readable text", Type: FlagBool},
			{Name: "--api-url", Value: "URL", Description: "override the control-plane endpoint", Type: FlagString},
			{Name: "--token", Value: "TOK", Description: "override the bearer token", Type: FlagString},
			{Name: "--help, -h", Description: "show usage", Type: FlagBool},
			{Name: "--version, -v", Description: "print the CLI version", Type: FlagBool},
		},
		Commands: []Command{
			{
				Name:        "login",
				Summary:     "login",
				Usage:       "bv login [--api-url URL] [--gh-token TOK] [--token TOK]",
				Description: "Resolve and persist a butverify installation token.",
				Details: []string{
					"Default flow: exchange a GitHub user token via POST /v1/auth/login. Token sources are --gh-token, GH_TOKEN, GITHUB_TOKEN, the gh CLI, or a TTY prompt.",
					"Direct flow: pass an installation token via --token, BV_TOKEN, the global --token flag, or piped stdin to skip the exchange.",
				},
				Flags: []Flag{
					{Name: "--api-url", Value: "URL", Description: "control-plane base URL", Type: FlagString, RuntimeHelp: "control-plane base URL (defaults to https://api.butverify.dev)"},
					{Name: "--gh-token", Value: "TOK", Description: "GitHub user token for the exchange flow", Type: FlagString, RuntimeHelp: "GitHub user token (otherwise read from GH_TOKEN/GITHUB_TOKEN, gh CLI, or TTY prompt)"},
					{Name: "--token", Value: "TOK", Description: "butverify installation token for the direct-token flow", Type: FlagString, RuntimeHelp: "butverify installation token (skip GH exchange; otherwise read from BV_TOKEN env or piped stdin)"},
				},
			},
			{Name: "logout", Summary: "logout", Usage: "bv logout", Description: "Clear saved authentication and switch default mode to local."},
			{
				Name:        "mode",
				Summary:     "mode [local|remote]",
				Usage:       "bv mode [local|remote]",
				Description: "Print or set the default publish mode.",
				Details:     []string{"Fresh installs default to local. A successful bv login switches the default to remote."},
			},
			{
				Name:        "push",
				Summary:     "push <dir>",
				Usage:       "bv push [--mode local|remote] [--upload-id ID] [--ttl-seconds N] [--image-quality N] [--include-hidden] [--skip-gitleaks-check] <dir>",
				Description: "Bundle a directory and publish it in local or remote mode.",
				Details: []string{
					"Local mode serves the filtered publish bundle on 127.0.0.1 until interrupted.",
					"Remote mode uploads a tar bundle as a new private site. Pass --upload-id to retry the same logical upload idempotently.",
					"Image optimization recompresses JPEGs with --image-quality and recompresses PNGs losslessly when smaller. Persist a default by setting image_quality in the bv config JSON.",
				},
				Flags: []Flag{
					{Name: "--mode", Value: "local|remote", Description: "override the configured publish mode", Type: FlagString, RuntimeHelp: "publish mode: local or remote (default: configured mode)"},
					{Name: "--upload-id", Value: "ID", Description: "explicit upload_id for idempotent retry", Type: FlagString, RuntimeHelp: "explicit upload_id for idempotent retry (default: auto-generated)"},
					{Name: "--ttl-seconds", Value: "N", Description: "site TTL in seconds; 0 uses the server default", Type: FlagInt64, Default: "0", RuntimeHelp: "site TTL in seconds (paid plan; 0 = use server default)"},
					{Name: "--image-quality", Value: "N", Description: "image optimization quality; JPEG uses this value and PNG is recompressed losslessly when smaller; 0 uses config/default", Type: FlagInt, Default: "0", RuntimeHelp: "image optimization quality (1-100; 0 = config/default)"},
					{Name: "--include-hidden", Description: "include dot-files in the bundle", Type: FlagBool, RuntimeHelp: "include dot-files in the bundle"},
					{Name: "--skip-gitleaks-check", Description: "skip the pre-upload gitleaks secret scan", Type: FlagBool, RuntimeHelp: "skip the pre-upload gitleaks secret scan"},
				},
			},
			{
				Name:        "ls",
				Summary:     "ls [--expired]",
				Usage:       "bv ls [--expired]",
				Description: "List sites for the authenticated tenant.",
				Details: []string{
					"By default, expired sites are hidden. Pass --expired to include them.",
					"The EXPIRES column shows each site's expiry as an absolute timestamp in the user's local timezone, plus a humanized magnitude in parentheses. Pinned sites and sites without an expiry render as an em dash.",
				},
				Flags: []Flag{
					{Name: "--expired", Description: "also include expired sites in the listing", Type: FlagBool, RuntimeHelp: "also include expired sites in the listing"},
				},
			},
			{Name: "rm", Summary: "rm <site-id>", Usage: "bv rm <site-id>", Description: "Soft-delete a site."},
			{Name: "cat", Summary: "cat <site-id> <path>", Usage: "bv cat <site-id> <path>", Description: "Print a single file from a site to stdout."},
			{Name: "get", Summary: "get <site-id> <dest>", Usage: "bv get <site-id> <dest>", Description: "Download a site's files into a destination directory."},
			{Name: "manifest", Summary: "manifest <site-id>", Usage: "bv manifest <site-id>", Description: "Print the site's manifest.json."},
			{Name: "pin", Summary: "pin <site-id>", Usage: "bv pin <site-id>", Description: "Pin a site to disable TTL-based expiry."},
			{Name: "unpin", Summary: "unpin <site-id>", Usage: "bv unpin <site-id>", Description: "Unpin a site and re-stamp the default TTL."},
			{
				Name:        "report",
				Summary:     "report --from <out.json> [--out DIR] [--push]",
				Usage:       "bv report --from <out.json|-> [--out DIR] [--push] [--upload-id ID] [--ttl-seconds N] [--image-quality N] [--mode local|remote]",
				Description: "Render a static report site from JSON.",
				Details:     []string{"Without --push, the rendered site is written to --out or ./bv-report-out. With --push, the rendered directory is published through the standard push flow.", "Image optimization recompresses JPEGs with --image-quality and recompresses PNGs losslessly when smaller. Persist a default by setting image_quality in the bv config JSON."},
				Flags: []Flag{
					{Name: "--from", Value: "PATH|-", Description: "input JSON file or stdin", Type: FlagString, RuntimeHelp: "input JSON file (use - for stdin)"},
					{Name: "--out", Value: "DIR", Description: "output directory", Type: FlagString, RuntimeHelp: "output directory (defaults to ./bv-report when --push not set)"},
					{Name: "--push", Description: "after rendering, push the directory as a new site", Type: FlagBool, RuntimeHelp: "after rendering, push the directory as a new site"},
					{Name: "--upload-id", Value: "ID", Description: "explicit upload_id for idempotent --push retry", Type: FlagString, RuntimeHelp: "explicit upload_id for idempotent --push retry"},
					{Name: "--ttl-seconds", Value: "N", Description: "site TTL in seconds; 0 uses the server default", Type: FlagInt64, Default: "0", RuntimeHelp: "site TTL in seconds (paid plan; 0 = use server default)"},
					{Name: "--image-quality", Value: "N", Description: "image optimization quality; JPEG uses this value and PNG is recompressed losslessly when smaller; 0 uses config/default", Type: FlagInt, Default: "0", RuntimeHelp: "image optimization quality (1-100; 0 = config/default)"},
					{Name: "--mode", Value: "local|remote", Description: "publish mode for --push", Type: FlagString, RuntimeHelp: "publish mode for --push: local or remote (default: configured mode)"},
				},
			},
			{
				Name:        "dashboard",
				Summary:     "dashboard --from <data.csv> [--out DIR] [--push]",
				Usage:       "bv dashboard --from <data.csv|-> [--out DIR] [--title T] [--subtitle T] [--max-table-rows N] [--push] [--upload-id ID] [--ttl-seconds N] [--image-quality N] [--mode local|remote]",
				Description: "Render a static dashboard site from CSV.",
				Details:     []string{"With --push, image optimization recompresses JPEGs with --image-quality and recompresses PNGs losslessly when smaller. Persist a default by setting image_quality in the bv config JSON."},
				Flags: []Flag{
					{Name: "--from", Value: "PATH|-", Description: "input CSV file or stdin", Type: FlagString, RuntimeHelp: "input CSV file (use - for stdin)"},
					{Name: "--out", Value: "DIR", Description: "output directory", Type: FlagString, RuntimeHelp: "output directory (defaults to ./bv-dashboard when --push not set)"},
					{Name: "--title", Value: "T", Description: "page title", Type: FlagString, RuntimeHelp: "page title (defaults to 'Dashboard')"},
					{Name: "--subtitle", Value: "T", Description: "page subtitle", Type: FlagString, RuntimeHelp: "page subtitle"},
					{Name: "--max-table-rows", Value: "N", Description: "cap rows shown in the HTML table; 0 means no cap", Type: FlagInt, Default: "200", RuntimeHelp: "cap rows shown in the HTML table (0 = no cap; data.csv always carries the full set)"},
					{Name: "--push", Description: "after rendering, push the directory as a new site", Type: FlagBool, RuntimeHelp: "after rendering, push the directory as a new site"},
					{Name: "--upload-id", Value: "ID", Description: "explicit upload_id for idempotent --push retry", Type: FlagString, RuntimeHelp: "explicit upload_id for idempotent --push retry"},
					{Name: "--ttl-seconds", Value: "N", Description: "site TTL in seconds; 0 uses the server default", Type: FlagInt64, Default: "0", RuntimeHelp: "site TTL in seconds (paid plan; 0 = use server default)"},
					{Name: "--image-quality", Value: "N", Description: "image optimization quality; JPEG uses this value and PNG is recompressed losslessly when smaller; 0 uses config/default", Type: FlagInt, Default: "0", RuntimeHelp: "image optimization quality (1-100; 0 = config/default)"},
					{Name: "--mode", Value: "local|remote", Description: "publish mode for --push", Type: FlagString, RuntimeHelp: "publish mode for --push: local or remote (default: configured mode)"},
				},
			},
			{
				Name:        "evidence",
				Summary:     "evidence --from <evidence.json> [--out DIR] [--push]",
				Usage:       "bv evidence (--schema | --from <evidence.json|-> [--out DIR] [--push] [--upload-id ID] [--ttl-seconds N] [--image-quality N] [--mode local|remote])",
				Description: "Render a static evidence/gallery site from JSON.",
				Details:     []string{"Use --schema to print the JSON Schema for the input without rendering.", "Manifest metadata may include issue_url, issue_id, and issue_title at the top level for the work-management item the whole gallery proves, or under an individual item when a capture maps to a specific Jira/Linear/GitHub issue.", "Rendered evidence pages include a viewer-side stacked/carousel layout switcher.", "With --push, image optimization recompresses JPEGs with --image-quality and recompresses PNGs losslessly when smaller. Persist a default by setting image_quality in the bv config JSON."},
				Flags: []Flag{
					{Name: "--from", Value: "PATH|-", Description: "input JSON file or stdin", Type: FlagString, RuntimeHelp: "input JSON file (use - for stdin)"},
					{Name: "--out", Value: "DIR", Description: "output directory", Type: FlagString, RuntimeHelp: "output directory (omit when only --push is set)"},
					{Name: "--push", Description: "after rendering, push the directory as a new site", Type: FlagBool, RuntimeHelp: "after rendering, push the directory as a new site"},
					{Name: "--schema", Description: "print the JSON Schema for the evidence input and exit", Type: FlagBool, RuntimeHelp: "print the JSON Schema for the evidence input and exit"},
					{Name: "--upload-id", Value: "ID", Description: "explicit upload_id for idempotent --push retry", Type: FlagString, RuntimeHelp: "explicit upload_id for idempotent --push retry"},
					{Name: "--ttl-seconds", Value: "N", Description: "site TTL in seconds; 0 uses the server default", Type: FlagInt64, Default: "0", RuntimeHelp: "site TTL in seconds (paid plan; 0 = use server default)"},
					{Name: "--image-quality", Value: "N", Description: "image optimization quality; JPEG uses this value and PNG is recompressed losslessly when smaller; 0 uses config/default", Type: FlagInt, Default: "0", RuntimeHelp: "image optimization quality (1-100; 0 = config/default)"},
					{Name: "--mode", Value: "local|remote", Description: "publish mode for --push", Type: FlagString, RuntimeHelp: "publish mode for --push: local or remote (default: configured mode)"},
					{Name: "--enable-reviews", Description: "opt the site into the review/annotation system (paid plan required)", Type: FlagBool, RuntimeHelp: "opt the site into the review/annotation system (paid plan required)"},
				},
			},
			{
				Name:        "agent-init",
				Summary:     "agent-init [--force|--uninstall] [--project] [--enable-hook|--no-hook]",
				Usage:       "bv agent-init [--project] [--force] [--uninstall] [--enable-hook|--no-hook]",
				Description: "Install the /butverify agent skills for the current agent environment.",
				Details:     []string{"v1 installs the Claude Code /butverify:prove-it and /butverify:review skills, plus a deprecated /butverify alias. Future versions may install additional skills, hooks, or MCP servers."},
				Flags: []Flag{
					{Name: "--project", Description: "install into ./.claude/skills/butverify/ instead of $HOME/.claude/skills/butverify/", Type: FlagBool, RuntimeHelp: "install into ./.claude/... instead of $HOME/.claude/..."},
					{Name: "--force", Description: "overwrite an existing install", Type: FlagBool, RuntimeHelp: "overwrite an existing SKILL.md (writes a .bak)"},
					{Name: "--uninstall", Description: "remove the deterministic install file set", Type: FlagBool, RuntimeHelp: "remove an installed SKILL.md and its sibling artifacts"},
					{Name: "--enable-hook", Description: "install Claude Code SessionStart and Stop hooks that surface unacknowledged reviews (skip the TTY prompt)", Type: FlagBool, RuntimeHelp: "install review-notification hooks (no prompt)"},
					{Name: "--no-hook", Description: "skip hook installation regardless of TTY state", Type: FlagBool, RuntimeHelp: "do not install review-notification hooks"},
				},
				Examples: []string{
					"bv agent-init",
					"bv agent-init --project",
				},
			},
			{
				Name:        "install-skill",
				Summary:     "install-skill <agent> [--force|--uninstall] [--project] [--enable-hook|--no-hook]",
				Usage:       "bv install-skill [--project] [--force] [--uninstall] [--enable-hook|--no-hook] <agent>",
				Description: "Install the /butverify agent skills (prove-it + review).",
				Details: []string{
					"Supported agent in v1: claude.",
					"Installs three skill files under <root>/.claude/skills/butverify/: SKILL.md (deprecated alias), prove-it/SKILL.md, review/SKILL.md.",
					"On a TTY, prompts to enable session-start and session-end hooks that surface unacknowledged reviews. --enable-hook forces yes (useful for CI); --no-hook forces no.",
				},
				Flags: []Flag{
					{Name: "--project", Description: "install into ./.claude/skills/<agent>/ instead of $HOME/.claude/skills/<agent>/", Type: FlagBool, RuntimeHelp: "install into ./.claude/... instead of $HOME/.claude/..."},
					{Name: "--force", Description: "overwrite an existing install", Type: FlagBool, RuntimeHelp: "overwrite an existing SKILL.md (writes a .bak)"},
					{Name: "--uninstall", Description: "remove the deterministic install file set", Type: FlagBool, RuntimeHelp: "remove an installed SKILL.md and its sibling artifacts"},
					{Name: "--enable-hook", Description: "install Claude Code SessionStart and Stop hooks that surface unacknowledged reviews (skip the TTY prompt)", Type: FlagBool, RuntimeHelp: "install review-notification hooks (no prompt)"},
					{Name: "--no-hook", Description: "skip hook installation regardless of TTY state", Type: FlagBool, RuntimeHelp: "do not install review-notification hooks"},
				},
				Examples: []string{
					"bv install-skill claude",
					"bv install-skill claude --project",
					"bv install-skill claude --enable-hook",
				},
			},
			{
				Name:        "review",
				Summary:     "review <list|get|acknowledge|request> ...",
				Usage:       "bv review <list|get|acknowledge|request> [args...]",
				Description: "Manage reviews on review-enabled evidence sites.",
				Details: []string{
					"bv review list [--site <id>] [--unacknowledged] [--format json|ids] — list reviews for the calling tenant.",
					"bv review get <review-id> — print a single review with all annotations.",
					"bv review acknowledge <review-id> — mark a review acknowledged (idempotent).",
					"bv review request <site-id> --to <github-login> — invite a GitHub user to review the site.",
				},
				Examples: []string{
					"bv review list --unacknowledged",
					"bv review list --unacknowledged --format=ids",
					"bv review get rev_abc",
					"bv review acknowledge rev_abc",
					"bv review request abcd1234 --to ryanh",
				},
			},
			{Name: "whoami", Summary: "whoami", Usage: "bv whoami", Description: "Print the resolved tenant for the configured token."},
			{Name: "version", Summary: "version", Usage: "bv version", Description: "Print the CLI version."},
			{Name: "init", Summary: "init", Usage: "bv init", Description: "Deprecated alias replaced by bv login.", Hidden: true, Deprecated: true},
		},
		ExitCodes: []ExitCode{
			{Code: "0", Meaning: "success"},
			{Code: "1", Meaning: "API or runtime error"},
			{Code: "2", Meaning: "invalid arguments"},
		},
	}
}

func Lookup(name string) (Command, bool) {
	for _, command := range DefaultReference().Commands {
		if command.Name == name {
			return command, true
		}
	}
	return Command{}, false
}

func UsageError(name string) string {
	command, ok := Lookup(name)
	if !ok {
		return "usage: bv " + name
	}
	return "usage: " + command.Usage
}

func NewFlagSet(name string, output io.Writer) (*flag.FlagSet, FlagValues, bool) {
	command, ok := Lookup(name)
	if !ok {
		return nil, FlagValues{}, false
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(output)
	fs.Usage = func() { _, _ = io.WriteString(output, CommandHelp(name)) }
	values := FlagValues{
		strings: map[string]*string{},
		bools:   map[string]*bool{},
		ints:    map[string]*int{},
		int64s:  map[string]*int64{},
	}
	for _, f := range command.Flags {
		key := flagKey(f.Name)
		help := f.RuntimeHelp
		if help == "" {
			help = f.Description
		}
		switch f.Type {
		case FlagBool:
			values.bools[key] = fs.Bool(key, f.Default == "true", help)
		case FlagInt:
			values.ints[key] = fs.Int(key, mustAtoi(f.Default), help)
		case FlagInt64:
			values.int64s[key] = fs.Int64(key, mustAtoi64(f.Default), help)
		default:
			values.strings[key] = fs.String(key, f.Default, help)
		}
	}
	return fs, values, true
}

const helpBanner = "bv — the butverify.dev agent CLI"

const (
	topLevelTrailer   = "Run 'bv <command> --help' for command-specific help."
	subcommandTrailer = "Run 'bv --help' for the full command list."
)

const commandsLeftWidth = 26

type twoColRow struct {
	left  string
	right string
}

type helpBuilder struct {
	b strings.Builder
}

func (h *helpBuilder) Banner(text string) {
	h.b.WriteString(text)
	h.b.WriteString("\n\n")
}

func (h *helpBuilder) Usage(line string) {
	h.b.WriteString("Usage:\n  ")
	h.b.WriteString(line)
	h.b.WriteString("\n\n")
}

func (h *helpBuilder) Section(title string) {
	h.b.WriteString(title)
	h.b.WriteString(":\n")
}

func (h *helpBuilder) Para(text string) {
	h.b.WriteString(text)
	h.b.WriteString("\n\n")
}

func (h *helpBuilder) TwoCol(rows []twoColRow, leftWidth int) {
	for _, row := range rows {
		if len(row.left) > leftWidth {
			fmt.Fprintf(&h.b, "  %s\n", row.left)
			fmt.Fprintf(&h.b, "  %-*s %s\n", leftWidth, "", row.right)
			continue
		}
		fmt.Fprintf(&h.b, "  %-*s %s\n", leftWidth, row.left, row.right)
	}
	h.b.WriteString("\n")
}

func (h *helpBuilder) Bullets(items []string) {
	for _, item := range items {
		h.b.WriteString("  ")
		h.b.WriteString(item)
		h.b.WriteString("\n")
	}
	h.b.WriteString("\n")
}

func (h *helpBuilder) Trailer(text string) {
	h.b.WriteString(text)
	h.b.WriteString("\n")
}

func (h *helpBuilder) String() string {
	return h.b.String()
}

func UsageText() string {
	ref := DefaultReference()
	var h helpBuilder
	h.Banner(helpBanner)
	h.Usage("bv [--json] [--api-url URL] [--token TOK] <command> [args]")
	h.Section("Commands")
	rows := make([]twoColRow, 0, len(ref.Commands))
	for _, command := range ref.Commands {
		if command.Hidden {
			continue
		}
		rows = append(rows, twoColRow{left: command.Summary, right: command.Description})
	}
	h.TwoCol(rows, commandsLeftWidth)
	h.Trailer(topLevelTrailer)
	return h.String()
}

func CommandHelp(name string) string {
	command, ok := Lookup(name)
	if !ok || command.Hidden {
		return UsageText()
	}
	var h helpBuilder
	h.Banner(helpBanner)
	h.Usage(command.Usage)
	h.Para(command.Description)
	for _, detail := range command.Details {
		h.Para(detail)
	}
	if len(command.Flags) > 0 {
		flagRows := make([]twoColRow, 0, len(command.Flags))
		for _, f := range command.Flags {
			label := f.Name
			if f.Value != "" {
				label += " " + f.Value
			}
			flagRows = append(flagRows, twoColRow{left: label, right: f.Description})
		}
		leftWidth := commandsLeftWidth
		for _, row := range flagRows {
			if len(row.left)+2 > leftWidth {
				leftWidth = len(row.left) + 2
			}
		}
		h.Section("Flags")
		h.TwoCol(flagRows, leftWidth)
	}
	if len(command.Examples) > 0 {
		h.Section("Examples")
		h.Bullets(command.Examples)
	}
	h.Trailer(subcommandTrailer)
	return h.String()
}

func Markdown() string {
	ref := DefaultReference()
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: CLI commands\n")
	b.WriteString("description: Every bv subcommand, its flags, and what it returns.\n")
	b.WriteString("---\n\n")
	b.WriteString("<!-- Code generated by task docs:generate; DO NOT EDIT. -->\n\n")
	b.WriteString("> This file is generated from the shared `bv` command metadata used by CLI handlers and help. Run `task docs:generate` after changing the CLI surface.\n\n")
	b.WriteString("The `bv` CLI is a single Go binary. It supports the global flags below and the subcommands that follow.\n\n")
	b.WriteString("## Global flags\n\n")
	writeFlagList(&b, ref.GlobalFlags)
	for _, command := range ref.Commands {
		if command.Hidden {
			continue
		}
		fmt.Fprintf(&b, "\n## `%s`\n\n", command.Usage)
		b.WriteString(command.Description)
		b.WriteString("\n")
		for _, detail := range command.Details {
			b.WriteString("\n")
			b.WriteString(detail)
			b.WriteString("\n")
		}
		if len(command.Flags) > 0 {
			b.WriteString("\nFlags:\n\n")
			writeFlagList(&b, command.Flags)
		}
		if len(command.Examples) > 0 {
			b.WriteString("\nExamples:\n\n")
			for _, example := range command.Examples {
				fmt.Fprintf(&b, "- `%s`\n", example)
			}
		}
	}
	b.WriteString("\n## Exit codes\n\n")
	b.WriteString("| code | meaning |\n")
	b.WriteString("| ---- | ------- |\n")
	for _, exitCode := range ref.ExitCodes {
		fmt.Fprintf(&b, "| %s | %s |\n", exitCode.Code, exitCode.Meaning)
	}
	return b.String()
}

func writeFlagList(b *strings.Builder, flags []Flag) {
	for _, flag := range flags {
		name := flag.Name
		if flag.Value != "" {
			name += " " + flag.Value
		}
		fmt.Fprintf(b, "- `%s` — %s.\n", name, flag.Description)
	}
}

func flagKey(name string) string {
	return strings.TrimLeft(strings.Split(name, ",")[0], "-")
}

func mustAtoi(raw string) int {
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		panic(err)
	}
	return value
}

func mustAtoi64(raw string) int64 {
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		panic(err)
	}
	return value
}
