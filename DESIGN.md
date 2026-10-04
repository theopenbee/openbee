---
name: OpenBee Console
description: A Cloudflare-grade control plane for a digital workforce, set in Kumo grammar.
colors:
  primary: "oklch(0.5772 0.2324 260)"
  primary-hover: "oklch(0.488 0.243 264.376)"
  primary-edge: "oklch(0.5 0.21 260)"
  primary-foreground: "oklch(1 0 0)"
  link: "oklch(0.424 0.199 265.638)"
  brand: "oklch(0.700 0.161 49)"
  logo-ink: "#1d2126"
  canvas: "oklch(0.9875 0 0)"
  background: "oklch(1 0 0)"
  elevated: "oklch(0.98 0 0)"
  recessed: "oklch(0.96 0 0)"
  muted: "oklch(0.97 0 0)"
  strong: "oklch(0.145 0 0)"
  foreground: "oklch(0.21 0.006 285.885)"
  muted-foreground: "oklch(0.54 0 0)"
  border: "oklch(0.145 0 0 / 0.1)"
  hairline: "oklch(0.935 0 0)"
  input: "oklch(0.145 0 0 / 0.14)"
  focus: "oklch(0.15 0 0)"
  success: "oklch(0.508 0.118 165.612)"
  success-tint: "oklch(0.962 0.043 156.7 / 0.8)"
  warning: "oklch(0.51 0.125 55)"
  warning-tint: "oklch(0.931 0.107 94.6 / 0.35)"
  info: "oklch(0.424 0.199 265.638)"
  info-tint: "oklch(0.932 0.032 255.6 / 0.7)"
  destructive: "oklch(0.577 0.245 27.325)"
  destructive-foreground: "oklch(0.505 0.213 27.518)"
  danger-tint: "oklch(0.936 0.032 17.7 / 0.6)"
  status-idle: "oklch(0.508 0.118 165.612)"
  status-working: "oklch(0.546 0.245 262.881)"
  status-error: "oklch(0.577 0.245 27.325)"
  chart-1: "oklch(0.5772 0.2324 260)"
typography:
  headline:
    fontFamily: "Inter Variable, Noto Sans SC, system-ui, sans-serif"
    fontSize: "20px"
    fontWeight: 600
    lineHeight: "28px"
    letterSpacing: "-0.015em"
  metric:
    fontFamily: "Inter Variable, Noto Sans SC, system-ui, sans-serif"
    fontSize: "24px"
    fontWeight: 600
    lineHeight: 1
    letterSpacing: "-0.01em"
    fontFeature: "\"tnum\""
  title:
    fontFamily: "Inter Variable, Noto Sans SC, system-ui, sans-serif"
    fontSize: "16px"
    fontWeight: 600
    lineHeight: 1.375
    letterSpacing: "-0.01em"
  title-sm:
    fontFamily: "Inter Variable, Noto Sans SC, system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 600
    lineHeight: "20px"
    letterSpacing: "-0.01em"
  body:
    fontFamily: "Inter Variable, Noto Sans SC, system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: "20px"
    letterSpacing: "-0.01em"
    fontFeature: "\"cv02\", \"cv03\", \"cv04\", \"calt\""
  body-strong:
    fontFamily: "Inter Variable, Noto Sans SC, system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 500
    lineHeight: "20px"
    letterSpacing: "-0.01em"
  body-sm:
    fontFamily: "Inter Variable, Noto Sans SC, system-ui, sans-serif"
    fontSize: "13px"
    fontWeight: 400
    lineHeight: "20px"
    letterSpacing: "-0.01em"
  nav-label:
    fontFamily: "Inter Variable, Noto Sans SC, system-ui, sans-serif"
    fontSize: "13px"
    fontWeight: 500
    lineHeight: "20px"
    letterSpacing: "-0.01em"
  label:
    fontFamily: "Inter Variable, Noto Sans SC, system-ui, sans-serif"
    fontSize: "12px"
    fontWeight: 500
    lineHeight: "16px"
    letterSpacing: "-0.01em"
  mono:
    fontFamily: "JetBrains Mono Variable, ui-monospace, monospace"
    fontSize: "13px"
    fontWeight: 400
    lineHeight: "20px"
    letterSpacing: "normal"
