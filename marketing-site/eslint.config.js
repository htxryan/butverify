// ESLint flat config for the marketing-site test suite + scripts.
//
// Scope: only `test/` (vitest specs, .ts) and `scripts/` (.mjs/.js).
// We do NOT lint `src/` — Astro components are typechecked via `astro check`
// (run as `pnpm typecheck`), and adding eslint-plugin-astro would balloon
// the surface area. If we ever want lint on .astro files, this is the file
// to extend.

import js from "@eslint/js";
import tseslint from "typescript-eslint";
import globals from "globals";

export default [
  // Ignore build output and vendored stuff.
  {
    ignores: ["dist/**", "node_modules/**", ".astro/**", "public/**"],
  },

  js.configs.recommended,

  // TypeScript test files.
  ...tseslint.configs.recommended,
  {
    files: ["test/**/*.ts"],
    languageOptions: {
      globals: {
        ...globals.node,
        ...globals.browser,
      },
    },
    rules: {
      // Vitest specs frequently have `expect(x).toBe(...)` etc; allow
      // unused vars prefixed with _ and disable the strictest typed rules
      // (we don't run with `parserOptions.project` because vitest specs
      // import lots of fixture-y shapes).
      "@typescript-eslint/no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
      ],
      "@typescript-eslint/no-explicit-any": "off",
    },
  },

  // Node scripts (.mjs / .js).
  {
    files: ["scripts/**/*.{js,mjs}"],
    languageOptions: {
      ecmaVersion: 2024,
      sourceType: "module",
      globals: globals.node,
    },
    rules: {
      "no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
      ],
    },
  },
];
