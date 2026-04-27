export function SectionHeading({ text, badge }: { text: string; badge?: number }) {
  return (
    <div className="flex items-center gap-2">
      <p className="text-sm font-medium leading-none">{text}</p>
      {badge !== undefined && badge > 0 && (
        <span className="rounded-sm bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium leading-none text-primary tabular-nums">
          {badge}
        </span>
      )}
    </div>
  )
}
