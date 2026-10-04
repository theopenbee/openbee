---
version: 1
slug: "web-src-app-tsx"
primary_target: "web/src/app.tsx"
related_targets: ["web/src"]
---

# Surface brief: OpenBee Web Console

Scope: the whole authenticated console under `web/src` (shell, shared components, all 17 pages: overview, chat, departments, workers, worker detail, create worker, users, roles, sessions, session detail, scheduled tasks, env variables, settings, login, setup, not-found, no-access).

Mode: Operate.

Job: anyone in the org (one user population, permission-shaped) checks the state of the digital workforce, reviews a worker's history, assigns or configures work, and administers people, roles, and system settings. Daily, routine, in an office under ordinary daylight or screen light; light theme is the user's default, dark is supported.

Constraints: features, copy, routes, permissions, and data flow unchanged. Worker identity (avatar, presence, history) keeps its colleague standing. Status meanings (idle / working / error) unchanged, never color-only. Radius capped at `sm` (CLAUDE.md). zh/en parity.

Decided by user (2026-10-03): primary action color is Cloudflare blue; OpenBee orange retreats to the brand mark. Scope B: tokens + shell + shared components + every page re-laid in Cloudflare page patterns.

Cited adaptations (forced by constraints, not drift):
- Radius: Kumo's 8px corners are translated to `rounded-sm` (3px) per CLAUDE.md.
- Page purpose line: shown only where the page already had that copy; copy is frozen by the user.
- Focus: text inputs use Kumo's neutral focus ring (`ring-focus/50`); buttons, links, tabs and other controls use the blue `focus-visible` ring.

## Direction contract

THESIS: OpenBee reads as a Cloudflare-grade control plane for a digital workforce. It refuses the generic shadcn admin look: soft gray cards floating on white, brand color spent on every button.

OWN-WORLD: Kumo grammar. Near-white canvas ground; white base surfaces outlined by a 1px 10%-black line; elevated gray header strips over white content (LayerCard). One blue (oklch 0.5772 0.2324 260) for primary action, link, focus, selection; OpenBee orange only on the mark. Inter + Noto Sans SC at 12/13/14/16px, -0.01em tracking, semibold table heads. Tint-on-text badges. Squared corners (≤ sm).

STORY: The user sees, per page, a title with one-line purpose, the primary action top-right, and the content as layered panels or a dense table; state is legible at a glance and the next action is obvious.

FIRST VIEWPORT: Overview: 48px top bar (mark, breadcrumb, account) over a 260px white sidebar; canvas content column, page title 20px semibold left, actions right; a row of three plain stat tiles (white base, 1px line), then the token-usage LayerCard panel with its chart, quick links below; agents and system info LayerCards in a 360px side column.

FORM: user-pinned canon, Cloudflare Dashboard / Kumo (github.com/cloudflare/kumo) executed at full fidelity; concept-seed 4e3d3683 ran and was overridden by the user's pin. Code-led: no image generation in this harness.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
