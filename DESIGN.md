---
name: OpenBee Console
description: An enterprise workspace for managing a workforce of AI digital employees.
colors:
  background: "oklch(1 0 0)"
  foreground: "oklch(0.145 0 0)"
  card: "oklch(1 0 0)"
  muted: "oklch(0.97 0 0)"
  muted-foreground: "oklch(0.556 0 0)"
  border: "oklch(0.922 0 0)"
  primary: "oklch(0.700 0.161 49)"
  primary-strong: "oklch(0.555 0.135 49)"
  primary-foreground: "oklch(0.985 0.005 60)"
  ring: "oklch(0.700 0.161 49)"
  status-idle: "oklch(0.527 0.154 150.069)"
  status-working: "oklch(0.546 0.185 264.376)"
  status-error: "oklch(0.577 0.245 27.325)"
  destructive: "oklch(0.577 0.245 27.325)"
typography:
  heading:
    fontFamily: "Oxanium Variable, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 500
    lineHeight: 1.375
    letterSpacing: "normal"
  body:
    fontFamily: "Oxanium Variable, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
    letterSpacing: "normal"
  mono:
    fontFamily: "JetBrains Mono Variable, ui-monospace, monospace"
    fontSize: "0.8125rem"
    fontWeight: 400
    lineHeight: 1.5
    letterSpacing: "normal"
  label:
    fontFamily: "Oxanium Variable, system-ui, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 500
    lineHeight: 1.4
    letterSpacing: "normal"
rounded:
  sm: "0.375rem"
  md: "0.5rem"
  lg: "0.625rem"
  xl: "0.875rem"
spacing:
  sm: "12px"
  md: "16px"
components:
  button-primary:
    backgroundColor: "{colors.primary-strong}"
    textColor: "{colors.primary-foreground}"
    rounded: "{rounded.lg}"
    padding: "0 12px"
    height: "32px"
  button-outline:
    backgroundColor: "{colors.background}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.lg}"
    padding: "0 12px"
    height: "32px"
  button-ghost:
    backgroundColor: "{colors.background}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.lg}"
    padding: "0 12px"
    height: "32px"
  input-default:
    backgroundColor: "{colors.background}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.lg}"
    padding: "4px 10px"
    height: "32px"
  card-default:
    backgroundColor: "{colors.card}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.xl}"
    padding: "16px"
  status-badge:
    backgroundColor: "{colors.status-working}"
    textColor: "{colors.status-working}"
    rounded: "{rounded.md}"
    padding: "2px 8px"
---

# Design System: OpenBee Console

## 1. Overview

**Creative North Star: "The Digital Workforce HQ" (企业数字员工工作台)**

OpenBee is the headquarters of a digital workforce. The operator manages AI workers the way a manager runs a team inside a serious enterprise office platform, and the interface is built on a single conviction: an AI worker is a digital employee, equal in standing to a human colleague. So workers are not rows in a job queue; they carry identity, presence, and history. The roster, the presence dots, the profiles, the activity, all the affordances we give people at work, are given to digital employees too. That dignity is the whole design.

The register is product, not brand. Design serves the work of directing a workforce during routine operational use. The aesthetic is calm, efficient, and precise, drawing on the clarity of Slack and Linear: pack meaningful information with restraint, do the heavy lifting so the operator's task feels effortless, and never make them think. Light mode is the primary home, the natural ambient for an office tool used through the working day; dark mode is held to the exact same standard.

Warmth lives in one place: a single brand orange (`#EC7B35`), the color of a workplace that respects the people, and the digital people, inside it. Everything else is a disciplined achromatic field so that orange, used sparingly, always means "this matters." The system explicitly rejects consumer-app energy (cartoon shapes, emoji, mascots), enterprise drab (heavy borders, dated grays, bloated nav), and verbose redundant copy. If the interface can say it, the words come out.

**Key Characteristics:**
- Warm brand orange used as a rare signal; an achromatic neutral field carries everything else.
- Worker state is read as employee presence: green (available/idle), purple (working/busy), red (blocked/error).
- Flat by default. Depth comes from hairline rings and tonal layering, never drop shadows.
- A single precise radius scale on a 10px base, machine-consistent across components.
- Light-primary, dark-equal. Both themes ship complete. WCAG AA throughout.

## 2. Colors

A restrained achromatic field warmed by one brand orange, plus three presence signals.

