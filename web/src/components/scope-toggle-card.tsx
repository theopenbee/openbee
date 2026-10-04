import { useTranslation } from "react-i18next"
import { Switch } from "@/components/ui/switch"
import type { ScopeDef } from "@/lib/scopes"

interface ScopeToggleCardProps {
  scope: ScopeDef
  checked: boolean
  onToggle: (id: string, checked: boolean) => void
  disabled?: boolean
}

export function ScopeToggleCard({ scope, checked, onToggle, disabled }: ScopeToggleCardProps) {
  const { t } = useTranslation()

  // A Kumo setting row: label and description left, switch right. Rows are
  // divided by the parent surface's hairlines, never wrapped in their own card.
  return (
    <div className="flex items-center justify-between gap-4 px-4 py-3">
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-strong">{t(scope.titleKey)}</p>
        <p className="mt-0.5 text-body-sm leading-5 text-muted-foreground">{t(scope.descriptionKey)}</p>
      </div>
      <Switch
        checked={checked}
        onCheckedChange={(val) => onToggle(scope.id, val)}
        disabled={disabled}
        aria-label={t(scope.titleKey)}
        className="shrink-0"
      />
    </div>
  )
}
