# Product

## Register

product

## Users

Individual developers, teams, and enterprises who run AI agents (Claude Code, Codex, Pi) on infrastructure they control and want to reach them from wherever they are.

- **Individual developers** run OpenBee on their own machine or server. They hand a task to an agent from their phone between meetings, check on it from a browser, and let scheduled tasks run while they are away.
- **Teams** share a set of agents inside the IM tools they already use (Lark, DingTalk, WeCom, WeChat, Telegram, Linear). Anyone on the team can message an agent by name and see the result in the same thread.
- **Enterprises** run many agents on one self-hosted instance, organize them into departments, and control who can see or do what through users, roles, and permissions.

The mental model is **your agents, on call**: agents keep running on your own infrastructure, and you reach them from any chat app or browser, at any time. Most interactions start in IM; the Web console is where users see the state of their agents, review what each one did, and configure them. Users are technical and self-sufficient; they expect the interface to respect their intelligence, do the heavy lifting for them, and get out of the way.

## Product Purpose

OpenBee lets people use their AI agents anywhere, anytime. Agents run where the code, data, and credentials already live; OpenBee connects them to the channels people already use and keeps them available around the clock.

Success looks like a user sending a message from any device and getting the work done without opening a laptop; or opening the console, from a phone or a desktop, and understanding at a glance which agents are available, what each one is doing, and what it did, then taking the next action with no friction and no second-guessing.

## Brand Personality

**Efficient. Precise. Concise.**

The voice is that of a calm, competent instrument: confident and direct, never chatty. The bee name grounds the brand but must never turn whimsical. Warmth is expressed through reliability and clarity, not cuteness or personality theater. Every interaction is intentional; every word earns its place.

## Anti-references

This product should explicitly NOT look or feel like:

- **Overly playful consumer apps** — no rounded cartoon shapes, emoji-heavy UI, mascots, or consumer-app energy.
- **Enterprise drab** — no heavy borders, dated grays, or bloated, over-stuffed navigation chrome.
- **Desk-bound admin back offices** — no flows that only work on a wide desktop screen or assume the user sits at the console all day.
- **Verbose, redundant copy** — no restated headings, no explanatory paragraphs where a label suffices, no text that repeats what the UI already shows. If the interface can communicate it, the words come out.

## Design Principles

1. **Do the hard work so the user doesn't have to.** Shoulder the complexity inside the product. Reaching an agent should feel effortless even when the system underneath is not.
2. **Reachable from anywhere.** IM is a first-class entry point, not an add-on. Core console flows (chat, checking status, stopping a task) must work on a phone-width screen.
3. **Make every agent legible.** Each agent carries a name, an identity, presence, and history, so it is recognizable at a glance and addressable by name from any chat.
4. **Clarity over cleverness.** Don't make the user think. States are obvious, patterns are predictable, the next action is never in doubt.
5. **Sweat the craft.** Alignment, spacing, motion, and micro-interactions are done with care. Quality is felt before it is noticed.
6. **Say only what matters.** Be concise. Cut redundant copy. Let the interface, not paragraphs, do the explaining.

## Accessibility & Inclusion

- Target **WCAG 2.1 AA** for color contrast and interaction.
- Respect `prefers-reduced-motion`: disable or reduce non-essential motion when requested.
- **Never rely on color alone** to convey agent state. The brand orange and the green/purple/red status palette are warm, mid-luminance hues that fail contrast as text and are ambiguous for color-blind users, so every status is paired with an icon or a text label.
- Maintain visible, high-contrast focus indicators for keyboard navigation.
