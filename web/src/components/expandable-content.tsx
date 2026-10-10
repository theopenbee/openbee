import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { observeResize } from "@/lib/resize-observer"
import { cn } from "@/lib/utils"

interface ExpandableContentProps {
  children: ReactNode
  maxHeight: number
  fadeClassName?: string
  className?: string
}

export function ExpandableContent({
  children,
  maxHeight,
  fadeClassName = "h-16 from-background/95",
  className,
}: ExpandableContentProps) {
  const { t } = useTranslation()
  const innerRef = useRef<HTMLDivElement>(null)
  const [collapsed, setCollapsed] = useState(true)
  const [overflows, setOverflows] = useState(false)

  useLayoutEffect(() => {
    const el = innerRef.current
    if (el) setOverflows(el.scrollHeight > maxHeight)
  }, [children, maxHeight])

  useEffect(() => {
    const el = innerRef.current
    if (!el) return
    return observeResize(el, () => setOverflows(el.scrollHeight > maxHeight))
  }, [maxHeight])

  const clipped = collapsed && overflows

  return (
    <div className={className}>
      <div
        className={cn("overflow-hidden transition-[max-height] duration-300", clipped && "relative")}
        style={{ maxHeight: clipped ? maxHeight : undefined }}
      >
        <div ref={innerRef}>{children}</div>
        {clipped && (
          <div
            className={cn(
              "pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t to-transparent",
              fadeClassName
            )}
          />
        )}
      </div>
      {overflows && (
        <button
          type="button"
          aria-expanded={!collapsed}
          onClick={() => setCollapsed((prev) => !prev)}
          className="mt-1 py-1 text-xs font-medium text-primary/80 transition-colors hover:text-primary"
        >
          {collapsed ? t("common.showMore") : t("common.showLess")}
        </button>
      )}
    </div>
  )
}
