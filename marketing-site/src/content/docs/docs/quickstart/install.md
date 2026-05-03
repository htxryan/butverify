---
title: Install the CLI
description: Install the butverify CLI on macOS, Linux, or Windows.
---

The CLI is a single Go binary. Pick the install method that matches your machine.

## macOS / Linux (Homebrew)

```bash
brew install butverify/tap/bv
```

This installs from our [Homebrew tap](https://github.com/butverify/homebrew-tap)
and verifies the cosign signature on the release binary.

## Linux (manual)

Download the latest release from
[github.com/butverify/cli/releases](https://github.com/butverify/cli/releases),
extract `bv`, and move it onto your `$PATH`:

```bash
# Replace VERSION + ARCH with the values from the release page.
curl -fsSL "https://github.com/butverify/cli/releases/download/VERSION/bv_linux_ARCH.tar.gz" \
  | tar -xz
install -m 0755 bv /usr/local/bin/bv
```

## Windows

Download `bv_windows_amd64.zip` from the same release page, unzip, and add the
folder to your `PATH`. The CLI runs in PowerShell and `cmd.exe`.

## Verify

```bash
bv --version
```

You should see something like `bv 0.1.0 (rev abcdef0, signed)`.

## Publish Mode

Fresh installs default to local mode: `bv push <dir>` serves a filtered
static bundle on `127.0.0.1` until you interrupt the command. Use this for
same-machine previews without signing in.

Run `bv mode` to inspect the current default, `bv mode local` to switch back
to local previews, or `bv mode remote` to make push-style commands publish to
butverify.dev.

## Authenticate For Remote Publishing

butverify authenticates the CLI with a GitHub App installation token. After
[installing the GitHub App](https://app.butverify.dev) on your account or
organization, the dashboard surfaces an installation token you can hand to
the CLI:

```bash
# Paste the token when prompted:
bv login --token <token>

# Or pipe it from a script:
echo "$BV_TOKEN" | bv login
```

`bv login` calls `/v1/auth/whoami`, captures your tenant + account, writes
the token to `~/.config/butverify/config.json` with mode `0600`, and switches
the default publish mode to `remote`.

If you haven't installed the GitHub App yet, visit
[app.butverify.dev](https://app.butverify.dev) — the dashboard walks you
through it.

## Next

- [Push your first site →](/docs/quickstart/first-push)
- [Use JSON output in your agent →](/docs/quickstart/json-output)
