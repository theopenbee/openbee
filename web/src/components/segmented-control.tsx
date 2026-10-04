import type { ReactNode } from "react"
import { cn } from "@/lib/utils"

export type SegmentedOption<V extends string | number> = {
  value: V
  label: ReactNode
  /** Accessible name when the visible label is abbreviated (e.g. "7d"). */
  ariaLabel?: string
}

/**
 * Kumo segmented control: a recessed track holding pressed/unpressed buttons,
 * the selected one lifted onto the base surface. Used for single-choice view
 * filters (log entry kind, chart day range); scrolls sideways when the options
 * outgrow a narrow container.
 */
export function SegmentedControl<V extends string | number>({
  options,
  value,
  onChange,
  ariaLabel,
  className,
}: {
  options: ReadonlyArray<SegmentedOption<V>>
  value: V
  onChange: (value: V) => void
  ariaLabel?: string
  className?: string
}) {
  return (
    <div
      role="group"
      aria-label={ariaLabel}
      className={cn(
        "inline-flex h-8 max-w-full shrink-0 items-center overflow-x-auto rounded-sm bg-recessed p-0.5",
        className
      )}
    >
      {options.map((option) => {
        const selected = option.value === value
        return (
          <button
            key={option.value}
            type="button"
            aria-pressed={selected}
            aria-label={option.ariaLabel}
            onClick={() => onChange(option.value)}
            className={cn(
              "inline-flex h-7 shrink-0 items-center gap-1.5 rounded-sm px-2.5 text-body-sm font-medium whitespace-nowrap tabular-nums transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring",
              selected
                ? "bg-background text-strong shadow-xs ring-1 ring-border"
                : "text-muted-foreground hover:text-foreground"
            )}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}
