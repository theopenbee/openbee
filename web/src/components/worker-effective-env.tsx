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

const TH = "px-6 py-3 text-left text-xs font-normal text-secondary"
const TD = "px-6 py-3"

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
        <div className="overflow-x-auto overscroll-x-contain">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-t border-border-subtle">
                <th scope="col" className={TH}>{t("envConfig.key")}</th>
                <th scope="col" className={TH}>{t("envConfig.masked")}</th>
                <th scope="col" className={TH}>{t("envConfig.source")}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(([key, { masked, source }]) => (
                <tr key={key} className="border-t border-border-subtle">
                  <td className={cn(TD, "font-mono break-all text-foreground")}>{key}</td>
                  <td className={cn(TD, "font-mono break-all text-secondary")}>{masked}</td>
                  <td className={cn(TD, "whitespace-nowrap", source === "worker" ? "font-medium text-foreground" : "text-secondary")}>
                    {t(SOURCE_LABEL[source])}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Panel>
  )
}
