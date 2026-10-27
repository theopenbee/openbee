import { useTranslation } from "react-i18next"
import { Badge } from "@/components/arc/badge/badge"
import { Panel } from "@/components/panel"
import { EngineIcon } from "@/components/agent-icons/engine-icon"
import { useEnabledEngines } from "@/hooks/use-config"
import { formatEngineLabel } from "@/lib/format"
import { ENGINES } from "@/lib/types"
import { cn } from "@/lib/utils"

export function SupportedAgentsCard({ className }: { className?: string }) {
  const { t } = useTranslation()
  const enabled = useEnabledEngines()

  return (
    <Panel
      title={t("dashboard.supportedAgents")}
      ariaLabel={t("dashboard.supportedAgents")}
      className={className}
      flush
    >
      <ul className="border-t border-border-subtle">
        {ENGINES.map((engine) => {
          const isOn = enabled.includes(engine)
          return (
            <li
              key={engine}
              className="flex min-h-18 items-center gap-4 border-b border-border-subtle px-6 py-3 last:border-b-0"
            >
              <EngineIcon
                engine={engine}
                className={cn("size-5 shrink-0", isOn ? "text-foreground" : "text-muted")}
              />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm text-foreground">{formatEngineLabel(engine, t)}</p>
                <p className="truncate font-mono text-xs text-secondary">{engine}</p>
              </div>
              <Badge
                tone={isOn ? "success" : "neutral"}
                size="sm"
                icon={<span className={cn("block size-1.5 rounded-full", isOn ? "bg-success" : "bg-border-strong")} />}
              >
                {isOn ? t("dashboard.agentEnabled") : t("dashboard.agentDisabled")}
              </Badge>
            </li>
          )
        })}
      </ul>
    </Panel>
  )
}
