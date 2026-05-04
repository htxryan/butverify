# Literal proof: `bv agent-init`

Remote proof page: https://w48z1wjs.butverify.dev

This proof was captured from the PR branch by running the user-facing CLI in an
isolated temporary `HOME`. It demonstrates the task requirements directly:

- `01-first-agent-init.json` shows `bv --json agent-init` installed
  `$HOME/.claude/skills/butverify/SKILL.md` with `status: "installed"`.
- `installed-SKILL.md` is the actual installed skill file copied from that
  temporary `HOME`; its final line is the metadata comment.
- `03-hash-check.txt` recomputes the SHA-256 prefix from the installed file body
  excluding the metadata line and shows it matches the metadata hash.
- `02-repeat-agent-init.json` shows a second run returns
  `status: "already_current"`.
- `04-drift-agent-init.*` shows a deliberate local edit before the metadata line
  makes the next run exit non-zero with a drift error containing
  `current_content=...`.
- `site/index.html` is the exact static page published to the remote proof URL.
- `remote-manifest.json` is the manifest returned by ButVerify for the published
  site `w48z1wjs`.

The proof site was published with:

```bash
go run ./cmd/bv --json push --mode remote docs/proof/6gWHR3WWjC7WRG64/literal-agent-init-proof/site
```

The upload manifest was verified with:

```bash
go run ./cmd/bv manifest w48z1wjs
```
