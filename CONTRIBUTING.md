# Contributing

Developer-facing notes for hacking on this repo. See [`README.md`](README.md) for end-user install/quick-start.

## Repo structure

This is the **public** repo. It holds:

| Path | What it is |
| --- | --- |
| `cmd/bv/` | `bv` CLI entrypoint (Go) |
| `pkg/` | Reusable Go packages (identity, sitelifecycle twins of the TS kernels in the service repo) |
| `internal/` | CLI-only Go code |
| `marketing-site/` | Astro + Starlight site for `butverify.dev` and `butverify.dev/docs/*` |
| `bv-skills/` | Canonical source for the `/butverify` agent skill (installed via `bv install-skill <agent>`) |
| `.github/workflows/` | CI, release, and Pages-deploy pipelines |

The **control plane, customer-site Worker, dashboard, integration-verification harness, shared TS kernels, ADRs, and architectural specs** live in the **private** sibling repo [`htxryan/butverify-service`](https://github.com/htxryan/butverify-service). The split landed on 2026-05-01.

## Cross-repo flows

| Flow | Where it starts | How it works |
| --- | --- | --- |
| `bv` release | tag `v*` on this repo | `.github/workflows/release.yaml` runs GoReleaser → cross-platform binaries → cosign keyless OIDC signing → GitHub Release → Homebrew formula PR'd to `htxryan/homebrew-butverify` |
| Marketing-site deploy | push to `main` touching `marketing-site/**` | `.github/workflows/marketing-deploy.yaml` builds the Astro site and runs `wrangler pages deploy dist --project-name=butverify-marketing` (CF Pages project; serves `butverify.dev` + `docs.butverify.dev`) |
| Control-plane / dashboard / customer-site deploy | manual `gh workflow run Deploy` on `htxryan/butverify-service` | Auto to dev on CI green there; qa and prod manual `workflow_dispatch` |
| `/butverify` skill update | edits in `bv-skills/` here | Users get the new skill content the next time they run `bv install-skill <agent>` |

The cross-repo deploy-order runbook (which side ships first when a CLI feature depends on a server-side change) is at [`htxryan/butverify-service/docs/operations/cross-repo-release-runbook.md`](https://github.com/htxryan/butverify-service/blob/main/docs/operations/cross-repo-release-runbook.md). Read it before tagging a CLI release that touches a new endpoint.

## CI

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) runs three jobs on every push to `main` and every PR:

- **Go (ubuntu-latest)** — generated CLI docs freshness (`task docs:check`) → `gofmt` format check → `go vet` → `golangci-lint` → `go test -count=1 ./...` → `go build ./cmd/bv`.
- **Marketing site (ubuntu-latest)** — `pnpm install --frozen-lockfile` → `pnpm typecheck` (`astro check`) → `pnpm test` (vitest, ~145 tests) → `pnpm build` (Astro) → `pnpm link-check` (no broken internal links in `dist/`).
- **Secret scan** — `gitleaks` across full git history.
- **CodeQL** — static security + quality analysis on Go and JS/TS using the `security-extended` query suite. Runs on every push/PR + weekly schedule. Severity ≥ high blocks merge via the ruleset's `code_scanning` rule.

All four are required by the `main` branch ruleset before merge.

## Releasing the CLI

Tag-driven. Tag bodies are free-form; GoReleaser generates a changelog from commit history.

```bash
git tag -a v0.1.3 -m "..."
git push origin v0.1.3
gh run watch  # follow the Release workflow
```

Watch for:

- GoReleaser produces `bv_<ver>_<os>_<arch>.{tar.gz,zip}`, `checksums.txt`, and cosign `.sig` + `.pem` per artifact.
- A formula commit lands on [`htxryan/homebrew-butverify`](https://github.com/htxryan/homebrew-butverify) bumping `bv.rb` to the new version with new sha256s.
- `brew upgrade htxryan/butverify/bv` resolves to the new version within a few minutes (no manual tap-bump needed).

If a release fails partway, retry by deleting the tag locally and remotely and re-tagging — GoReleaser is idempotent on the artifact name set.

## Verifying release signatures

See [`README.md` → Verifying release signatures](README.md#verifying-release-signatures-cosign) for the user-facing recipe. The cosign certificate identity for any artifact published from this repo will match `^https://github.com/htxryan/butverify/.github/workflows/release.yaml@`.

## Marketing-site local dev

```bash
cd marketing-site
pnpm install
pnpm dev          # http://localhost:4321
pnpm build        # produces dist/
pnpm preview      # serve dist/ locally
pnpm test         # vitest
pnpm link-check   # static internal-link check against dist/
```

The deployed CF Pages project is `butverify-marketing`. Project name and build command are pinned in [`.github/workflows/marketing-deploy.yaml`](.github/workflows/marketing-deploy.yaml); custom domains (apex `butverify.dev` and `docs.butverify.dev`) are bound to the project in the Cloudflare Dashboard.

## Generated CLI docs

The CLI reference at `marketing-site/src/content/docs/docs/reference/cli.md` is generated from the shared `bv` command metadata used by CLI handlers and help.

```bash
task docs:generate  # regenerate cli.md
task docs:check     # fail if generation changes cli.md
task hooks:install  # configure .githooks/pre-commit for local freshness checks
```

## Branch policy

`main` is protected by a GitHub branch ruleset:

- Pushes to `main` must go through a pull request (squash-merge only).
- All three CI checks (`Go (ubuntu-latest)`, `Marketing site (ubuntu-latest)`, `Secret scan`) must pass before merge.
- Force-pushes and branch deletions are blocked.

Repo admins can `--admin`-override in emergencies via `gh pr merge --admin`, but the expectation is to wait for green checks.
