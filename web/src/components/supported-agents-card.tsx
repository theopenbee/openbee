import { useTranslation } from "react-i18next"
import { Panel } from "@/components/panel"
import { useEnabledEngines } from "@/hooks/use-config"
import { formatEngineLabel } from "@/lib/format"
import { ENGINES } from "@/lib/types"

// The three engines OpenBee can drive. Each is presented as a named capability
// with its CLI key, not a logo grid, so the lineup reads as a deliberate
// typographic statement and marks which engines this deployment has enabled.
export function SupportedAgentsCard() {
  const { t } = useTranslation()
  const enabled = useEnabledEngines()

  return (
    <Panel title={t("dashboard.supportedAgents")} ariaLabel={t("dashboard.supportedAgents")} flush>
      <ul className="divide-y divide-border/70">
        {ENGINES.map((engine) => {
          const isOn = enabled.includes(engine)
          return (
            <li key={engine} className="flex items-baseline justify-between gap-4 px-5 py-3.5">
              <div className="min-w-0">
                <p className="text-base font-semibold tracking-tight truncate">
                  {formatEngineLabel(engine, t)}
                </p>
                <p className="mt-0.5 font-mono text-xs text-muted-foreground">{engine}</p>
              </div>
              <span
                className={`flex shrink-0 items-center gap-1.5 text-[11px] font-medium uppercase tracking-[0.05em] ${
                  isOn ? "text-foreground" : "text-muted-foreground/70"
                }`}
              >
                <span
                  className={`size-1.5 rounded-full ${isOn ? "bg-brand" : "bg-muted-foreground/40"}`}
                  aria-hidden
                />
                {isOn ? t("dashboard.agentEnabled") : t("dashboard.agentDisabled")}
              </span>
            </li>
          )
        })}
      </ul>
    </Panel>
  )
}
