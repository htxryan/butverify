# Todoist 6gWHR5gVqcwRXW84 Proof

This folder captures user-facing proof for the local-first `bv` CLI mode delivery.

- `cli-proof-transcript.txt` shows the built CLI reporting fresh local mode, switching to remote, logging out to local mode, serving a site locally, and installing the `/butverify` skill without login.
- `hidden-file-backing.txt` shows the local server returning the rendered `index.html` while `/.secret` returns `404`, proving local mode serves the filtered publish set instead of raw dotfiles.
- `hidden-body.txt` is the HTTP response body for the hidden-file request.

Supporting verification also passed: `go test -count=1 ./...`, `go build ./cmd/bv`, `go vet ./...`, `gofmt -l .`, `pnpm test`, `pnpm typecheck`, and `pnpm build` for the marketing site.
