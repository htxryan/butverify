# Todoist 6gWHR5gVqcwRXW84 Proof

This folder captures user-facing proof for the local-first `bv` CLI mode delivery.

- `local-push-site-browser-window.png` shows Chrome open to `127.0.0.1:55039` with a site served by the built `bv push --mode local` binary.
- `local-evidence-site-browser-window.png` shows Chrome open to `127.0.0.1:55040` with an evidence-template site served by `bv evidence --push --mode local`.
- `local-push-site-browser.png` and `local-evidence-site-browser.png` are direct browser page captures of those same local sites.
- `local-site-browser-urls.txt`, `local-push-browser.log`, and `local-evidence-browser.log` record the local URLs and CLI output used for the browser screenshots.
- `cli-proof-transcript.txt` shows the built CLI reporting fresh local mode, switching to remote, logging out to local mode, serving a site locally, and installing the `/butverify` skill without login.
- `hidden-file-backing.txt` shows the local server returning the rendered `index.html` while `/.secret` returns `404`, proving local mode serves the filtered publish set instead of raw dotfiles.
- `hidden-body.txt` is the HTTP response body for the hidden-file request.

Supporting verification also passed: `go test -count=1 ./...`, `go build ./cmd/bv`, `go vet ./...`, `gofmt -l .`, `pnpm test`, `pnpm typecheck`, and `pnpm build` for the marketing site.
