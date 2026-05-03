# Todoist 6gWpvvg93W5f3FWf Proof

This proof covers CLI-side image optimization before upload.

## Primary User-Facing Proof

- `manual-qa-transcript.txt` shows a built `bv` binary publishing a site through the real `bv push --mode remote --image-quality 40` command against a local control-plane-compatible HTTP surface. The uploaded tar contained `photo.jpg` recompressed from 1678 bytes to 786 bytes before upload.
- The same transcript shows `bv push --mode local --image-quality 101` returning exit code 2 with the user-facing `BAD_REQUEST` validation error.

## Supporting Verification

The implementation was also verified with:

```text
go test -count=1 ./...
go build ./cmd/bv
go vet ./...
gofmt -l .
task docs:check:cli
```

All commands exited successfully, and LSP diagnostics were clean on changed production files.
