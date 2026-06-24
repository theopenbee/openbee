import { useTranslation } from "react-i18next"
import { Switch } from "@/components/ui/switch"
import { cn } from "@/lib/utils"
import type { ScopeDef } from "@/lib/scopes"

interface ScopeToggleCardProps {
  scope: ScopeDef
  checked: boolean
  onToggle: (id: string, checked: boolean) => void
  disabled?: boolean
}

export function ScopeToggleCard({ scope, checked, onToggle, disabled }: ScopeToggleCardProps) {
  const { t } = useTranslation()

  return (
    <div
      className={cn(
        "flex items-start justify-between gap-4 rounded-sm border border-border/70 bg-card px-4 py-3.5 transition-colors",
        checked && "border-primary/30 bg-primary/5"
      )}
    >
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-foreground">{t(scope.titleKey)}</p>
        <p className="mt-0.5 text-xs leading-5 text-muted-foreground">{t(scope.descriptionKey)}</p>
      </div>
      <Switch
        checked={checked}
        onCheckedChange={(val) => onToggle(scope.id, val)}
        disabled={disabled}
        aria-label={t(scope.titleKey)}
        className="mt-0.5 shrink-0"
      />
    </div>
  )
}
