import { useTranslation } from "react-i18next"
import { CalendarIcon, History } from "lucide-react"
import { DetailOverviewStat } from "@/components/detail-primitives"
import { formatRelative, formatTimestamp } from "@/lib/format"
import type { Worker } from "@/lib/types"

export function WorkerOverview({ worker }: { worker: Worker }) {
  const { t } = useTranslation()

  return (
    <div className="grid grid-cols-2 gap-3">
      <DetailOverviewStat
        icon={CalendarIcon}
        label={t("workerDetail.created")}
        value={formatRelative(worker.created_at, t)}
        hint={formatTimestamp(worker.created_at)}
        valueClassName="tabular"
      />
      <DetailOverviewStat
        icon={History}
        label={t("workerDetail.updated")}
        value={formatRelative(worker.updated_at, t)}
        hint={formatTimestamp(worker.updated_at)}
        valueClassName="tabular"
      />
    </div>
  )
}
