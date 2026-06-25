# Product

## Register

product

## Users

Developers, DevOps engineers, and technical operators running OpenBee as a self-hosted platform. They open the console daily to direct a team of AI workers, the platform's "digital employees." The context is routine operational management, not emergency incident response: checking who is working, reviewing what each worker did, configuring and assigning new work, organizing workers into departments.

The mental model is an **enterprise office platform for a digital workforce**. The operator relates to AI workers the way a manager relates to staff in a tool like Slack, Lark, or Workday: each worker has an identity, a presence, and a history. Users are technical and self-sufficient; they expect the interface to respect their intelligence, do the heavy lifting for them, and get out of the way.

## Product Purpose

OpenBee is the workspace where humans manage a workforce of AI workers. Its core thesis is that **an AI worker is a digital employee, equivalent in standing to a human colleague.** The interface must therefore carry the interaction conventions of a serious enterprise office platform, with workers afforded identity, presence, and dignity, not reduced to rows in a job queue.

Success looks like an operator opening the console and, at a glance, understanding the state of their entire digital workforce, then taking the next action with no friction and no second-guessing.

## Brand Personality

**Efficient. Precise. Concise.**

The voice is that of a calm, competent enterprise instrument: confident and direct, never chatty. The hive metaphor (workers, departments, sessions) grounds the product but must never turn whimsical. There is warmth in how the product treats its digital employees as real colleagues, but that warmth is expressed through respect and clarity, not cuteness or personality theater. Every interaction is intentional; every word earns its place.

## Anti-references

This product should explicitly NOT look or feel like:

- **Overly playful consumer apps** — no rounded cartoon shapes, emoji-heavy UI, mascots, or consumer-app energy.
- **Enterprise drab** — no heavy borders, dated grays, or bloated, over-stuffed navigation chrome.
- **Verbose, redundant copy** — no restated headings, no explanatory paragraphs where a label suffices, no text that repeats what the UI already shows. If the interface can communicate it, the words come out.

## Design Principles

Adapted from Slack's design ethos and applied to a digital-workforce platform:

1. **Do the hard work so the operator doesn't have to.** Shoulder the complexity inside the product. The operator's task should feel effortless even when the system underneath is not.
2. **Treat digital workers as colleagues.** AI workers carry identity, presence, and history with the same standing as human staff. Afford them like people in an org, never like anonymous tasks in a queue.
3. **Clarity over cleverness.** Don't make the operator think. States are obvious, patterns are predictable, the next action is never in doubt.
4. **Sweat the craft.** Alignment, spacing, motion, and micro-interactions are done with care. Quality is felt before it is noticed.
5. **Say only what matters.** Be concise. Cut redundant copy. Let the interface, not paragraphs, do the explaining.

## Accessibility & Inclusion

- Target **WCAG 2.1 AA** for color contrast and interaction.
- Respect `prefers-reduced-motion`: disable or reduce non-essential motion when requested.
- **Never rely on color alone** to convey worker state. The brand orange and the green/purple/red status palette are warm, mid-luminance hues that fail contrast as text and are ambiguous for color-blind users, so every status is paired with an icon or a text label.
- Maintain visible, high-contrast focus indicators for keyboard navigation.