rounded:
  none: "0px"
  sm: "0.1875rem"
  full: "9999px"
spacing:
  "1": "4px"
  "1.5": "6px"
  "2": "8px"
  "2.5": "10px"
  "3": "12px"
  "4": "16px"
  "5": "20px"
  "6": "24px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.primary-foreground}"
    typography: "{typography.body-strong}"
    rounded: "{rounded.sm}"
    padding: "0 12px"
    height: "36px"
  button-primary-hover:
    backgroundColor: "{colors.primary-hover}"
  button-outline:
    backgroundColor: "{colors.background}"
    textColor: "{colors.foreground}"
    typography: "{typography.body-strong}"
    rounded: "{rounded.sm}"
    padding: "0 12px"
    height: "36px"
  button-outline-hover:
    backgroundColor: "{colors.muted}"
    textColor: "{colors.strong}"
  button-ghost:
    textColor: "{colors.foreground}"
    typography: "{typography.body-strong}"
    rounded: "{rounded.sm}"
    padding: "0 12px"
    height: "36px"
  button-ghost-hover:
    backgroundColor: "{colors.muted}"
    textColor: "{colors.strong}"
  button-destructive:
    backgroundColor: "{colors.destructive}"
    textColor: "{colors.primary-foreground}"
    typography: "{typography.body-strong}"
    rounded: "{rounded.sm}"
    padding: "0 12px"
    height: "36px"
  button-sm:
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "0 8px"
    height: "28px"
  input:
    backgroundColor: "{colors.background}"
    textColor: "{colors.foreground}"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
    padding: "4px 12px"
    height: "36px"
  select-trigger:
    backgroundColor: "{colors.background}"
    textColor: "{colors.foreground}"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
    padding: "8px 8px 8px 12px"
    height: "36px"
  badge-status-idle:
    backgroundColor: "{colors.success-tint}"
    textColor: "{colors.success}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "0 6px"
    height: "20px"
  badge-status-working:
    backgroundColor: "{colors.info-tint}"
    textColor: "{colors.info}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "0 6px"
    height: "20px"
  badge-status-error:
    backgroundColor: "{colors.danger-tint}"
    textColor: "{colors.destructive-foreground}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "0 6px"
    height: "20px"
  badge-status-pending:
    backgroundColor: "{colors.muted}"
    textColor: "{colors.foreground}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "0 6px"
    height: "20px"
  status-dot:
    rounded: "{rounded.full}"
    size: "6px"
  panel:
    backgroundColor: "{colors.elevated}"
    rounded: "{rounded.sm}"
  panel-header:
    backgroundColor: "{colors.elevated}"
    textColor: "{colors.strong}"
    typography: "{typography.title-sm}"
    padding: "8px 16px"
    height: "44px"
  panel-body:
    backgroundColor: "{colors.background}"
    rounded: "{rounded.sm}"
    padding: "16px"
  stat-tile:
    backgroundColor: "{colors.background}"
    textColor: "{colors.strong}"
    typography: "{typography.metric}"
    rounded: "{rounded.sm}"
    padding: "16px"
  table-head:
    textColor: "{colors.strong}"
    typography: "{typography.title-sm}"
    padding: "0 12px"
    height: "40px"
  table-cell:
    textColor: "{colors.foreground}"
    typography: "{typography.body}"
    padding: "12px"
  table-row-hover:
    backgroundColor: "{colors.elevated}"
  topbar:
    backgroundColor: "{colors.background}"
    padding: "0 16px"
    height: "48px"
  sidebar:
    backgroundColor: "{colors.background}"
    width: "260px"
  nav-item:
    textColor: "{colors.foreground}"
    typography: "{typography.nav-label}"
    rounded: "{rounded.sm}"
    padding: "8px 12px"
    height: "34px"
  nav-item-active:
    backgroundColor: "{colors.recessed}"
    textColor: "{colors.strong}"
  tabs-list:
    backgroundColor: "{colors.recessed}"
    rounded: "{rounded.sm}"
    padding: "2px"
    height: "36px"
  tabs-trigger-active:
    backgroundColor: "{colors.background}"
    textColor: "{colors.strong}"
    typography: "{typography.body-strong}"
    rounded: "{rounded.sm}"
  tooltip:
    backgroundColor: "{colors.strong}"
    textColor: "{colors.background}"
    rounded: "{rounded.sm}"
    padding: "6px 10px"
  dialog:
    backgroundColor: "{colors.background}"
    rounded: "{rounded.sm}"
    padding: "20px"
    width: "384px"
  dialog-footer:
    backgroundColor: "{colors.elevated}"
    padding: "12px 20px"
  alert-destructive:
    backgroundColor: "{colors.danger-tint}"
    textColor: "{colors.destructive-foreground}"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
    padding: "12px 16px"
  avatar:
    backgroundColor: "{colors.muted}"
    textColor: "{colors.muted-foreground}"
    rounded: "{rounded.full}"
    size: "32px"
