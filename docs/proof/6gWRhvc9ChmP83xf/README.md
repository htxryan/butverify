# Credential Upload Guard Proof

Todoist task: `6gWRhvc9ChmP83xf`

This proof exercises the real `bv` CLI binary built from this branch against a temporary site containing an OpenSSH private key marker.

## Evidence

- `01-blocked-secret.status` is `1`, showing the default push was rejected.
- `01-blocked-secret.stdout` shows gitleaks detected `private-key`, the push was blocked before creating or uploading a site, and the explicit override flag is named `--skip-gitleaks-check`.
- `02-explicit-override.status` is `0`, showing the same site can only be pushed with the explicit override.
- `02-explicit-override.stdout` shows a successful remote publish result for the override path.

## Verification Commands

```bash
task docs:check
task format:check
go vet ./...
task build:go
task test:go
```
