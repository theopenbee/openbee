---
name: OpenBee Console
description: An enterprise workspace for managing a workforce of AI digital employees, built on the Arc UI library.
foundation: "Arc (https://uiarc.dev), tokens in web/src/components/arc/foundation.css"
colors:
  background: "oklch(100% 0 0)"
  surface: "oklch(100% 0 0)"
  surface-muted: "oklch(97.8% 0 0)"
  foreground: "oklch(15% 0 0)"
  text-secondary: "oklch(46% 0 0)"
  text-muted: "oklch(59% 0 0)"
  border: "oklch(93.5% 0 0)"
  border-subtle: "oklch(96.5% 0 0)"
  border-strong: "oklch(82% 0 0)"
  accent: "#f48120"
  accent-strong: "#aa4700"
  accent-subtle: "rgb(244 129 32 / .13)"
  brand: "oklch(70% .161 49)"
  success: "oklch(49% .19 150)"
  warning: "oklch(58% .18 75)"
  danger: "oklch(54% .21 25)"
typography:
  display:
    fontFamily: "Geist Variable, Noto Sans SC, sans-serif"
    fontSize: "1.75rem to 2.25rem"
    fontWeight: 500
    lineHeight: 1.1
    letterSpacing: "-0.03em"
  body:
    fontFamily: "Inter Variable, Noto Sans SC, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.4
    letterSpacing: "-0.01em"
  mono:
    fontFamily: "JetBrains Mono Variable, ui-monospace, monospace"
    fontSize: "0.75rem to 0.8125rem"
    fontWeight: 400
    lineHeight: 1.5
rounded:
  control: "1.125rem"
  panel: "1.625rem"
  surface: "2.125rem"
  inset: "max(6px, calc(2.125rem - 1.5rem))"
  pill: "9999px"
spacing:
  grid: "4px"
  group: "16px"
  region: "24px"
  section: "32px to 48px"
controls:
  height-sm: "36px"
  height-md: "44px"
  height-lg: "50px"
---

# Design System: OpenBee Console

## 1. Overview

**Creative North Star: "The Digital Workforce HQ" (企业数字员工工作台)**

OpenBee is the headquarters of a digital workforce. The operator manages AI workers the way a manager runs a team inside a serious enterprise office platform, and the interface is built on one conviction: an AI worker is a digital employee, equal in standing to a human colleague. Workers carry identity, presence, and history.