---

# Design System: OpenBee Console

## Overview

**Creative North Star: "The Workforce Control Plane"**

The OpenBee console is a control plane for a digital workforce, written in the grammar of Cloudflare's Kumo design system. It reads like infrastructure you trust: a near-white canvas, white working surfaces drawn with a single 1px line, gray header strips that name each section, and one blue that marks the action you came to take. Workers keep their colleague standing through identity (initials avatar, presence dot, history), not through decoration. The hive metaphor shows up only as geometry, never as a mascot or a brand wash.

Density is operational. Controls sit at 36px, table rows breathe at 12px of cell padding, and every metric, timestamp and count sets in tabular numerals so columns line up at a glance. Type stays on a tight Kumo ramp (12/13/14/16px) with Inter's open digits and slight negative tracking, set beside Noto Sans SC so Chinese and English carry equal weight. Corners are squared to 3px, a deliberate translation of Kumo's 8px under the project's strict radius cap.

The system turns down the generic shadcn admin look: soft gray cards floating on white, brand color spent on every button. It also turns down playful consumer chrome (cartoon shapes, emoji UI) and drab enterprise heaviness (thick borders, dated grays, bloated navigation). Light is the default theme; dark is a full peer with its own values, listed under Colors.

**Key Characteristics:**
- Kumo tonal layering: canvas ground, white base surfaces with a 10%-black line, gray elevated header strips.
- One blue for primary action, links, focus-visible and selection; OpenBee orange appears only in the logo.
- Status shown as tint + dot + label, never by color alone.
- Squared corners (3px), with full circles kept for avatars, dots and switches.
- A 12/13/14/16px type ramp in Inter + Noto Sans SC, tabular numerals for all data.
- Flat surfaces. Shadows appear only on floating layers and as a hairline under controls.

## Colors

A neutral, almost colorless field where a single saturated blue does all the pointing, and status hues appear only as soft tints.

### Primary
- **Control Blue** (primary): the one action color. It fills the primary button (one per view, top-right of the page header), the checked switch track, the active underline of line tabs, the focus-visible ring, the trend-chart line (chart-1) and, at 22% mix, the text selection. Its hover deepens to **Pressed Blue** (primary-hover), and filled buttons carry a 1px **Blue Edge** ring (primary-edge).
- **Link Ink** (link): a deeper blue for inline links and linked identifiers (session IDs in mono, "view all" links). The same value backs the info badge text (info).

