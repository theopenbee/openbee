import { defineConfig, globalIgnores } from "eslint/config"
import tsParser from "@typescript-eslint/parser"
import tsPlugin from "@typescript-eslint/eslint-plugin"

const SIDES = "((t|b|l|r|s|e|tl|tr|bl|br|ss|se|es|ee)-)?"
const RADIUS_SCALE = `\\brounded-${SIDES}(xs|sm|md|lg|xl|2xl|3xl|4xl)\\b`
const RADIUS_ARBITRARY = `\\brounded-${SIDES}\\[`
const RADIUS_MESSAGE =
  "Use Arc radius tokens (see CLAUDE.md): rounded-control, rounded-panel, rounded-surface, rounded-inset, rounded-full or rounded-none."

const LEGACY_COLOR =
  "\\b(bg|text|border|ring|fill|stroke|from|via|to|divide|outline|decoration|placeholder|caret)-(muted-foreground|primary(-foreground|-strong)?|card(-foreground)?|popover(-foreground)?|secondary-foreground|destructive(-foreground)?|input|ring|brand(-foreground)?|status-(idle|working|error)|sidebar(-[a-z-]+)?)\\b"
const LEGACY_FILL = "\\b(bg-muted|bg-secondary)\\b(?!-)"
const COLOR_MESSAGE =
  "Use Arc semantic tokens (see CLAUDE.md): background, surface, surface-muted, foreground, secondary, muted, border, accent, success, warning, danger."

const WEIGHT = "\\bfont-(semibold|bold|extrabold|black)\\b"
const WEIGHT_MESSAGE = "Arc uses weights 400 and 500 only: font-normal or font-medium."

const restricted = (pattern, message) => [
  { selector: `Literal[value=/${pattern}/]`, message },
  { selector: `TemplateElement[value.cooked=/${pattern}/]`, message },
]

const eslintConfig = defineConfig([
  globalIgnores(["dist/**", "build/**"]),
  {
    files: ["src/**/*.{ts,tsx}"],
    ignores: ["src/components/ui/**", "src/components/arc/**"],
    linterOptions: { reportUnusedDisableDirectives: "off" },
    languageOptions: {
      parser: tsParser,
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
    plugins: { "@typescript-eslint": tsPlugin },
    rules: {
      "no-restricted-syntax": [
        "error",
        ...restricted(RADIUS_SCALE, RADIUS_MESSAGE),
        ...restricted(RADIUS_ARBITRARY, RADIUS_MESSAGE),
        ...restricted(LEGACY_COLOR, COLOR_MESSAGE),
        ...restricted(LEGACY_FILL, COLOR_MESSAGE),
        ...restricted(WEIGHT, WEIGHT_MESSAGE),
      ],
    },
  },
])

export default eslintConfig
