# Proof: bv push auth token refresh

Todoist task: `6gWRjmJRVqPHV2cf`
Beads issue: `butverify-0hl`
PR: https://github.com/htxryan/butverify/pull/9
Branch: `feat/bv-push-token-refresh`

## Primary Proof: What A User Sees

The primary proof is now the same terminal output a user would see when running the real `bv` CLI in human mode. These screenshots intentionally hide server traces, config dumps, JSON mode, test output, and CI output.

User-facing screenshots live in [`user-terminal-run/`](./user-terminal-run/):

- [`01-expired-token-user-push.png`](./user-terminal-run/01-expired-token-user-push.png): user runs `bv push demo-site` after the saved token expired and sees `Refreshed auth token for alice`, followed by successful publish output.
- [`02-create-401-user-push.png`](./user-terminal-run/02-create-401-user-push.png): user runs `bv push demo-site`; the API rejects create once behind the scenes, but the CLI refreshes and still publishes successfully.
- [`03-finalize-401-user-push.png`](./user-terminal-run/03-finalize-401-user-push.png): user runs `bv push demo-site`; the API rejects finalize once behind the scenes, but the CLI refreshes and still publishes successfully.
- [`04-token-override-user-push.png`](./user-terminal-run/04-token-override-user-push.png): user runs `bv --token ghs_example_expired push demo-site` and sees a normal auth error, proving explicit token overrides are not silently refreshed.
- [`full-user-terminal-session.png`](./user-terminal-run/full-user-terminal-session.png): combined terminal view of the same user-facing runs.
- [`full-user-terminal-transcript.txt`](./user-terminal-run/full-user-terminal-transcript.txt): raw copyable terminal transcript.

Run command used to generate the user-facing proof:

```bash
python3 docs/proof/butverify-0hl/run_user_terminal_proof.py
```

## Backing Evidence: Live Local App Runs

The user-facing screenshots are backed by an actual running CLI harness. It builds the real `bv` binary, starts live local HTTP servers that implement the `bv push` control-plane and staging endpoints, seeds real `BV_CONFIG_PATH` config files, invokes `bv push`, and records the HTTP requests observed by the server.

Backing evidence lives in [`actual-app-run/`](./actual-app-run/):

- [`full-transcript.png`](./actual-app-run/full-transcript.png): complete screenshot of all actual `bv` runs with server observations.
- [`proactive-expired-token-refresh.png`](./actual-app-run/proactive-expired-token-refresh.png): server observes `/v1/auth/login` before create, then create/upload/finalize with the refreshed token.
- [`create-401-refresh-retry.png`](./actual-app-run/create-401-refresh-retry.png): server observes create attempt 1 with the old token, `/v1/auth/login`, then create attempt 2 with the refreshed token.
- [`finalize-401-refresh-retry.png`](./actual-app-run/finalize-401-refresh-retry.png): server observes finalize attempt 1 with the old token, `/v1/auth/login`, then finalize attempt 2 with the refreshed token.
- [`token-override-no-refresh.png`](./actual-app-run/token-override-no-refresh.png): server observes only `/v1/sites` with the explicit token override and no `/v1/auth/login`.
- [`transcript.txt`](./actual-app-run/transcript.txt): raw copyable transcript for the backing run.

Run command used to generate the backing evidence:

```bash
python3 docs/proof/butverify-0hl/run_actual_app_proof.py
```

## Screenshot Inspection

I inspected the final screenshots after regenerating them:

- `user-terminal-run/01-expired-token-user-push.png` shows only `bv push demo-site`, `Refreshed auth token for alice`, and normal successful human-mode publish output.
- `user-terminal-run/02-create-401-user-push.png` shows only `bv push demo-site`, refresh, and successful human-mode publish output.
- `user-terminal-run/03-finalize-401-user-push.png` shows only `bv push demo-site`, successful progress output, refresh, and final successful human-mode publish output.
- `user-terminal-run/04-token-override-user-push.png` shows only `bv --token ghs_example_expired push demo-site` and `bv: error UNAUTHENTICATED: token expired`.

## Supporting Test/CI Evidence

These are secondary checks only; the user-facing CLI screenshots above are the primary proof.

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
