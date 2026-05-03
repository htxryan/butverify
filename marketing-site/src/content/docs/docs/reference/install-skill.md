---
title: bv install-skill claude
description: Install the /butverify agent skill into Claude Code so any session can publish proof of finished work.
---

The `/butverify` skill is a pre-baked agent prompt that closes the loop on
"agent finished work → human can verify it." When the agent invokes
`/butverify` after delivering a piece of functional work, it exercises the
app end-to-end, captures screenshot/video proof, and publishes it as a
private gallery on butverify.dev that you can open and review without
scrolling through chat. Install it once with:

```bash
bv install-skill claude
```

## What gets installed

By default, `bv install-skill claude` writes a single file to your home
directory:

```
~/.claude/skills/butverify/SKILL.md
```

That's everything. The skill content is embedded in the `bv` binary, so
the install is offline and does not require login.

### `--project` (per-repo install)

Pass `--project` to install into the current working directory instead of
`$HOME`:

```bash
bv install-skill claude --project
```

This writes `./.claude/skills/butverify/SKILL.md`, which you can check in
alongside the project so every contributor (and every agent running in the
repo) gets the same skill.

## Prerequisites

The CLI must be installed. Login is optional for installation, but the
installed skill publishes remote proof with `--mode remote`; run `bv login`
before using `/butverify` if you want the skill to return a private
butverify.dev URL.

## Re-installing (drift detection)

The installed `SKILL.md` carries a `bv-skill-version` hash in its
frontmatter. When you run `bv install-skill claude` again, the CLI
compares the installed hash against the embedded one:

- **Match** → exits `0` with `already up to date`. The file's mtime is
  not touched.
- **Differ** (typical after upgrading the `bv` binary) → exits non-zero
  with both versions named, and points you at `--force` or `--uninstall`.

The CLI **never** silently overwrites an existing file.

## `--force`

Overwrite the installed `SKILL.md` with the embedded version:

```bash
bv install-skill claude --force
```

Before overwriting, the CLI copies the existing file to
`SKILL.md.bak` next to it. Only one level of backup is kept — running
`--force` again **always overwrites the prior `.bak`**. Most-recent
semantics: the version preserved is the one you most recently asked to
replace, since that's what you're most likely to want to recover.

## `--uninstall`

Remove the skill files:

```bash
bv install-skill claude --uninstall
```

This deletes the deterministic per-agent file set:

```
<install-root>/.claude/skills/butverify/SKILL.md
<install-root>/.claude/skills/butverify/SKILL.md.bak
<install-root>/.claude/skills/butverify/SKILL.md.tmp.*
```

…where `<install-root>` is `$HOME` by default, or the current working
directory if you pass `--project`.

After deletion, `bv` runs `rmdir` on the agent directory **only if it is
empty**. The CLI never touches any ancestor (e.g. `~/.claude/skills/`) and
never uses `RemoveAll`.

## Supported agents

| Agent      | v1?       | Install path (default)                | `--project` path                      |
| ---------- | --------- | ------------------------------------- | ------------------------------------- |
| `claude`   | yes       | `~/.claude/skills/butverify/SKILL.md` | `./.claude/skills/butverify/SKILL.md` |
| `codex`    | follow-on | —                                     | —                                     |
| `copilot`  | follow-on | —                                     | —                                     |
| `opencode` | follow-on | —                                     | —                                     |

Passing an unsupported agent name exits non-zero and lists the supported
values. Adapters for Codex CLI, GitHub Copilot, and OpenCode land as
follow-on epics once the v1 abstraction is validated against Claude Code.

## End-to-end flow

The flow looks like this once the skill is installed:

1. Run `bv install-skill claude` once on your machine. Run `bv login` too if
   you want the skill to publish private remote galleries.
2. In any project, start a Claude Code session and ask it to do real
   work ("build feature X").
3. After the agent says it's done, type `/butverify`.
4. The skill drives the agent through the workflow: exercise the app
   end-to-end, capture proof, write `evidence.json`, run
   `bv evidence --from evidence.json --push --mode remote`, and surface the returned
   URL to you.
5. Open the URL — you see exactly what the agent built, signed in with
   GitHub.

The skill is opinionated: it refuses to publish proof of unfinished work,
never fakes UI state, and treats "delivered" as "URL surfaced to the
human." That's the contract — see the embedded `SKILL.md` after install
for the full rules.

## See also

- [`bv` CLI reference](/docs/reference/cli) — every subcommand, including
  `bv evidence`.
- [Claude Code recipe](/docs/agents/claude-code) — wire `bv push` into a
  Claude Code session via hooks.
- [Error codes](/docs/reference/error-codes) — what each non-zero exit
  from `bv` means.
