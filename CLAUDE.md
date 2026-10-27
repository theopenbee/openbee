# OpenBee — Project Instructions

## Design System

The web UI (`web/src`) is built on [Arc](https://uiarc.dev) (agent index: `https://uiarc.dev/llms.txt`). Arc items install with `pnpm arc:add <id>` (run in `web/`; it wraps the shadcn CLI and strips comments) into `web/src/components/arc/`, and the design tokens live in `web/src/components/arc/foundation.css`. Use an existing Arc component before writing UI, and style with Arc semantic tokens (`--background`, `--surface`, `--foreground`, `--text-secondary`, `--border`, `--accent`, `--success`, `--warning`, `--danger`) instead of raw colors.

### Border radius (STRICT)

Corner radius follows **Arc's concentric radius tokens**. Never hard-code a radius.

| Token | Tailwind | Value | Use |
| --- | --- | --- | --- |
| `--radius-control` | `rounded-control` | 18px | Buttons, inputs, selects, small controls |
| `--radius-panel` | `rounded-panel` | 26px | Menus, popovers, nested panels |
| `--radius-surface` | `rounded-surface` | 34px | Cards, dialogs, drawers, large surfaces |
| `--radius-inset` | `rounded-inset` | surface − 24px (10px) | A surface nested inside a 24px-padded card |
| `--radius-pill` | `rounded-full` | 9999px | Tags, badges, status marks, avatars, dots |

- **Nested corners are concentric:** inner radius = outer radius − the padding between them, clamped at a small positive value, e.g. `max(6px, calc(var(--radius-surface) - 24px))`. Never repeat the outer radius on an inner surface.
- ❌ **Do NOT use:** the Tailwind radius scale (`rounded-sm`, `rounded-md`, `rounded-lg`, `rounded-xl`, `rounded-2xl`, `rounded-3xl`, `rounded-4xl`), raw radii (`border-radius: 12px`, `rounded-[10px]`), or restyled radii inside Arc components.
- ✅ **Use:** `var(--radius-*)` in CSS modules, and `rounded-control`, `rounded-panel`, `rounded-surface`, `rounded-inset`, `rounded-full` or `rounded-none` in Tailwind. `pnpm lint` enforces this.

## Code Style

### Comments (STRICT)

Do **not** write code comments. The only exception is a comment the code needs to run (e.g. `//go:build`, `//go:embed`, `//go:generate`, `// @ts-expect-error`, `// eslint-disable-next-line`, shebangs).

### Language

Prefer **English** for all text in code — identifiers, strings, log and error messages, test names. The only exception is user-facing text in i18n locale files.

## Changelog

Each `CHANGELOG.md` entry must be **at most 10 words** and written in **English**.
