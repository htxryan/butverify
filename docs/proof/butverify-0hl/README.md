# Proof: bv push auth token refresh

Todoist task: `6gWRjmJRVqPHV2cf`
Beads issue: `butverify-0hl`
PR: https://github.com/htxryan/butverify/pull/9
Branch: `feat/bv-push-token-refresh`

## What Was Proved

This is a CLI-only change, so the user-visible surface is terminal execution of the `bv` push flow. The proof artifacts in this folder capture real command output and screenshot images generated from that output.

Verified observable outcomes:

- an expired saved installation token is refreshed before `POST /v1/sites`
- a 401 from `POST /v1/sites` refreshes the token and retries once
- an explicit `--token` override does not auto-refresh
- remote PR CI checks are green

## Screenshots

- [`01-manual-qa-terminal.png`](./01-manual-qa-terminal.png) shows the manual QA command and all three auth-refresh scenarios passing.
- [`02-ci-green-terminal.png`](./02-ci-green-terminal.png) shows `gh pr checks 9` with every remote CI check passing.

## Raw Command Evidence

Manual QA command output is preserved in [`manual-qa-output.txt`](./manual-qa-output.txt):

```text
$ go test -count=1 -v ./cmd/bv -run 'TestPushRefreshesExpiredTokenBeforeCreate|TestPushRefreshesAndRetriesCreateAfter401|TestPushTokenOverrideDoesNotAutoRefresh'
=== RUN   TestPushRefreshesExpiredTokenBeforeCreate
--- PASS: TestPushRefreshesExpiredTokenBeforeCreate (0.00s)
=== RUN   TestPushRefreshesAndRetriesCreateAfter401
--- PASS: TestPushRefreshesAndRetriesCreateAfter401 (0.00s)
=== RUN   TestPushTokenOverrideDoesNotAutoRefresh
--- PASS: TestPushTokenOverrideDoesNotAutoRefresh (0.00s)
PASS
ok  	github.com/htxryan/butverify/cmd/bv	0.346s
```

Remote CI command output is preserved in [`ci-checks-output.txt`](./ci-checks-output.txt):

```text
$ gh pr checks 9
CodeQL	pass	2s	https://github.com/htxryan/butverify/runs/74076490583
CodeQL (go)	pass	1m1s	https://github.com/htxryan/butverify/actions/runs/25264315173/job/74076449352
CodeQL (javascript-typescript)	pass	1m4s	https://github.com/htxryan/butverify/actions/runs/25264315173/job/74076449345
Go (ubuntu-latest)	pass	16s	https://github.com/htxryan/butverify/actions/runs/25264315170/job/74076449272
Marketing site (ubuntu-latest)	pass	39s	https://github.com/htxryan/butverify/actions/runs/25264315170/job/74076449274
Secret scan	pass	6s	https://github.com/htxryan/butverify/actions/runs/25264315170/job/74076449278
```

## Screenshot Inspection

I inspected both screenshot files after generating them:

- `01-manual-qa-terminal.png` clearly shows the manual QA command and `PASS` results for `TestPushRefreshesExpiredTokenBeforeCreate`, `TestPushRefreshesAndRetriesCreateAfter401`, and `TestPushTokenOverrideDoesNotAutoRefresh`.
- `02-ci-green-terminal.png` clearly shows `gh pr checks 9` with CodeQL, CodeQL (go), CodeQL (javascript-typescript), Go, Marketing site, and Secret scan all passing.

## Additional Verification

These checks passed before the PR was opened:

```bash
go test -count=1 ./...
go vet ./...
lsp_diagnostics cmd/bv/clientutil.go cmd/bv/cmd_login.go cmd/bv/pushflow.go cmd/bv/main_test.go
```

All changed Go files returned `No diagnostics found`.
