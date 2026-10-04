// Shared class strings for recurring micro-typography, so the same visual role
// stays identical everywhere instead of drifting across components.

// Field label: the small caption above a metric, a key/value pair, or a list
// group. Kumo keeps these sentence-case at 12px medium in the subtle tone; no
// uppercase or wide tracking, which also keeps CJK labels evenly set.
export const FIELD_LABEL = "text-xs font-medium text-muted-foreground"

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
  "text-sm leading-6 text-foreground [&_[data-streamdown=code-block]]:my-3 [&_[data-streamdown=code-block]]:gap-0 [&_[data-streamdown=code-block]]:rounded-sm [&_[data-streamdown=code-block]]:border-0 [&_[data-streamdown=code-block]]:bg-recessed [&_[data-streamdown=code-block]]:p-0 [&_[data-streamdown=code-block]]:ring-1 [&_[data-streamdown=code-block]]:ring-border [&_[data-streamdown=code-block-header]]:border-b [&_[data-streamdown=code-block-header]]:border-border [&_[data-streamdown=code-block-header]]:px-3 [&_[data-streamdown=code-block-header]+div]:-mt-8 [&_[data-streamdown=code-block-header]+div]:pr-1 [&_[data-streamdown=code-block-actions]]:rounded-sm [&_[data-streamdown=code-block-actions]]:border-0 [&_[data-streamdown=code-block-actions]]:bg-transparent! [&_[data-streamdown=code-block-actions]]:[backdrop-filter:none]! [&_[data-streamdown=code-block-body]]:rounded-none [&_[data-streamdown=code-block-body]]:border-0 [&_[data-streamdown=code-block-body]]:bg-transparent [&_[data-streamdown=code-block-body]]:px-3 [&_[data-streamdown=code-block-body]]:py-2.5 [&_[data-streamdown=code-block-body]]:font-mono [&_[data-streamdown=code-block-body]]:text-[13px] [&_[data-streamdown=code-block-body]_pre]:bg-transparent!"
