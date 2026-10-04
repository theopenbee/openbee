import { useTranslation } from "react-i18next"
import { Panel } from "@/components/panel"
import { EngineIcon } from "@/components/agent-icons/engine-icon"
import { Badge } from "@/components/ui/badge"
import { useEnabledEngines } from "@/hooks/use-config"
import { formatEngineLabel } from "@/lib/format"
import { ENGINES } from "@/lib/types"
import { cn } from "@/lib/utils"

// The engines OpenBee can drive, as a divided list: engine mark, display name
// and CLI key, and an enabled/off badge that carries the state in words.
export function SupportedAgentsCard() {
  const { t } = useTranslation()
  const enabled = useEnabledEngines()

  return (
    <Panel title={t("dashboard.supportedAgents")} ariaLabel={t("dashboard.supportedAgents")} flush>
      <ul className="divide-y divide-hairline">
        {ENGINES.map((engine) => {
          const isOn = enabled.includes(engine)
          const label = formatEngineLabel(engine, t)
          return (
            <li key={engine} className="flex items-center justify-between gap-3 px-4 py-3">
              <div className="flex min-w-0 items-center gap-3">
                <EngineIcon
                  engine={engine}
                  title={label}
                  className={cn("size-5 shrink-0", isOn ? "text-foreground" : "text-muted-foreground/60")}
                />
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-strong">{label}</p>
                  <p className="truncate font-mono text-xs text-muted-foreground">{engine}</p>
                </div>
              </div>
              <Badge variant={isOn ? "success" : "secondary"}>
                {isOn ? t("dashboard.agentEnabled") : t("dashboard.agentDisabled")}
              </Badge>
            </li>
          )
        })}
      </ul>
    </Panel>
  )
}