### Secondary
- **OpenBee Orange** (brand): the logo mark and wordmark only, bound into the SVG so it follows the theme. It is never a UI color. The mark's outline uses **Logo Ink** (logo-ink), dark on light grounds and near-white in dark mode so the head never dissolves.

### Neutral
- **Canvas** (canvas): the page ground behind all content, and the main area under the top bar. It is never a surface that holds content directly in a panel.
- **Base White** (background): every working surface: panels' bodies, stat tiles, tables, inputs, outline buttons, the top bar and the sidebar.
- **Elevated Gray** (elevated): the header strip of a LayerCard panel, dialog footers, table footers, and table row hover.
- **Recessed Gray** (recessed): wells set below the surface: the tabs track, code blocks and trigger-input blocks, attachment chips, active sidebar item fill.
- **Muted Gray** (muted): quiet fills: hover on outline and ghost buttons, neutral badges, avatar fallbacks, skeletons, rail selections.
- **Strong Ink** (strong): titles, table heads, active nav labels, metric values, tooltip fill.
- **Body Ink** (foreground): default text, a near-black with a faint cool cast.
- **Subtle Ink** (muted-foreground): captions, field labels, secondary meta, placeholders, icons at rest.
- **Line** (border): the 1px outline of every surface, at 10% black so it tints whatever it crosses.
- **Hairline** (hairline): opaque dividers inside a surface: table rows, divided lists, metric splits, menu separators.
- **Input Line** (input): the slightly heavier 14% outline of text fields and select triggers.
- **Focus Ink** (focus): the neutral focus ring of text inputs, used at 50%.

### Status
- **Idle / Success** (success on success-tint), **Working / Info** (info on info-tint), **Error / Danger** (destructive-foreground on danger-tint), **Warning** (warning on warning-tint): tint-on-text pairs for badges and inline alerts. The tints are translucent so they sit cleanly on white and on elevated strips.
- **Presence dots** (status-idle, status-working, status-error): the solid hues for the 6px dot inside a status badge and the presence dot on a worker avatar. Destructive (destructive) fills the destructive button.

### Dark theme
Dark mode keeps the same roles with its own values: canvas oklch(0.1 0 0), base oklch(0.17 0 0), and an elevated strip that drops below the base to oklch(0.12 0 0), so header strips recede instead of lifting. Recessed is oklch(0.15 0 0) and muted fills oklch(0.269 0 0). Lines turn solid at oklch(0.32 0 0) with hairlines at oklch(0.269 0 0). The action fill steps down to oklch(0.53 0.215 260) to hold white text, while link, info, focus ring, chart-1 and the working dot lift to oklch(0.707 0.165 254.624). The brand orange lifts to oklch(0.753 0.135 51) and logo ink turns near-white.

### Named Rules
**The One Blue Rule.** Control Blue means "act here" and nothing else: primary action, link, focus-visible, selection. Navigation selection, decoration, icons and headings never use it.

**The Mark-Only Orange Rule.** OpenBee orange lives inside the logo SVG and nowhere else: no orange buttons, washes, chart series or highlights. (chart-2 aliases the orange and stays unused.)

**The Tint, Dot, Label Rule.** Every state renders as a soft tint, a solid 6px dot and a translated text label together. Color alone never carries state.

**The Neutral Delta Rule.** Token-usage change is a measurement, not a verdict: the day-over-day delta sets in Strong Ink with an arrow icon, never in success green or danger red.

## Typography

**Display Font:** none. The console has no display face; the page title is the largest text.
**Body Font:** Inter Variable (with Noto Sans SC for CJK, then system-ui)
**Label/Mono Font:** JetBrains Mono Variable (with ui-monospace) for IDs, keys, versions, engine names and code

**Character:** A neutral, engineered sans tuned the Kumo way: -0.01em tracking on everything, Inter's open-digit alternates (cv02, cv03, cv04) and contextual alternates on globally, and Noto Sans SC at matching weights so mixed Chinese and English lines sit level. Mono resets tracking to normal.

