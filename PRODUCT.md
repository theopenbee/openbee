# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

One user population, like WeChat. OpenBee draws no mandatory distinction between kinds of people: anyone in the organization who works with the digital workforce is simply a user. Some direct workers from a connected IM platform (Lark, DingTalk, WeCom, WeChat, Telegram, Linear); some open the Web console to see who is working, review what a worker did, assign work, or organize workers into departments. Many do both.

What a person can see or do is shaped only by the roles and permissions an administrator grants, never by a built-in user type. The product must not presume a particular role, job, or technical background.

Installing and running the self-hosted server (`openbee config`, `openbee server`) is a deployment step done by whoever operates the instance. It does not create a separate class of console user.

## Product Purpose

OpenBee is an enterprise collaboration platform for a digital workforce: the workspace where people and AI workers get work done together. Its core thesis is that **an AI worker is a digital employee, equal in standing to a human colleague.** Workers carry identity, presence, a department, and a history; they are never reduced to rows in a job queue. OpenBee keeps them on duty 7×24.

Success looks like this: a user messages a worker from the IM tool they already use and the work gets done; or they open the console and, at a glance, understand the state of the workforce they can access, then take the next action with no friction and no second-guessing.

## Positioning

Digital employees that work where the organization already works. Workers are reachable inside the team's existing IM platforms, coordinated by the Bee, and backed by real agent engines (Claude Code, Codex, Pi) running on infrastructure the organization controls: one self-hosted, open-source (MIT) binary. The console is the office those employees work in, not a job scheduler.

## Operating Context

- **Conversations start in IM.** Messages from connected platforms are ingested, the Bee coordinator routes them to workers, workers run sessions on their agent engine, and results return to the chat.
- **The Web console** (default `http://localhost:8080`) is organized as: Overview (departments, workers, scheduled tasks, token usage and trend, supported agents, system info, quick access); Chat (direct conversation with workers); Digital Employees (Departments, Workers, worker detail); Directory (Users, Roles); Sessions (session detail, executions, logs); Scheduled tasks; System (Env Variables, Settings). Plus login, first-run setup, and a no-access landing.
- Every surface ships in Chinese and English, in light and dark themes.

## Capabilities and Constraints

- **Terminology:** Worker / Digital Employee (数字员工), Bee (the coordinator), Department, Session, Execution, Scheduled Task, Engine (the agent runtime behind a worker), Env Variables, User, Role, Permission.
- **Engines:** Claude Code, Codex, Pi.
- **IM platforms:** Lark (Feishu), DingTalk, WeCom, WeChat, Telegram, Linear, plus local chat in the console.
- **Access control:** every route is gated by a `resource:action` permission (contacts, tasks, chat, sessions, dashboard, env, system config, users, roles). Roles bundle permissions; a wildcard grants everything.
- **Deployment:** a self-hosted Go binary with the web console embedded and SQLite storage. Distributed via npm (`@theopenbee/cli`), Homebrew, Scoop, an install script, and GitHub Releases, for Linux, macOS, and Windows on amd64 and arm64.

## Brand Commitments

- **Name and line:** OpenBee. "Build smarter AI teams."
- **Logos:** `web/public/logo-full.svg`, `web/public/logo-mark.svg`, `docs/logo-full.svg`.
- **Voice:** Efficient. Precise. Concise. A calm, competent enterprise instrument: confident and direct, never chatty. The hive metaphor (Bee, workers, departments) grounds the product but never turns whimsical. Warmth toward digital employees comes from respect and clarity, not cuteness or personality theater.
- **Primary reference: Cloudflare (binding).** Both the Cloudflare Dashboard (dash.cloudflare.com) as the product and interaction reference (information architecture, navigation, information density, tables, configuration forms, copy tone) and Cloudflare's brand visual language. Cloudflare replaces the earlier Slack-derived reference. It governs how the product is organized and how it looks; it does not change what the product is. The digital-employee positioning stands.
- **Anti-references:** overly playful consumer apps (cartoon shapes, emoji-heavy UI, mascots); enterprise drab (heavy borders, dated grays, bloated navigation chrome); verbose, redundant copy (restated headings, explanatory paragraphs where a label suffices, text that repeats what the UI already shows).
- **Corner radius is capped at `sm`.** Strict project rule; see `CLAUDE.md`.

## Evidence on Hand

- Console screenshot: `docs/openbee-dashboard.png`.
- Documentation site: docs.theopenbee.com. READMEs in English and Chinese (`README.md`, `README.zh.md`), `CHANGELOG.md`.
- Public distribution: npm `@theopenbee/cli`, GitHub `theopenbee/openbee`, MIT license, contributor CLA (`CLA.md`).
- **Absent:** no customer testimonials, customer logos, case studies, usage figures, benchmarks, or pricing. Future work must not invent them.

## Product Principles

1. **Do the hard work so the user doesn't have to.** The product shoulders the complexity; the user's task feels effortless even when the system underneath is not.
2. **Treat digital workers as colleagues.** Workers have identity, presence, and history with the same standing as human staff. Present them like people in an org, never like anonymous jobs.
3. **One kind of user.** Never design for a presumed user type. Differences in what people see come only from roles and permissions.
4. **Clarity over cleverness.** States are obvious, patterns are predictable, and the next action is never in doubt.
5. **Say only what matters.** Cut redundant copy. Let the interface, not paragraphs, do the explaining.

## Accessibility & Inclusion

- Target **WCAG 2.1 AA** for contrast and interaction.
- Respect `prefers-reduced-motion`: reduce or remove non-essential motion.
- **Never rely on color alone** for worker or task state; pair every status with an icon or a text label.
- Keep focus indicators visible and high-contrast for keyboard navigation.
- Chinese and English are equal: every surface must hold up in both, including CJK text, longer English strings, and line breaking.
