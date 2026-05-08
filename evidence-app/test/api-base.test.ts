// pebble-4nwj — API-base resolver tests. The host-naming convention is
// asymmetric: customer wildcards put env on the right (`*-qa`), while
// the api host puts env on the left (`qa-api.`). Verified against the
// service repo's wrangler.toml.

import { afterEach, describe, expect, it } from "vitest";
import { resolveApiBase } from "../src/lib/api-base.js";

const g = globalThis as { __BV_API_BASE__?: string };

afterEach(() => {
  delete g.__BV_API_BASE__;
});

describe("resolveApiBase", () => {
  it("prod customer host → api.butverify.dev", () => {
    expect(resolveApiBase({ host: "abc123.butverify.dev" })).toBe(
      "https://api.butverify.dev",
    );
  });

  it("qa customer host → qa-api.butverify.dev (NOT api-qa)", () => {
    expect(resolveApiBase({ host: "abc123-qa.butverify.dev" })).toBe(
      "https://qa-api.butverify.dev",
    );
  });

  it("dev customer host → dev-api.butverify.dev", () => {
    expect(resolveApiBase({ host: "abc123-dev.butverify.dev" })).toBe(
      "https://dev-api.butverify.dev",
    );
  });

  it("localhost → http fallback", () => {
    expect(resolveApiBase({ host: "localhost:4321", protocol: "http:" })).toBe(
      "http://localhost:8787",
    );
  });

  it("override beats host detection", () => {
    g.__BV_API_BASE__ = "http://127.0.0.1:9999";
    expect(resolveApiBase({ host: "abc.butverify.dev" })).toBe("http://127.0.0.1:9999");
  });

  it("apex butverify.dev → api.butverify.dev (defensive)", () => {
    expect(resolveApiBase({ host: "butverify.dev" })).toBe("https://api.butverify.dev");
  });
});
