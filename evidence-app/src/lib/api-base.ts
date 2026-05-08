// pebble-4nwj — Resolve the control-plane API base URL from the host
// the bundle is running on.
//
//   Customer host suffix          → API base
//   ────────────────────────────────────────────────────────
//   *.butverify.dev               → https://api.butverify.dev
//   *-qa.butverify.dev            → https://qa-api.butverify.dev
//   *-dev.butverify.dev           → https://dev-api.butverify.dev
//   localhost:* (vite preview)    → http://localhost:8787 (or override)
//
// Naming follows the verified host convention (memory:
// butverify-host-naming-prod-role-butverify-dev-api): non-prod is
// `<env>-<role>` so QA api lives at qa-api.butverify.dev, NOT
// api-qa.butverify.dev. The customer-site wildcards put the env on the
// RIGHT of the wildcard (`*-qa.butverify.dev`) so the cert covers, but
// the api host puts it on the LEFT.

export interface ApiBaseInput {
  host: string; // window.location.host
  protocol?: string; // window.location.protocol — defaults to https:
}

const LOCAL_FALLBACK = "http://localhost:8787";

// Override hook for local dev: a global window.__BV_API_BASE__ string,
// if set, replaces the resolved value. The dev shell sets this so the
// gallery can talk to a locally-running control-plane Worker.
function readOverride(): string | null {
  if (typeof globalThis === "undefined") return null;
  const w = globalThis as { __BV_API_BASE__?: unknown };
  if (typeof w.__BV_API_BASE__ === "string" && w.__BV_API_BASE__.length > 0) {
    return w.__BV_API_BASE__;
  }
  return null;
}

export function resolveApiBase(input: ApiBaseInput): string {
  const override = readOverride();
  if (override) return override;

  const host = input.host.toLowerCase();
  const protocol = (input.protocol ?? "https:").toLowerCase();

  // Local dev: any localhost host → local control-plane (assumed
  // wrangler dev on :8787). Tests can override via window.__BV_API_BASE__.
  if (host.startsWith("localhost") || host.startsWith("127.0.0.1")) {
    return LOCAL_FALLBACK;
  }

  // Strip the port if present (the customer-site wildcards always run
  // on 443, but a synthetic test could supply :8787).
  const bareHost = host.split(":")[0] ?? host;

  // Determine the env from the host suffix.
  if (bareHost.endsWith("-dev.butverify.dev")) {
    return `${protocol}//dev-api.butverify.dev`;
  }
  if (bareHost.endsWith("-qa.butverify.dev")) {
    return `${protocol}//qa-api.butverify.dev`;
  }
  if (bareHost === "butverify.dev" || bareHost.endsWith(".butverify.dev")) {
    return `${protocol}//api.butverify.dev`;
  }

  // Unknown host — fail closed by returning the prod host so a
  // misconfigured deploy on a vanity domain can still attempt to
  // submit. The control-plane will reject with CORS if needed.
  return `${protocol}//api.butverify.dev`;
}

// Convenience: resolve from window.location at call time. Returns the
// LOCAL_FALLBACK in non-browser contexts (SSR / unit tests that didn't
// stub window) so tests don't need to mock window.
export function resolveApiBaseFromWindow(): string {
  if (typeof window === "undefined" || !window.location) {
    return LOCAL_FALLBACK;
  }
  return resolveApiBase({
    host: window.location.host,
    protocol: window.location.protocol,
  });
}