### Hierarchy
- **Headline** (600, 20px, 28px, -0.015em): the page title, left-aligned in the page header or the full-bleed header strip. One per page.
- **Metric** (600, 24px, line-height 1, tabular): headline counts in stat tiles and token-usage metrics. Stat tiles drop to 20px under 640px; detail overview stats use 20px and session-detail stats use 16px.
- **Title** (600, 16px, 1.375): dialog titles, card titles, empty-state titles.
- **Title Small** (600, 14px, 20px): panel header titles, table heads, rail and section headings, list primary names at 500.
- **Body** (400, 14px, 20px): default text, table cells, form values, descriptions. Chat transcripts use 24px line-height.
- **Body Small** (400, 13px, 20px): secondary meta under a primary line (worker descriptions, timestamps, durations), breadcrumbs, key/value terms, pagination counts. **Nav Label** is the same 13px at 500 for sidebar and rail items (600 when active).
- **Label** (500, 12px, 16px, sentence case): field labels above metrics and key/value pairs, badges, small buttons.
- **Mono** (400, 13px, 20px, normal tracking): session IDs, env keys, versions, commit hashes; 12px for engine slugs and build info.

### Named Rules
**The Four-Step Ramp Rule.** Running text uses only 12, 13, 14 and 16px. 20px is the page title; 20 to 24px is reserved for metric numerals. Nothing else.

**The Three Weights Rule.** Weights are 400, 500 and 600 only, matching the Noto Sans SC cuts the console loads. Never 700.

**The Sentence-Case Label Rule.** Captions and field labels are 12px medium in Subtle Ink, sentence case, with no uppercase or wide tracking, so CJK labels set evenly.

**The Tabular Data Rule.** Every number a user compares (counts, tokens, durations, timestamps, page numbers) sets in tabular numerals.

## Layout

**Shell.** A full-width 48px top bar (Base White, 1px bottom line) carries the 24px-tall logo, a 20°-slanted divider, the breadcrumb, and the account menu at the far right. Below it, a 260px white sidebar sits against the Canvas main area. The sidebar collapses to a 48px icon rail with tooltips, and under 768px it moves off-canvas into a 288px sheet opened from a trigger in the top bar (the breadcrumb hides there). Standard pages scroll inside the main area with 24px padding.

**Page header.** Title (and an optional one-line subtitle) at the left, actions at the right with 8px gaps, 24px of space below. It wraps rather than truncates on narrow screens.

**Full-bleed workspaces.** Workers, worker detail and chat own the whole main area. Workers and worker detail use a 240px white rail with a 72px header strip, a 72px white page-header strip with a 1px bottom line, and a scrolling canvas body padded 24px. Worker-detail content is capped at 1024px. Chat centers a 896px column: operator messages align right in white outlined bubbles capped at 640px, Bee replies align left up to 832px, and the composer docks at the bottom.

**Grids.** The overview pairs a fluid main column with a 360px side column from 1024px, and stacks into one column below. Stat tiles are always three across (12px gap, 16px from 640px). Session detail goes two-column at 1280px (an 18 to 22rem sticky execution list beside the content), and its metadata grid steps from 2 to 5 columns. Forms and settings cap at 768px; dialogs at 384px (448px for wider forms).

**Rhythm.** A 4px base: 6 to 8px between related controls, 12px cell and row padding, 16px for panel padding and grid gaps, 24px for page padding and between stacked sections.

**Responsive tables.** Wide tables keep a 760px minimum from 768px and scroll horizontally inside their surface with a soft edge fade on the overflowing side. Below 768px, secondary columns hide and their facts (status, engine) move under the primary cell.

### Named Rules
**The Canvas-Is-Ground Rule.** Content never sits loose on the canvas. It always lives on a Base White surface outlined by the Line, or inside a full-bleed white strip.

