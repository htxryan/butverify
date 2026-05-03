# Proof: Todoist 6gWV3Cc552m85Wp7

Task: [Polish `bv push` CLI output](https://app.todoist.com/app/task/6gWV3Cc552m85Wp7)

This proof exercises the built `bv` binary through the same CLI surface a user runs. The command targets a local fake control-plane that implements the real `POST /v1/sites`, signed `PUT`, and `POST /v1/sites/{id}/finalize` contract so the output can be deterministic without publishing a real site.

## Primary Evidence

- `human-push-terminal.png` — screenshot of `bv push ./demo-site` showing progress-bar status lines, the clear published URL, and structured metadata.
- `human-push.transcript.txt` — raw transcript used to render the screenshot.

## Backing Evidence

- `json-push-terminal.png` — screenshot proving `bv --json push ./demo-site` remains machine-readable and free of human progress/status text.
- `json-push.transcript.txt` — raw JSON-mode transcript.
- `human-server-trace.ndjson` — fake server trace for the human-mode run, showing create, upload, and finalize requests.
- `json-server-trace.ndjson` — fake server trace for the JSON-mode run.

## Automated Validation

Run from `/Users/redhale/src/butverify` unless noted:

```text
gofmt -l cmd/bv/pushflow.go cmd/bv/main_test.go
go test -count=1 ./cmd/bv -run 'TestPush'
go test -count=1 ./cmd/bv -run 'TestReport_Push|TestDashboard_Push|TestEvidence_Push'
go test -count=1 ./internal/output ./cmd/bv
go test -count=1 ./...
go vet ./...
go build -o /var/folders/_z/mrnvcw_j25746h9mwn9rqv8r0000gn/T/opencode/bv-push-output-proof-6gWV3Cc552m85Wp7/bv ./cmd/bv
```

Run from `/Users/redhale/src/butverify/marketing-site`:

```text
pnpm typecheck
pnpm test
pnpm build && pnpm link-check
```

All commands passed locally on 2026-05-02.
