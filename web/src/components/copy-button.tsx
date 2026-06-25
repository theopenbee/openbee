import { useState } from "react"
import { Check, Copy } from "lucide-react"
import { useTranslation } from "react-i18next"
import { cn } from "@/lib/utils"

// Inline copy affordance: copies `value` to the clipboard and shows a brief
// confirmation. Keyboard accessible (focus-visible ring) and labeled for
// screen readers, so it satisfies the WCAG AA bar the console targets.
export function CopyButton({
  value,
  label,
  className,
}: {
  value: string
  label?: string
  className?: string
}) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const title = copied ? t("common.copied") : label ?? t("common.copy")

  return (
    <button
      type="button"
      onClick={() => {
        navigator.clipboard.writeText(value)
        setCopied(true)
        setTimeout(() => setCopied(false), 2000)
      }}
      aria-label={title}
      title={title}
      className={cn(
        "shrink-0 rounded-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
        className,
      )}
    >
      {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
    </button>
  )
}
