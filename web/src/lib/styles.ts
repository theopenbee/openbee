// Shared class strings for recurring micro-typography, so the same visual role
// stays identical everywhere instead of drifting across components.

// Eyebrow / caption label: the small uppercase metadata caption above metrics,
// list rows, and section subtitles. Tracking is kept tight (0.05em) on purpose:
// uppercase + wide tracking is a Latin idiom, and heavy letter-spacing spreads
// CJK glyphs apart unnaturally, so a restrained value reads well in both scripts.
export const EYEBROW_LABEL =
  "text-[11px] font-medium uppercase tracking-[0.05em] text-muted-foreground"

// Destructive inline alert: the bordered, tinted error banner shown for form
// and request failures. Keeps the same radius, border, tint, and text colour
// everywhere; call sites layer on their own margin/layout (and override padding
// via cn/twMerge) for context.
export const ALERT_DESTRUCTIVE =
  "rounded-sm border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive"

export const TOOLTIP_BUBBLE =
  "z-50 w-fit max-w-xs origin-(--transform-origin) rounded-sm bg-foreground px-3 py-1.5 text-xs text-background data-[side=bottom]:slide-in-from-top-2 data-[side=inline-end]:slide-in-from-left-2 data-[side=inline-start]:slide-in-from-right-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95"
