# Generated Public API Docs Proof

Date: 2026-05-03

This proof demonstrates the generated Public API reference through the same surface a reader uses: the built butverify docs site at `/docs/reference/api/`.

## Primary Proof

- `01-public-api-reference-page.png` — browser screenshot of the built Public API docs page showing the generated page, public endpoint list, and the rendered auth guidance with `Bearer <butverify installation token>` preserved.

## Backing Evidence

- `rendered-api-page.html` — captured HTML from the same built route, confirming the page includes `POST /v1/auth/login` and renders the auth guidance in code formatting.

## Capture Commands

```bash
task build:site
python3 -m http.server 4174 --bind 127.0.0.1 --directory marketing-site/dist
pnpm exec playwright screenshot --full-page "http://127.0.0.1:4174/docs/reference/api/" docs/proof/generated-public-api-docs/01-public-api-reference-page.png
curl -s "http://127.0.0.1:4174/docs/reference/api/" -o docs/proof/generated-public-api-docs/rendered-api-page.html
```

## Verification

- Screenshot inspected immediately after capture: readable, not clipped, and shows the Public API docs with the expected auth placeholder and public endpoint list.
- Browser QA against the built route confirmed no console errors or JavaScript errors.
- The captured HTML contains `Bearer <butverify installation token>` and `POST /v1/auth/login`.
- The captured HTML does not contain the internal-only endpoints checked during QA: `/v1/dashboard`, `/v1/webhooks`, `/v1/auth/start`, `/v1/auth/callback`, `/v1/auth/logout`, `/v1/status`, `/v1/billing`, `/healthz`, or `operator`.
