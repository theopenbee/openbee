// Shared class strings for recurring micro-typography, so the same visual role
// stays identical everywhere instead of drifting across components.

// Field label: the small caption above a metric, a key/value pair, or a list
// group. Kumo keeps these sentence-case at 12px medium in the subtle tone; no
// uppercase or wide tracking, which also keeps CJK labels evenly set.
export const FIELD_LABEL = "text-xs font-medium text-muted-foreground"

// Page title: the one h1 per page (page headers, the worker-detail and workers
// header bars, the login/setup cards). 20px semibold with slight negative
// tracking, in the strong tone.
export const PAGE_TITLE = "text-xl leading-7 font-semibold tracking-[-0.015em] text-strong"

// Header bar: the full-width strip atop a split-pane page (workers, worker
// detail) on the base surface, at least 72px tall so it lines up with the 72px
// heading of the left rail beside it; wraps its actions under the title when narrow.
export const HEADER_BAR =
  "flex min-h-18 shrink-0 flex-wrap items-center justify-between gap-x-6 gap-y-3 border-b border-border bg-background px-6 py-3"

// Surface: the white working panel drawn with a single 1px Line ring (tables,
// lists, detail sections, empty states). Clips its content to the corners so
// row hover fills and dividers stay inside the ring.
export const SURFACE = "overflow-hidden rounded-sm bg-card ring-1 ring-border"

// Rail items (department filter, worker-detail section nav) mirror the main
// sidebar: 34px rows, 13px medium labels, a gray accent fill on hover, and the
// same gray fill plus semibold for the selection (never the action blue or the
// brand orange).
export const RAIL_ITEM =
  "flex h-8.5 w-full items-center gap-2.5 rounded-sm px-3 text-left text-body-sm font-medium whitespace-nowrap transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
export const RAIL_ITEM_IDLE = "text-foreground hover:bg-accent hover:text-strong"
export const RAIL_ITEM_SELECTED = "bg-accent font-semibold text-strong"

// Destructive inline alert: the bordered, tinted error banner shown for form
// and request failures. Keeps the same radius, border, tint, and text colour
// everywhere; call sites layer on their own margin/layout (and override padding
// via cn/twMerge) for context.
export const ALERT_DESTRUCTIVE =
  "rounded-sm bg-danger-tint px-4 py-3 text-sm text-destructive-foreground ring-1 ring-destructive/25 ring-inset"

// Streamdown fenced code: one recessed block (mono 13px) with a hairline under
// the language strip, instead of the library's nested bordered boxes. Shared by
// the chat transcript and the session log viewer.
export const STREAMDOWN_BLOCKS =
  "text-sm leading-6 text-foreground [&_[data-streamdown=code-block]]:my-3 [&_[data-streamdown=code-block]]:gap-0 [&_[data-streamdown=code-block]]:rounded-sm [&_[data-streamdown=code-block]]:border-0 [&_[data-streamdown=code-block]]:bg-recessed [&_[data-streamdown=code-block]]:p-0 [&_[data-streamdown=code-block]]:ring-1 [&_[data-streamdown=code-block]]:ring-border [&_[data-streamdown=code-block-header]]:border-b [&_[data-streamdown=code-block-header]]:border-border [&_[data-streamdown=code-block-header]]:px-3 [&_[data-streamdown=code-block-header]+div]:-mt-8 [&_[data-streamdown=code-block-header]+div]:pr-1 [&_[data-streamdown=code-block-actions]]:rounded-sm [&_[data-streamdown=code-block-actions]]:border-0 [&_[data-streamdown=code-block-actions]]:bg-transparent! [&_[data-streamdown=code-block-actions]]:[backdrop-filter:none]! [&_[data-streamdown=code-block-body]]:rounded-none [&_[data-streamdown=code-block-body]]:border-0 [&_[data-streamdown=code-block-body]]:bg-transparent [&_[data-streamdown=code-block-body]]:px-3 [&_[data-streamdown=code-block-body]]:py-2.5 [&_[data-streamdown=code-block-body]]:font-mono [&_[data-streamdown=code-block-body]]:text-body-sm [&_[data-streamdown=code-block-body]_pre]:bg-transparent!"