The visual language is **Arc** (https://uiarc.dev): a calm, neutral interface where type, spacing, and one considered interaction carry the character. Arc components (CSS modules, semantic tokens, Motion, Radix primitives) are vendored as source in `web/src/components/arc/` and installed with `pnpm arc:add <id>`. App code composes them and lays pages out with Tailwind utilities that resolve to the same Arc tokens.

**Key characteristics**
- Neutral surfaces, meaningful color: the orange accent marks what is active, selected, in progress, or emphasized, and nothing else.
- Soft, concentric corners: controls 18px, panels 26px, cards 34px, nested surfaces derive their radius from the parent.
- Hierarchy from size, spacing, and contrast, with weights 400 and 500 only.
- Motion explains a change: springs from shared tokens, transform and opacity only, reduced motion respected.
- Light and dark are equal citizens, switched with `data-theme` on `<html>` (light, dark, or follow the system).

## 2. Colors

Every color is a semantic Arc token with a light and a dark value. Tailwind exposes them as `background`, `surface`, `surface-raised`, `surface-muted`, `foreground`, `secondary`, `muted`, `border`, `border-subtle`, `border-strong`, `accent`, `accent-strong`, `accent-subtle`, `accent-foreground`, `success`, `warning`, `danger`, and `series-1` to `series-4`. The Tailwind palette is disabled.

- **Accent** (Arc `data-accent="orange"`): the active nav item, a selected row or option, progress, the one data series that matters, links. Tint with `accent-subtle` behind accent text or icons. Never on headings, card backgrounds, or every icon.
- **Brand orange** (`--brand`): reserved for the OpenBee logo mark and wordmark.
- **Status**: `success`, `warning`, `danger` appear only for real status and always with a label. Worker presence maps to them: idle is `success`, working is the accent with a soft pulse, error is `danger`.
- **Neutrals**: pure achromatic ramps; `secondary` for supporting text, `muted` for tertiary text and placeholders.

## 3. Typography

- **Geist** (`display-title` utility, `font-display`): page titles at 28 to 36px, weight 500, tracking -0.03em.
- **Inter** (`font-sans`, default): everything else, 14px body, weights 400 and 500.
- **Noto Sans SC** is the CJK fallback for both, so mixed Chinese and English copy renders as one voice.
- **JetBrains Mono**: logs, identifiers, keys, token counts.
- Sizes come from the Arc scale only: 12, 14, 16, 18, 22, 28, 36, 52, 72. Changing or aligned numbers use `tabular`.
- Copy is sentence case, with no uppercase transforms, no eyebrow labels, and no em dashes. Buttons are verb plus object.

## 4. Surfaces and elevation

- Group with proximity first, a 1px border second, a card only for a real boundary.
- Cards (`Panel`, `DetailSection`) are `rounded-surface` with a 1px `border` on `surface`, no shadow, 24px padding.
- Only floating layers cast shadows (`shadow-floating`): menus, popovers, dialogs, drawers, toasts.
- No nested decorative cards, no gradients or glows inside product UI, no icons inside rounded tiles.

## 5. Layout

- One page container in `components/layout.tsx`: `max-w-page` (1200px) with 16 to 32px gutters. Pages never add a second gutter; forms and settings narrow themselves with `max-w-form` (720px).
- Full-bleed routes (chat, workers, worker detail) own their padding and scroll, and keep the same left edge as other pages.
- Rhythm: 16px inside groups, 24px between regions, 32 to 48px between long-form sections.
- Desktop navigation is a 240px sidebar that collapses to a 64px icon rail; on phones it becomes a top bar with an Arc drawer.
- Check 390, 768, 1024, and 1440px: no sideways scroll, wide tables scroll inside their card, touch targets at least 44px.

## 6. Components

Use an Arc component before writing UI, and use only documented props.

| Need | Arc item |
| --- | --- |
| Actions | `button` (one primary per surface), `dropdown-menu` (with `iconOnly` for row menus), `copy-button`, `confirm-morph` |
| Fields | `input`, `textarea`, `password-field`, `search-field`, `number-field`, `select`, `combobox`, `switch`, `checkbox`, `radio-group`, `segmented-control` |
| Overlays | `dialog` (decisions that interrupt), `drawer` (long forms and record detail), `popover`, `tooltip`, `hover-card` |
| Data | `sortable-data-table`, `metric-card`, `line-chart`, `bar-chart`, `tree-view`, `pagination`, `badge`, `avatar` |
| States | `skeleton`, `empty-state`, `alert`, `text-shimmer` |
| Account | `user-menu` |

App-level wrappers in `web/src/components/`: `PageHeader`, `Panel`, `DetailSection`, `EmptyState`, `StatusBadge`, `WorkerAvatar`, `PaginationControls`, `CopyButton`, `Toaster`.

**Worker identity row (signature component).** Each worker appears as a colleague would in a team roster: avatar, name, and a presence dot driven by status, always paired with a text label.

## 7. Do's and don'ts

### Do
- Use Arc tokens and components; read `https://uiarc.dev/components/<id>/markdown` before using an item.
- Keep the accent rare and meaningful; pair every status color with a label.
- Design loading (skeleton in the final layout), empty (one next step), error (inline alert with retry), and in-place success for every data region.
- Keep nested corners concentric and sibling cards aligned to the same top and height.

### Don't
- Don't use the Tailwind radius scale, raw radii, palette colors, weights above 500, or arbitrary font sizes (`pnpm lint` guards these).
- Don't add hand-made focus rings; Arc's foundation draws the keyboard focus ring.
- Don't use a toast as the only confirmation of a foreground action.
- Don't add decorative gradients, glows, colored shadows, or motion that loops without meaning.
