import { useMemo } from "react"
import { useTranslation } from "react-i18next"
import { DetailSection } from "@/components/detail-primitives"
import { EnvConfigPanel } from "@/components/env-config-panel"
import { WorkerEffectiveEnv } from "@/components/worker-effective-env"
import type { Worker } from "@/lib/types"

export function WorkerEnvSection({ worker }: { worker: Worker }) {
  const { t } = useTranslation()
  const departmentIds = useMemo(() => worker.departments?.map((d) => d.id).sort() ?? [], [worker.departments])

  return (
    <div className="grid gap-6">
      <DetailSection className="p-6">
        <EnvConfigPanel scope="worker" scopeId={worker.id} title={t("envConfig.workerTitle")} inset />
      </DetailSection>
      <WorkerEffectiveEnv workerId={worker.id} departmentIds={departmentIds} />
    </div>
  )
}