### Primary
- **Brand Orange** (`#EC7B35` / `oklch(0.700 0.161 49)`): The single warm accent and identity color. Used sparingly (≤10% of any screen) on active and selected states, focus rings, key indicators, and the brand mark. At L≈0.70 it is a fill-and-signal color, not a text color.
- **Strong Orange** (`oklch(0.555 0.135 49)`): A deepened shade of the same hue used only where white text must sit on an orange fill (solid primary buttons), so the label clears WCAG AA (≈5:1). Same identity, accessible contrast.

### Neutral
- **Paper / Ink** (`oklch(1 0 0)` light / `oklch(0.145 0 0)` dark): Base canvas and primary text; the two themes mirror each other.
- **Muted** (`oklch(0.97 0 0)` light / `oklch(0.269 0 0)` dark): Secondary fills, hover states, inert chips.
- **Muted Foreground** (`oklch(0.556 0 0)` light / `oklch(0.708 0 0)` dark): Secondary text, timestamps, metadata.
- **Hairline** (`oklch(0.922 0 0)` light / `oklch(1 0 0 / 10%)` dark): Borders, dividers, and the 1px rings that define every container.

### Status (presence signals)
- **Available / Idle Green** (`oklch(0.527 0.154 150)` light / `oklch(0.696 0.171 150)` dark): A digital employee at rest, or a task finished cleanly.
- **Working / Busy Purple** (`oklch(0.546 0.185 264)` light / `oklch(0.693 0.17 264)` dark): A worker actively executing. The workforce in motion.
- **Blocked / Error Red** (`oklch(0.577 0.245 27)` light / `oklch(0.704 0.191 22)` dark): A failure or destructive action. Shared with the `destructive` role; kept clearly redder than the brand orange so the two never blur.

### Named Rules
**The One-Orange Rule.** There is exactly one brand color and it is used rarely. Orange appears on ≤10% of any screen; its scarcity is what makes it read as "this matters." Never use orange for decoration, mood, or emphasis-by-default.

**The Orange-Is-Signal-Not-Text Rule.** Brand orange at L≈0.70 fails AA against both white and dark, so it never carries reading text. Text emphasis comes from weight. A label on an orange fill is either white on Strong Orange or dark ink on Brand Orange, never orange on neutral.

**The Achromatic-Field Rule.** Outside the one orange and the three presence colors, neutrals are pure (chroma `0`). The quiet field is what lets every signal land.

## 3. Typography

**Display / UI Font:** Oxanium Variable (with system-ui, sans-serif)
**Mono Font:** JetBrains Mono Variable (with ui-monospace, monospace)

**Character:** Oxanium is geometric and precise, with a faintly technical cut that reinforces the efficient, serious register without novelty. It carries every piece of interface text. JetBrains Mono handles logs, IDs, token counts, and command output, where alignment and small-size legibility matter most.

### Hierarchy
- **Heading** (Oxanium, 500, `1rem`–`1.125rem`, line-height 1.375): Card titles, section and page headings. Weight does most of the hierarchy work.
- **Body** (Oxanium, 400, `0.875rem` / `text-sm`, line-height 1.5): The default reading size. Cap measured prose at 65–75ch.
- **Label** (Oxanium, 500, `0.75rem` / `text-xs`): Badges, captions, control labels.
- **Mono** (JetBrains Mono, 400, `~0.8125rem`): Logs, identifiers, token counts, command output.

### Named Rules
**The Weight-Before-Size Rule.** Build hierarchy through weight contrast (400 body vs 500 headings) and restrained scale steps, not large type. This is a dense operational tool, not an editorial page.

**The No-Filler Rule.** Type carries only what matters. Cut restated headings and explanatory sentences the UI already shows; a label beats a paragraph.

## 4. Elevation

The system is flat by default. Depth is conveyed through hairline rings and tonal layering, never drop shadows. A card is lifted only by a single `ring-1` at 10% foreground opacity and, where needed, a step in background tone (sidebar and popovers sit slightly off the base). This keeps the interface reading as a machined office surface rather than a stack of floating consumer cards. Shadows, when present at all, belong only to transient overlays (dropdowns, dialogs) from the component library, never to resting surfaces.

### Named Rules
**The Hairline-Not-Shadow Rule.** Containers are defined by a 1px ring or border, not a box-shadow. To make a surface distinct, change its tone or its ring, never drop a shadow under it.

## 5. Components

