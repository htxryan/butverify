# Proof: bv push auth token refresh

Todoist task: `6gWRjmJRVqPHV2cf`
Beads issue: `butverify-0hl`
PR: https://github.com/htxryan/butverify/pull/9
Branch: `feat/bv-push-token-refresh`

## Primary Proof: Actual Running CLI

This proof exercises the actual built `bv` CLI binary against live local HTTP servers that implement the control-plane and staging endpoints used by `bv push`. The harness builds `bv`, starts a live server per scenario, seeds real `BV_CONFIG_PATH` config files, invokes `bv push`, captures stdout/stderr, records every HTTP request the server observed, and verifies the config after the run.

Run command:

```bash
python3 docs/proof/butverify-0hl/run_actual_app_proof.py
```

The generated actual-app evidence lives in [`actual-app-run/`](./actual-app-run/):

- [`full-transcript.png`](./actual-app-run/full-transcript.png): complete screenshot of all actual `bv` runs.
- [`proactive-expired-token-refresh.png`](./actual-app-run/proactive-expired-token-refresh.png): expired saved token refreshes before `POST /v1/sites`.
- [`create-401-refresh-retry.png`](./actual-app-run/create-401-refresh-retry.png): create returns 401, `bv` refreshes, then retries create with the new token.
- [`finalize-401-refresh-retry.png`](./actual-app-run/finalize-401-refresh-retry.png): finalize returns 401, `bv` refreshes, then retries finalize with the new token.
- [`token-override-no-refresh.png`](./actual-app-run/token-override-no-refresh.png): explicit `--token` override returns 401 and does not call `/v1/auth/login`.
- [`transcript.txt`](./actual-app-run/transcript.txt): raw copyable transcript backing the screenshots.

Verified observable outcomes:

- expired saved installation token triggers `/v1/auth/login` before `POST /v1/sites`
- refreshed installation token is used for create/upload/finalize
- create 401 triggers one refresh and one create retry
- finalize 401 triggers one refresh and one finalize retry
- refreshed token is persisted back to the config file
- explicit `--token` override does not refresh and does not call `/v1/auth/login`

## Screenshot Inspection

I inspected the regenerated screenshot files after the actual CLI harness ran:

- `actual-app-run/full-transcript.png` shows `go build`, four live local server URLs, each actual `bv` invocation, CLI stdout/stderr, observed HTTP requests, config-after-run state, and PASS results.
- `actual-app-run/proactive-expired-token-refresh.png` shows `/v1/auth/login` before create, then create/upload/finalize using `ghs_new_proactive`.
- `actual-app-run/create-401-refresh-retry.png` shows create attempt 1 with `ghs_old_create_retry`, `/v1/auth/login`, then create attempt 2 with `ghs_new_create_retry`.
- `actual-app-run/finalize-401-refresh-retry.png` shows finalize attempt 1 with `ghs_old_finalize_retry`, `/v1/auth/login`, then finalize attempt 2 with `ghs_new_finalize_retry`.
- `actual-app-run/token-override-no-refresh.png` shows `--token ghs_override_rejected`, exit code 4, and only one `/v1/sites` request with no `/v1/auth/login`.

## Supporting Test/CI Evidence

These are secondary checks only; the actual running CLI screenshots above are the primary proof.

- [`01-manual-qa-terminal.png`](./01-manual-qa-terminal.png) shows the focused regression tests passing.
- [`02-ci-green-terminal.png`](./02-ci-green-terminal.png) shows `gh pr checks 9` with every remote CI check passing.
- [`manual-qa-output.txt`](./manual-qa-output.txt) preserves focused regression test output.
- [`ci-checks-output.txt`](./ci-checks-output.txt) preserves CI check output.

Additional local verification before the PR was opened:

```bash
go test -count=1 ./...
go vet ./...
lsp_diagnostics cmd/bv/clientutil.go cmd/bv/cmd_login.go cmd/bv/pushflow.go cmd/bv/main_test.go
```

All changed Go files returned `No diagnostics found`.
