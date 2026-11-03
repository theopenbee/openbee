import { useId } from "react"
import { useTranslation } from "react-i18next"
import { Switch } from "@/components/arc/switch/switch"
import type { ScopeDef } from "@/lib/scopes"

interface ScopeToggleCardProps {
  scope: ScopeDef
  checked: boolean
  onToggle: (id: string, checked: boolean) => void
  disabled?: boolean
}

export function ScopeToggleCard({ scope, checked, onToggle, disabled }: ScopeToggleCardProps) {
  const { t } = useTranslation()
  const id = useId()

  return (
    <div className="flex min-h-16 items-center justify-between gap-4 px-4 py-4 sm:px-6">
      <div className="grid min-w-0 gap-0.5">
        <span id={`${id}-title`} className="text-sm font-medium text-foreground">
          {t(scope.titleKey)}
        </span>
        <span id={`${id}-hint`} className="text-sm text-secondary">
          {t(scope.descriptionKey)}
        </span>
      </div>
      <Switch
        checked={checked}
        onCheckedChange={(val) => onToggle(scope.id, val)}
        disabled={disabled}
        aria-labelledby={`${id}-title`}
        aria-describedby={`${id}-hint`}
        className="shrink-0"
      />
    </div>
  )
}