### Buttons
- **Shape:** Gently squared corners (`rounded-lg`, 10px). Small and icon variants tighten proportionally.
- **Sizing:** Compact by default (`h-8`, 32px). Sizes range `xs` (24px) through `lg` (36px); icon buttons are square.
- **Primary:** Strong Orange fill with white label (`bg-primary-strong text-primary-foreground`), medium weight, `text-sm`. The deepened shade keeps the orange identity while clearing WCAG AA for the label.
- **Outline / Secondary / Ghost:** Transparent or muted fills revealing on `hover:bg-muted`; outline carries the standard hairline border.
- **Destructive:** Tinted, not solid, `bg-destructive/10 text-destructive` rising to `/20` on hover. Clear without shouting.
- **Hover / Focus / Active:** Transitions via `transition-all`. Focus is a 3px brand-orange ring (`ring-ring/50`) plus border shift, never an outline jump. Active presses translate down 1px (`active:translate-y-px`) for a precise, mechanical click.

### Cards / Containers
- **Corner Style:** `rounded-xl` (14px).
- **Background:** `bg-card` (mirrors the base surface per theme).
- **Depth Strategy:** Single `ring-1 ring-foreground/10`. No shadow. (See Elevation.)
- **Internal Padding:** `16px` default, tightening to `12px` in the `sm` size. Vertical rhythm via `gap-4` / `gap-3`.

### Inputs / Fields
- **Style:** `h-8` (32px), `rounded-lg`, hairline `border-input` over a transparent (light) or faintly tinted (dark) fill.
- **Focus:** Border shifts to the brand-orange `ring` with a 3px halo. Smooth `transition-colors`.
- **Error / Disabled:** Invalid borders and rings in `destructive`; disabled drops to 50% opacity with no pointer events.

### Worker Identity Row (signature component)
The component that carries the digital-employee thesis. Each worker is shown as a person would be in a team roster: an **avatar**, a **name**, and a **presence dot** driven by status (green available, purple working, red blocked). Presence is never color-only, the dot pairs with a short status label or icon. This row appears in the sidebar, worker lists, and headers, giving every AI worker the same identity affordances a human colleague would have.

### Status Badge
- **Style:** Outline badge colored entirely by presence: `bg-status-X/15 text-status-X border-status-X/20`. Idle maps to green, working to purple, error to red; `pending` falls back to neutral muted.
- **Aliases:** `completed → idle`, `running → working`, `failed → error`, so backend vocabulary maps onto the three presence signals without inventing new colors.
- **Never color-only:** the badge always carries its text label, satisfying the color-blind requirement.

### Navigation
- **Style:** A persistent left sidebar on its own tonal step (`bg-sidebar`), separated by a hairline. Items use ghost-button behavior with a clear active state; the active item is marked with the brand orange as a fill or indicator, not as orange text. Keyboard-first and quiet.

## 6. Do's and Don'ts

### Do:
- **Do** keep orange rare (≤10% of a screen) and reserved for "this matters": active states, focus, key indicators, the primary CTA.
- **Do** use white text on Strong Orange (`oklch(0.555 0.135 49)`) for solid orange buttons, so labels clear WCAG AA.
- **Do** read worker state as employee presence and always pair the color with an icon or text label.
- **Do** give every worker identity affordances (avatar, name, presence) as you would a human colleague.
- **Do** define containers with a single `ring-1 ring-foreground/10` and convey depth through tone.
- **Do** hold light mode (primary) and dark mode to the exact same polish; respect `prefers-reduced-motion`.
- **Do** prefer skeletons over spinners and inline states over modals; the user is technical and wants immediate feedback.

### Don't:
- **Don't** use brand orange for reading text or for decoration; at L≈0.70 it fails AA as text and dilutes the signal.
- **Don't** rely on color alone to convey status; green/purple/red must always carry an icon or label.
- **Don't** introduce consumer-app energy: no rounded cartoon shapes, no emoji-heavy UI, no mascots, no personality theater.
- **Don't** fall into enterprise drab: no heavy borders, no dated grays, no bloated nav chrome.
- **Don't** write verbose or redundant copy: no restated headings, no paragraphs where a label works. If the UI shows it, cut the words.
- **Don't** drop shadows under resting surfaces; use hairlines and tonal steps instead.
- **Don't** use a `border-left`/`border-right` greater than 1px as a colored stripe, gradient text, or decorative glassmorphism.
