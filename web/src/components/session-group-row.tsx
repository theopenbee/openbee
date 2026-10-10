import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { Clock, Hash, Logs, Zap } from "lucide-react"
import { StatusBadge } from "@/components/status-badge"
import { CopyButton } from "@/components/copy-button"
import { TokenStatsInfoButton } from "@/components/token-stats-tooltip"
import { cn } from "@/lib/utils"
import { extractMessageContent, formatRelative, formatTimestamp, formatTokenCount, isActiveStatus } from "@/lib/format"
import type { SessionTokenStats, WorkerExecution } from "@/lib/types"

const META_ICON = "size-3.5 text-muted-foreground/70"

export function SessionGroupRow({
  group,
  tokenStats,
  showWorker = false,
}: {
  group: WorkerExecution[]
  tokenStats?: SessionTokenStats | null
  showWorker?: boolean
}) {
  const { t } = useTranslation()
  const latest = group[0]
  const oldest = group[group.length - 1]
  const intent = extractMessageContent(oldest.trigger_input) || t("sessions.noTriggerContent")

  return (
    <div
      className={cn(
        "group relative flex items-center gap-4 px-4 py-3 transition-colors hover:bg-primary/5 sm:px-6 sm:py-4",
        isActiveStatus(latest.status) && "bg-status-working/[0.04]"
      )}
    >
      <Link
        to={`/sessions/detail?session_id=${encodeURIComponent(latest.session_id)}`}
        aria-label={t("sessions.viewSession", { id: latest.session_id })}
        className="absolute inset-0 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
      />

      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-foreground">{intent}</p>

        <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          {showWorker && latest.worker_id && (
            <Link
              to={`/workers/${latest.worker_id}`}
              className="relative z-10 text-foreground/80 transition-colors hover:text-foreground"
            >
              {latest.worker_name || latest.worker_id.slice(0, 8)}
            </Link>
          )}
          <span
            className="inline-flex items-center gap-1"
            title={oldest.started_at ? formatTimestamp(oldest.started_at) : undefined}
          >
            <Clock className={META_ICON} aria-hidden="true" />
            {formatRelative(oldest.started_at, t)}
          </span>
          <span className="inline-flex items-center gap-1 font-mono">
            <Logs className={META_ICON} aria-hidden="true" />
            {t("sessions.turnCount", { count: group.length })}
          </span>
          {tokenStats && (
            <span className="inline-flex items-center gap-1 font-mono">
              <Zap className={META_ICON} aria-hidden="true" />
              {formatTokenCount(tokenStats.total_tokens)}
              <span className="relative z-10 inline-flex">
                <TokenStatsInfoButton stats={tokenStats} />
              </span>
            </span>
          )}
          <span className="inline-flex items-center gap-1">
            <Hash className={META_ICON} aria-hidden="true" />
            <span className="font-mono">{latest.session_id.slice(0, 8)}</span>
            <CopyButton
              value={latest.session_id}
              className="relative z-10 transition-opacity pointer-fine:opacity-0 pointer-fine:group-hover:opacity-100 pointer-fine:focus-visible:opacity-100"
            />
          </span>
        </div>
      </div>

      <StatusBadge status={latest.status} />
    </div>
  )
}
