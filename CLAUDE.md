# OpenBee — Project Instructions

## Design System

### Border radius (STRICT)

Corner radius must be **at most `sm`**. The graduated radius scale above `sm` is **forbidden** across all web UI (`web/src`).

- ❌ **Do NOT use:** `rounded-md`, `rounded-lg`, `rounded-xl`, `rounded-2xl`, `rounded-3xl`, `rounded-4xl` — nor their tokens (`--radius-md/lg/xl/2xl/3xl/4xl`, `var(--radius)` at `md` or above) or arbitrary radii larger than `sm` (e.g. `rounded-[10px]`).
- ✅ **Allowed:** `rounded-none`, `rounded-sm`, and `rounded-full` (reserved for true circles/pills only — avatars, status dots, icon chips).

When a component needs rounding, default to `rounded-sm`. This keeps the interface visually serious and squared.

## Code Style

### Comments (STRICT)

Do **not** write code comments. The only exception is a comment the code needs to run (e.g. `//go:build`, `//go:embed`, `//go:generate`, `// @ts-expect-error`, `// eslint-disable-next-line`, shebangs).

### Language

Prefer **English** for all text in code — identifiers, strings, log and error messages, test names. The only exception is user-facing text in i18n locale files.

## Changelog

Each `CHANGELOG.md` entry must be **at most 10 words** and written in **English**.
