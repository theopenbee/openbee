import { defineConfig, globalIgnores } from "eslint/config"
import tsParser from "@typescript-eslint/parser"
import tsPlugin from "@typescript-eslint/eslint-plugin"

// Border radius is capped at `sm` (see CLAUDE.md › Design System).
// Forbid the graduated radius scale above `sm` in app code. Base shadcn
// components under components/ui are exempt (the radius tokens themselves
// are collapsed to `sm` in globals.css, so they still render squared).
const FORBIDDEN_RADIUS =
  "rounded-((t|b|l|r|s|e|tl|tr|bl|br|ss|se|es|ee)-)?(md|lg|xl|2xl|3xl|4xl)\\b"
const RADIUS_MESSAGE =
  "Border radius must be at most `rounded-sm` (see CLAUDE.md). Forbidden: rounded-md/lg/xl/2xl/3xl/4xl. Use rounded-sm."

const eslintConfig = defineConfig([
  globalIgnores(["dist/**", "build/**"]),
  {
    files: ["src/**/*.{ts,tsx}"],
    ignores: ["src/components/ui/**"],
    // Only the radius guard is enabled, so pre-existing `@typescript-eslint/*`
    // disable directives would read as "unused" — don't nag about them.
    linterOptions: { reportUnusedDisableDirectives: "off" },
    languageOptions: {
      parser: tsParser,
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
    // Registered so existing inline `@typescript-eslint/*` disable directives
    // resolve. No rules from it are enabled here — only the radius guard runs.
    plugins: { "@typescript-eslint": tsPlugin },
    rules: {
      "no-restricted-syntax": [
        "error",
        { selector: `Literal[value=/${FORBIDDEN_RADIUS}/]`, message: RADIUS_MESSAGE },
        { selector: `TemplateElement[value.cooked=/${FORBIDDEN_RADIUS}/]`, message: RADIUS_MESSAGE },
      ],
    },
  },
])

export default eslintConfig
