# Proof: bv push auth token refresh

Todoist task: `6gWRjmJRVqPHV2cf`
Beads issue: `butverify-0hl`
PR: https://github.com/htxryan/butverify/pull/9
Branch: `feat/bv-push-token-refresh`

## What Was Proved

This is a CLI-only change, so the real user surface is command execution rather than a browser UI. The proof below uses the same push flow exercised by `bv push` and verifies these observable outcomes:

- an expired saved installation token is refreshed before `POST /v1/sites`
- a 401 from `POST /v1/sites` refreshes the token and retries once
- an explicit `--token` override does not auto-refresh

## Manual QA Evidence

Command:

```bash
go test -count=1 -v ./cmd/bv -run 'TestPushRefreshesExpiredTokenBeforeCreate|TestPushRefreshesAndRetriesCreateAfter401|TestPushTokenOverrideDoesNotAutoRefresh'
```

Output:

```text
=== RUN   TestPushRefreshesExpiredTokenBeforeCreate
--- PASS: TestPushRefreshesExpiredTokenBeforeCreate (0.00s)
=== RUN   TestPushRefreshesAndRetriesCreateAfter401
--- PASS: TestPushRefreshesAndRetriesCreateAfter401 (0.00s)
=== RUN   TestPushTokenOverrideDoesNotAutoRefresh
--- PASS: TestPushTokenOverrideDoesNotAutoRefresh (0.00s)
PASS
ok  	github.com/htxryan/butverify/cmd/bv	0.257s
```

## CI Evidence

`gh pr checks 9` after opening the PR:

```text
CodeQL	pass	2s	https://github.com/htxryan/butverify/runs/74076395462
CodeQL (go)	pass	1m0s	https://github.com/htxryan/butverify/actions/runs/25264278971/job/74076357364
CodeQL (javascript-typescript)	pass	1m6s	https://github.com/htxryan/butverify/actions/runs/25264278971/job/74076357380
Go (ubuntu-latest)	pass	19s	https://github.com/htxryan/butverify/actions/runs/25264278973/job/74076357372
Marketing site (ubuntu-latest)	pass	40s	https://github.com/htxryan/butverify/actions/runs/25264278973/job/74076357378
Secret scan	pass	5s	https://github.com/htxryan/butverify/actions/runs/25264278973/job/74076357377
```

## Local Verification Evidence

These checks passed before the PR was opened:

```bash
go test -count=1 ./...
go vet ./...
lsp_diagnostics cmd/bv/clientutil.go cmd/bv/cmd_login.go cmd/bv/pushflow.go cmd/bv/main_test.go
```

All changed Go files returned `No diagnostics found`.
