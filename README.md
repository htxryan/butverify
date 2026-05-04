# butverify

The CLI agents use to preview static work locally or publish proof of finished work as a private gallery on butverify.dev.

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
bv push ./build                     # serve a directory locally by default
bv login                            # optional: switch default mode to remote
bv push --mode remote ./build       # publish a private microsite
bv agent-init                       # install the /butverify agent skill
```

Use `bv mode` to inspect or change the default publish mode. Fresh installs default to `local`; `bv login` switches the default to `remote`; `bv logout` clears saved auth and switches back to `local`.

`bv push` optimizes supported images before publishing. JPEGs use quality `75` by default, and PNGs are recompressed losslessly when the result is smaller. Use `bv push --image-quality <1-100> ./build` for a single push, or set `image_quality` in `$XDG_CONFIG_HOME/butverify/config.json` (default `~/.config/butverify/config.json`) to change the default for future pushes.

Before a push creates or uploads a site, the CLI scans the exact upload bundle with gitleaks and blocks potential credentials by default. If you intentionally need to publish matching content, rerun with `bv push --skip-gitleaks-check ./build`.

After installing the skill, your agent can run `/butverify` after delivering work — it captures evidence and publishes it remotely via `bv evidence --push --mode remote`.

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
