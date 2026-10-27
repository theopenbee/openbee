import { useMemo } from "react"
import { useTranslation } from "react-i18next"
import { useEnvList, useDepartmentEnvs } from "@/hooks/use-envs"
import { Panel } from "@/components/panel"
import { cn } from "@/lib/utils"
import type { EnvScope } from "@/lib/types"

type EnvSource = Exclude<EnvScope, "bee">

const SOURCE_LABEL: Record<EnvSource, string> = {
  global: "envConfig.sourceGlobal",
  department: "envConfig.sourceDepartment",
  worker: "envConfig.sourceWorker",
}

export function WorkerEffectiveEnv({ workerId, departmentIds }: { workerId: string; departmentIds: string[] }) {
  const { t } = useTranslation()
  const { data: globalEnvs = [], isLoading: globalLoading } = useEnvList("global")
  const { data: workerEnvs = [], isLoading: workerLoading } = useEnvList("worker", workerId)
  const deptEnvsList = useDepartmentEnvs(departmentIds)

  const rows = useMemo(() => {
    const merged = new Map<string, { masked: string; source: EnvSource }>()
    for (const env of globalEnvs) merged.set(env.key, { masked: env.masked, source: "global" })
    for (const deptEnvs of deptEnvsList) {
      for (const env of deptEnvs) merged.set(env.key, { masked: env.masked, source: "department" })
    }
    for (const env of workerEnvs) merged.set(env.key, { masked: env.masked, source: "worker" })
    return Array.from(merged.entries()).sort(([a], [b]) => a.localeCompare(b))
  }, [globalEnvs, workerEnvs, deptEnvsList])

  const loading = globalLoading || workerLoading

  return (
    <Panel
      title={t("envConfig.effectiveTitle")}
      description={t("workerDetail.effectiveEnvDescription")}
      flush
    >
      {loading ? (
        <div className="grid gap-3 px-6 pb-4" aria-busy="true">
          {[0, 1, 2].map((i) => (
            <div key={i} className="skeleton h-5 w-full" />
          ))}
        </div>
      ) : rows.length === 0 ? (
        <p className="px-6 pb-4 text-sm text-secondary">{t("envConfig.noEffective")}</p>
      ) : (
        <ul>
          {rows.map(([key, { masked, source }]) => (
            <li
              key={key}
              className="grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-4 gap-y-1 border-t border-border-subtle px-6 py-3"
            >
              <span className="font-mono text-sm break-all text-foreground">{key}</span>
              <span
                className={cn(
                  "text-xs whitespace-nowrap",
                  source === "worker" ? "font-medium text-foreground" : "text-secondary",
                )}
              >
                <span className="sr-only">{t("envConfig.source")}: </span>
                {t(SOURCE_LABEL[source])}
              </span>
              <span className="col-span-2 font-mono text-xs break-all text-secondary">
                <span className="sr-only">{t("envConfig.masked")}: </span>
                {masked}
              </span>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  )
}
