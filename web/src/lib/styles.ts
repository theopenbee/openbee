// Shared class strings for recurring micro-typography, so the same visual role
// stays identical everywhere instead of drifting across components.

// Eyebrow / caption label: the small uppercase metadata caption above metrics,
// list rows, and section subtitles. Tracking is kept tight (0.05em) on purpose:
// uppercase + wide tracking is a Latin idiom, and heavy letter-spacing spreads
// CJK glyphs apart unnaturally, so a restrained value reads well in both scripts.
export const EYEBROW_LABEL =
  "text-[11px] font-medium uppercase tracking-[0.05em] text-muted-foreground"