**The Earned Purpose Line Rule.** The page-header subtitle is optional. It appears only where the product already carries that copy or a live count. Never write a purpose line just to fill the slot.

## Elevation & Depth

Depth comes from tone, not shadow. The canvas sits lowest; Base White surfaces rest on it, outlined by a 1px Line at 10% black; Elevated Gray strips sit on top of a surface to name it; Recessed Gray wells sink into it. Resting panels, tiles and tables carry no shadow at all. Shadows are reserved for things that float (menus, selects, dialogs, sheets) and for a hairline drop under controls so buttons and fields read as pressable. In dark mode the elevated strip goes darker than the base and the shadow edge turns into a 10% white line.

### Shadow Vocabulary
- **Popover** (`box-shadow: 0 0 0 1px var(--shadow-edge), 0 4px 8px -2px var(--shadow-drop), 0 12px 32px -8px var(--shadow-drop)`): dropdown menus, select lists, dialogs and sheets. The 1px edge replaces a border.
- **Control hairline** (`box-shadow: 0 1px 2px 0 rgb(0 0 0 / 0.05)`): filled and outline buttons, text inputs, select triggers, the active segment of default tabs, the chat composer.
- **Switch thumb** (`box-shadow: 0 1px 3px 0 rgb(0 0 0 / 0.1), 0 1px 2px -1px rgb(0 0 0 / 0.1)`): the switch thumb only.
- **Scrim** (black at 30%): behind dialogs and sheets.

### Named Rules
**The Flat Surface Rule.** Panels, stat tiles, tables and detail sections are flat at rest. If something casts a shadow, it floats.

**The Layered Strip Rule.** A section is named by an Elevated Gray header strip sitting over its white body (the Kumo LayerCard), never by a heading floating above a shadowed card.

## Shapes

Corners are squared: 3px (rounded sm) on every surface, control, badge, menu, tooltip, dialog and code block. This is a cited translation of Kumo's 8px under the project's strict radius cap in CLAUDE.md, not drift. The larger radius scale is collapsed to 3px at the token level so stray legacy classes still render squared. Full circles are kept for true circles and pills: avatars, status and presence dots, the switch track and thumb, and the small icon chip in chat.

Edges are drawn with 1px rings rather than borders, so outlines never change a component's box size; structural dividers (top bar, rails, header strips, table rows) use 1px borders in Line or Hairline. The only recurring geometry is the empty-state honeycomb: seven outlined hexagons in Subtle Ink at 35% with 5 to 8% fills.

### Named Rules
**The Squared Corner Rule.** Corner radius is 3px or none. Never use rounded-md or larger, arbitrary radii above 3px, or Kumo's native 8px.

**The True-Circle Rule.** A full radius is allowed only on things that are actually circles or pills: avatars, dots, switches, icon chips.

## Components

### Buttons
Quiet and exact; one filled blue button per view.
- **Shape:** squared (3px), 36px tall with 12px side padding; small is 28px with 12px text; icon buttons are 36px or 28px squares.
- **Primary:** Control Blue fill, white 14px medium label, a 1px Blue Edge ring, a faint 8% white top sheen and the control hairline shadow. Hover deepens to Pressed Blue over 150ms.
- **Focus:** a 2px Control Blue ring with a 1px offset in the base color on filled buttons; a 2px blue ring on the rest.
- **Outline:** Base White with a 1px Line ring and the control hairline shadow; hover fills Muted Gray and lifts the label to Strong Ink. Used for secondary actions and pagination.
- **Ghost:** no fill; hover fills Muted Gray. Used for row actions (the "more" menu), close buttons and the sidebar trigger.
- **Destructive:** Destructive red fill with white label, darkening 10% on hover; used to confirm deletions, never as a page's default action.
- **Link:** Link Ink text, underline on hover.
- **Disabled:** 50% opacity, no pointer events.

