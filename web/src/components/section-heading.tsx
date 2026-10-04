export function SectionHeading({ text, badge }: { text: string; badge?: number }) {
  return (
    <div className="flex items-center gap-2">
      <p className="text-sm leading-none font-semibold text-strong">{text}</p>
      {badge !== undefined && badge > 0 && (
        <span className="rounded-sm bg-muted px-1.5 py-0.5 text-[11px] leading-none font-medium text-foreground tabular-nums">
          {badge}
        </span>
      )}
    </div>
  )
}
