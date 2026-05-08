import { defineConfig } from "vitest/config";

// Vitest config. Manifest tests are pure-logic (no DOM needed) so we run
// in the default node env. Component tests, when added, can opt into
// happy-dom or jsdom on a per-file basis.
export default defineConfig({
  test: {
    include: ["test/**/*.test.ts"],
  },
});