### Badges (status chips)
- **Style:** 20px tall, 6px side padding, 3px corners, 12px medium label, tint background with text of the same hue.
- **Status:** idle (success tint), working (info tint), error (danger tint), pending (Muted Gray), each with a 6px solid dot before the translated label. Aliases map completed to idle, running to working, failed to error.
- **Other variants:** neutral (Muted Gray), outline (white with an inset Line ring), inverted (Strong Ink fill) for rare emphasis.

### Cards / Containers
- **LayerCard panel (signature):** an Elevated Gray frame with a 1px Line ring and 3px corners; a header strip at least 44px tall with 8px/16px padding holds the 14px semibold title (optional 13px description) and actions at the right; the white body sits flush inside with its own 1px ring, padded 16px, or flush for tables, divided lists and metric grids. Use it for every titled section.
- **Stat tile:** Base White, 1px Line ring, 3px corners, 16px padding (12px under 640px); a 12px field label over a 24px tabular value. No icon, logo or tint.
- **Detail section:** Base White with a 1px Line ring for untitled groups; key/value rows divided by hairlines, 13px Subtle Ink term beside a 14px value, stacking under 448px of container width.
- **Shadow Strategy:** none at rest (see Elevation & Depth).
- **Divided grids:** cells inside a surface split by hairlines (token metrics, quick links), never nested outlined tiles.

