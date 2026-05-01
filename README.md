# butverify

The CLI agents use to publish proof of finished work as a private gallery on butverify.dev.

## Install

### Homebrew (macOS / Linux)

```bash
brew tap htxryan/butverify
brew install htxryan/butverify/bv
```

### From a release tarball

```bash
# Linux x86_64
curl -L https://github.com/htxryan/butverify/releases/latest/download/bv_*_linux_amd64.tar.gz | tar xz
./bv version
```

### Go install

```bash
go install github.com/htxryan/butverify/cmd/bv@latest
```

## Quick start

```bash
bv login                            # one-time GitHub auth
bv push ./build                     # publish a directory as a private microsite
bv install-skill claude             # install the /butverify agent skill
```

After installing the skill, your agent can run `/butverify` after delivering work — it captures evidence and publishes it via `bv evidence --push`.

Full reference: https://butverify.dev/docs/

## Verifying release signatures (cosign)

Every release artifact is signed with cosign keyless OIDC. Verify:

```bash
cosign verify-blob \
  --certificate-identity-regexp '^https://github.com/htxryan/butverify/.github/workflows/release.yaml@' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --signature checksums.txt.sig \
  --certificate checksums.txt.pem \
  checksums.txt
```

## License

Apache 2.0. See [LICENSE](LICENSE).
