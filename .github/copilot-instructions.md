# OpenBee Design System

## Design Context

### Users
Developers, DevOps engineers, and technical teams running OpenBee as a self-hosted open-source tool. They open the console daily to monitor and manage autonomous AI workers — checking execution status, reviewing task history, configuring workers. The context is routine operational use, not emergency incident response. Users are technical and self-sufficient; they expect the interface to respect their intelligence and get out of the way.

### Brand Personality
**Serious. Precise. Efficient.**

The hive metaphor (workers, sessions, departments) grounds the product, but it should never become whimsical. The tone is that of a professional instrument — confident, calm, and direct. No fanfare, no personality theater. Every interaction should feel intentional.

### Aesthetic Direction
**References:** Linear and Vercel — clean, fast, opinionated minimalism. Linear's information density and keyboard-first flow; Vercel's crisp developer-facing clarity and restrained use of color.

**Anti-references:**
- Overly playful — no consumer app energy, rounded cartoon shapes, or emoji-heavy UI
- Enterprise-drab — no heavy borders, dated grays, or bloated nav chrome

**Visual tone:** Achromatic neutrals carry the layout. Color is reserved exclusively for meaning — green (idle/complete), purple (working), red (error). No decorative color. Dark mode is primary; light mode is equally supported with the same level of polish.

**Typography:** Oxanium (geometric, precise) for all brand and UI text. JetBrains Mono for code, logs, and technical output. Both choices reinforce the serious-but-modern register.

### Design Principles

1. **Clarity over decoration** — Every element must earn its place. If it doesn't communicate something, remove it.

2. **Color carries meaning, never mood** — The palette is achromatic by default. The three status colors (green, purple, red) are the only chromatic elements. Never use color for emphasis or aesthetic interest.

3. **Density with breathing room** — Inspired by Linear: pack meaningful information efficiently, but maintain generous internal spacing within components. Avoid both sparse emptiness and cramped clutter.

4. **Precision in every detail** — Alignment, spacing, and sizing must be exact. Rounded corners are intentional (10px base radius), not decorative. The interface should feel machine-accurate.

5. **Developer-grade polish** — The user is technical. Interactions should be fast, states should be obvious, and feedback should be immediate. No loading spinners where skeletons work; no modals where inline works.