### Inputs / Fields
- **Style:** Base White, 1px Input Line ring, 3px corners, 36px tall, 12px side padding, 14px text, Subtle Ink placeholder, control hairline shadow. Labels are 14px medium above the field with 6px between.
- **Focus:** a neutral 1.5px ring in Focus Ink at 50% (Kumo's input focus), not the blue ring. This is a cited adaptation: text inputs, textareas and select triggers focus neutral; buttons, links, tabs and other controls focus blue.
- **Error:** a 1.5px ring in Destructive at 60%, with the bordered danger-tint alert above the form for request failures.
- **Disabled:** Muted Gray fill with Subtle Ink text.
- **Select:** same trigger as an input, with a 16px Subtle Ink chevron; the list floats on the popover shadow with 3px items that fill Muted Gray on focus.

### Navigation
- **Top bar:** logo, slanted divider, then a 13px breadcrumb in Subtle Ink with chevron separators; the current page sets in Strong Ink at 500. Account menu (initials avatar + name) at the right.
- **Sidebar:** 34px rows with 10px icon gaps, 13px medium labels in Body Ink, 16px icons. Hover and active both fill gray; active adds semibold Strong Ink. Groups collapse with a rotating chevron and indent their children as plain text links, with optional 12px Subtle Ink section labels. GitHub and the collapse toggle sit at the bottom.
- **Rails:** the department filter and worker-detail section rail repeat the sidebar row (34px, 13px medium, gray selection plus semibold).
- **Tabs:** the default style is a Recessed Gray track (36px, 2px padding) whose active segment turns Base White with a Line ring and hairline shadow; the line style marks the active tab with a 2px Control Blue underline.
- **Mobile:** the sidebar becomes an off-canvas sheet, the breadcrumb hides, and rails stack above content (the worker-detail rail turns into a horizontal scroller).

### Tables
- **Container:** Base White surface with a 1px Line ring and 3px corners, flush rows, 16px inset on the first and last columns.
- **Head:** 40px, 14px semibold Strong Ink, no fill, a 1px Line below.
- **Rows:** 12px cell padding, Hairline dividers, Elevated Gray on hover. The primary cell stacks a 14px medium name (Strong Ink, Link Ink with underline on hover) over a 13px Subtle Ink line; avatars lead worker rows.
- **Footer:** pagination below the table: a 13px tabular count at the left, outline small buttons at the right.

### Dialogs, Sheets and Tooltips
- **Dialog:** Base White, 3px corners, 20px padding, popover shadow over a 30% black scrim, 16px semibold title; the footer is an Elevated Gray strip with a top Line holding right-aligned actions. Opens with a 100ms fade and 95% zoom.
- **Sheet:** a side panel on the popover shadow with a 1px edge line, sliding 40px over 200ms; 384px max on desktop.
- **Tooltip:** Strong Ink fill with Base White 12px text, 3px corners, appearing at once with no delay.

### Worker Avatar (signature)
Workers are presented as colleagues: a circular initials avatar (first glyph for CJK names, two letters otherwise) in Muted Gray with a 1px Line, plus a presence dot at the lower right ringed in the base color. The dot is labeled for assistive tech, and in the working state it pulses unless reduced motion is on. Sizes run 24, 32, 40 and 56px (profile header).

### Empty State
A centered honeycomb of seven outlined hexagons in Subtle Ink at 35%, then a 16px semibold title, an optional 14px Subtle Ink line capped at 384px, and an optional action, fading in over 200ms. The honeycomb is geometry, never illustration or brand color.

### Inline Alert
A bordered tint banner: danger tint fill, Danger Ink text, an inset 1px Destructive ring at 25%, 3px corners, 12px/16px padding. An info variant uses the info tint for read-only notices.

### Motion
Motion is brief and functional: content fades up 4px over 200ms on route entry, buttons and rows shift color over 150ms, menus and dialogs fade and zoom from 95% over 100ms, sheets slide over 200ms, and group chevrons rotate over 200ms. Working states pulse (1.5s dot sequence in chat, a ping on the presence dot). Under reduced motion, all animation and transition collapse to near zero.

### Named Rules
**The Gray Selection Rule.** Navigation selection (sidebar, rails, menu focus) is a gray fill plus semibold Strong Ink, never Control Blue and never orange.

**The Two Focus Rings Rule.** Text inputs focus with a neutral 50% Focus Ink ring; every other interactive control focuses with the 2px Control Blue ring. Focus is always visible.

**The Ring-Not-Border Rule.** Surfaces and controls are outlined with 1px rings so outlines never shift layout; borders are only for structural dividers.

**The Muted Hive Rule.** The hive appears as muted hexagon geometry in empty states, never as a brand-colored pattern, mascot or illustration.

## Do's and Don'ts

### Do:
- **Do** place exactly one primary (Control Blue) button per view, at the right end of the page header or the header strip.
- **Do** wrap every titled section in the LayerCard panel: Elevated Gray header strip (44px minimum) over a flush white body.
- **Do** render every status with StatusBadge (tint + 6px dot + translated label) or the labeled presence dot.
- **Do** use 3px corners by default, and a full radius only for avatars, dots, switches and icon chips.
- **Do** set counts, tokens, durations and timestamps in tabular numerals, and IDs, keys and versions in JetBrains Mono.
- **Do** keep field labels at 12px medium, sentence case, in Subtle Ink.
- **Do** divide cells inside a surface with Hairline lines instead of nesting outlined tiles.
- **Do** test every surface in Chinese and English, light and dark.

### Don't:
- **Don't** use rounded-md, rounded-lg or larger, arbitrary radii above 3px, or Kumo's native 8px corners.
- **Don't** use Control Blue for navigation selection, icons, headings or decoration; it means "act here".
- **Don't** use OpenBee orange anywhere outside the logo: no orange buttons, badges, washes, highlights or chart series.
- **Don't** convey state by color alone, and don't color the token-usage delta green or red.
- **Don't** put shadows on resting panels, tiles or tables; shadows are for floating layers and the control hairline only.
- **Don't** add gradients beyond the 8% white top sheen on filled buttons.
- **Don't** use uppercase, wide-tracked eyebrows or kickers above titles.
- **Don't** set text at 700 weight or outside the 12/13/14/16px ramp (titles at 20px and metric numerals at 20 to 24px excepted).
- **Don't** give text inputs the blue focus ring; they focus neutral.
- **Don't** add a page purpose line where the product has no such copy.
