# Agent Instructions

## Project Context

This is the **public** butverify repo. It contains:
- `bv` CLI — the Go tool agents use to publish proof of finished work to butverify.dev
- `marketing-site/` — Astro + Starlight docs/marketing site (butverify.dev)
- `bv-skills/` — canonical `/butverify` agent skill source
- `pkg/` — reusable Go packages (identity, sitelifecycle)
- `internal/` — CLI-only Go code

The **private** service monorepo (control-plane Worker, dashboard, TS kernels, ADRs) lives at
`htxryan/butverify-service`. Cross-repo flows are documented in `CONTRIBUTING.md`.

## Repo Layout

```text
cmd/bv/                  # bv CLI entrypoint (Go)
pkg/                     # Reusable Go packages
internal/                # CLI-only Go code
marketing-site/          # Astro + Starlight docs/marketing (pnpm)
bv-skills/               # /butverify agent skill templates
.github/workflows/       # CI, release, Pages-deploy pipelines
```

## Core Commands

```bash
# Go
go build ./cmd/bv         # Build bv binary
go test -count=1 ./...    # Run all Go tests
go vet ./...              # Vet
gofmt -l .                # Format check (empty output = clean)

# Marketing site
cd marketing-site
pnpm install
pnpm dev                  # http://localhost:4321
pnpm typecheck            # astro check
pnpm lint
pnpm test                 # vitest
pnpm build
pnpm link-check
```

## Conventions

- Pushes to `main` require a PR; squash-merge only. All CI checks must pass.
- Go: follow `gofmt`, `go vet`, `golangci-lint` (see `.golangci.yml`).
- Marketing-site: TypeScript strict mode; ESLint flat config; vitest for tests.
- Never push directly to `main` — always go through a PR.
- Releases are tag-driven (`git tag -a vX.Y.Z`); see CONTRIBUTING.md for the full flow.

## Non-Interactive Shell Commands

Always use non-interactive flags to avoid hanging on confirmation prompts:

```bash
cp -f source dest
mv -f source dest
rm -f file
rm -rf directory
```

## Compound Agent Integration

This project uses compound-agent for session memory. Use `./node_modules/.bin/ca` (installed
locally) or `ca` if available on PATH.

| Command | Purpose |
| --- | --- |
| `./node_modules/.bin/ca search "query"` | Search lessons; run before architectural decisions or when uncertain about prior patterns |
| `./node_modules/.bin/ca knowledge "keyword phrase"` | Semantic search over indexed project docs |
| `./node_modules/.bin/ca learn "insight"` | Capture lessons after corrections or discoveries |
| `./node_modules/.bin/ca list` | List stored lessons |
| `./node_modules/.bin/ca show <id>` | Show a lesson |
| `./node_modules/.bin/ca wrong <id>` | Mark a lesson incorrect |

Run `ca search` and `ca knowledge` before architectural decisions, complex planning, or when
uncertain about repo patterns. Capture only novel, specific, actionable lessons. Never edit
`.claude/lessons/index.jsonl` directly; use the CLI.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:ca08a54f -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

## Session Completion

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd dolt push
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
<!-- END BEADS INTEGRATION -->
<!-- compound-agent:start -->
## Compound Agent Integration

This project uses compound-agent for session memory via **CLI commands**.

### CLI Commands (ALWAYS USE THESE)

**You MUST use CLI commands for lesson management:**

| Command | Purpose |
|---------|---------|
| `ca search "query"` | Search lessons - MUST call before architectural decisions; use anytime you need context |
| `ca knowledge "query"` | Semantic search over project docs - MUST call before architectural decisions; use keyword phrases, not questions |
| `ca learn "insight"` | Capture lessons - use AFTER corrections or discoveries |
| `ca list` | List all stored lessons |
| `ca show <id>` | Show details of a specific lesson |
| `ca wrong <id>` | Mark a lesson as incorrect |

### Mandatory Recall

You MUST call `ca search` and `ca knowledge` BEFORE:
- Architectural decisions or complex planning
- Patterns you've implemented before in this repo
- After user corrections ("actually...", "wrong", "use X instead")

**NEVER skip search for complex decisions.** Past mistakes will repeat.

Beyond mandatory triggers, use these commands freely — they are lightweight queries, not heavyweight operations. Uncertain about a pattern? `ca search`. Need a detail from the docs? `ca knowledge`. The cost of an unnecessary search is near-zero; the cost of a missed one can be hours.

### Capture Protocol

Run `ca learn` AFTER:
- User corrects you
- Test fail -> fix -> pass cycles
- You discover project-specific knowledge

**Workflow**: Search BEFORE deciding, capture AFTER learning.

### Quality Gate

Before capturing, verify the lesson is:
- **Novel** - Not already stored
- **Specific** - Clear guidance
- **Actionable** (preferred) - Obvious what to do

### Never Edit JSONL Directly

**WARNING: NEVER edit .claude/lessons/index.jsonl directly.**

The JSONL file requires proper ID generation, schema validation, and SQLite sync.
Use CLI (`ca learn`) — never manual edits.

See [documentation](https://github.com/Nathandela/compound-agent) for more details.
<!-- compound-agent:end -->

## Basic Memory

This repository participates in the shared Basic Memory project **`butverify`**, hosted at `../butverify-service/docs/`. Use the `basic-memory` MCP server (configured at the user level for Claude Code and Codex CLI) — both this repo and `butverify-service` operate against the same knowledge base.
