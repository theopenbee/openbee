import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { Alert, type AlertTone } from "@/components/arc/alert/alert"
import { Avatar } from "@/components/arc/avatar/avatar"
import { Badge, type BadgeTone } from "@/components/arc/badge/badge"
import { Button } from "@/components/arc/button/button"
import { Select } from "@/components/arc/select/select"
import { TextShimmer } from "@/components/arc/text-shimmer/text-shimmer"
import { EngineIcon } from "@/components/agent-icons/engine-icon"
import { ExecutionSteps } from "@/components/chat/execution-steps"
import { WorkerAvatar } from "@/components/worker-avatar"
import { useEnabledEngines } from "@/hooks/use-config"
import { useExecution } from "@/hooks/use-execution"
import { useWorkers } from "@/hooks/use-workers"
import { ApiError } from "@/lib/api"
import { formatMessageTimestamp } from "@/lib/chat-format"
import { findEngine } from "@/lib/engines"
import { formatDuration, knownSteps, stepStats } from "@/lib/execution"
import type { EngineChoice, ExecutionDetail, ExecutionStatus, ExecutionStep, Worker } from "@/lib/types"

const STATUS_TONE: Record<ExecutionStatus, BadgeTone> = {
  running: "info",
  completed: "success",
  failed: "danger",
}

const NOTICE_TONE: Record<string, AlertTone> = {
  transcript_unsupported: "info",
  engine_unavailable: "warning",
  transcript_failed: "warning",
}

function useNow(active: boolean) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!active) return
    setNow(Date.now())
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [active])
  return now
}

interface ExecutionPanelProps {
  executionId: string
  onSelectRun: (executionId: string) => void
}

export function ExecutionPanel({ executionId, onSelectRun }: ExecutionPanelProps) {
  const { t } = useTranslation()
  const { data, isPending, error, refetch, isFetching } = useExecution(executionId)
  const { data: workers } = useWorkers()
  const engines = useEnabledEngines()

  if (isPending) {
    return (
      <p role="status" className="text-sm text-secondary">
        {t("execution.loading")}
      </p>
    )
  }

  if (!data) {
    if (error instanceof ApiError && error.status === 404) {
      return <p className="text-sm text-secondary">{t("execution.notFound")}</p>
    }
    return (
      <div className="grid justify-items-start gap-3">
        <p className="text-sm text-secondary">{t("common.loadError")}</p>
        <Button variant="secondary" size="sm" loading={isFetching} onClick={() => void refetch()}>
          {t("common.retry")}
        </Button>
      </div>
    )
  }

  return (
    <ExecutionView
      execution={data}
      worker={workers?.find((item) => item.id === data.worker_id)}
      engine={findEngine(engines, data.engine)}
      onSelectRun={onSelectRun}
    />
  )
}

interface ExecutionViewProps {
  execution: ExecutionDetail
  worker: Worker | undefined
  engine: EngineChoice
  onSelectRun: (executionId: string) => void
}

export function ExecutionView({ execution, worker, engine, onSelectRun }: ExecutionViewProps) {
  const steps = knownSteps(execution.steps)
  const running = execution.status === "running"
  const failure = execution.status === "failed" && execution.error ? execution.error : null

  return (
    <div className="grid gap-6">
      <ExecutionSummary execution={execution} worker={worker} engine={engine} onSelectRun={onSelectRun} />
      <ExecutionNotice execution={execution} empty={steps.length === 0} />
      {(steps.length > 0 || failure) && <ExecutionSteps steps={steps} running={running} failure={failure} />}
      <ExecutionFooter execution={execution} steps={steps} />
    </div>
  )
}

function ExecutionSummary({ execution, worker, engine, onSelectRun }: ExecutionViewProps) {
  const { t, i18n } = useTranslation()
  const running = execution.status === "running"
  const now = useNow(running)
  const end = execution.completed_at ?? (running ? now : null)
  const duration = end === null ? null : formatDuration(end - execution.started_at, t)
  const instruction = execution.instruction.replaceAll("`", "").trim()

  return (
    <div className="grid gap-3 border-b border-border pb-5">
      <div className="flex min-w-0 items-center gap-2.5">
        {worker ? (
          <WorkerAvatar workerId={worker.id} name={worker.name} size="sm" />
        ) : (
          <Avatar name="?" size="sm" aria-hidden="true" />
        )}
        <span className="truncate text-sm font-medium text-foreground">{worker?.name ?? t("chat.deletedWorker")}</span>
        <Badge tone={STATUS_TONE[execution.status]} size="sm" className="ml-auto shrink-0">
          {t(`execution.status.${execution.status}`)}
        </Badge>
      </div>
      {instruction && <p className="line-clamp-3 text-sm whitespace-pre-line text-foreground">{instruction}</p>}
      <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-xs text-secondary tabular">
        {engine.id && (
          <span className="inline-flex min-w-0 items-center gap-1.5">
            <EngineIcon icon={engine.icon} className="size-3.5" />
            <span className="truncate">{engine.name}</span>
          </span>
        )}
        <span>{t("execution.startedAt", { time: formatMessageTimestamp(execution.started_at, i18n.language) })}</span>
        {duration &&
          (running ? (
            <span className="inline-flex items-center gap-1.5">
              <span aria-hidden="true" className="relative flex size-1.5">
                <span className="absolute inline-flex size-full animate-ping rounded-full bg-success opacity-60" />
                <span className="relative inline-flex size-1.5 rounded-full bg-success" />
              </span>
              {t("execution.elapsed", { duration })}
            </span>
          ) : (
            <span>{t("execution.took", { duration })}</span>
          ))}
      </div>
      {execution.runs && execution.runs.length > 1 && (
        <RunSwitcher current={execution.id} runs={execution.runs} onSelectRun={onSelectRun} />
      )}
    </div>
  )
}

function RunSwitcher({
  current,
  runs,
  onSelectRun,
}: {
  current: string
  runs: NonNullable<ExecutionDetail["runs"]>
  onSelectRun: (executionId: string) => void
}) {
  const { t, i18n } = useTranslation()
  const options = runs.map((run) => ({
    value: run.id,
    label: t("execution.runOption", {
      time: formatMessageTimestamp(run.started_at, i18n.language),
      status: t(`execution.status.${run.status}`),
    }),
  }))

  return (
    <div className="[&_label]:sr-only">
      <Select label={t("execution.runs")} options={options} value={current} onValueChange={onSelectRun} />
    </div>
  )
}

function ExecutionNotice({ execution, empty }: { execution: ExecutionDetail; empty: boolean }) {
  const { t } = useTranslation()
  const stepsError = execution.steps_error

  if (stepsError) {
    return (
      <Alert
        tone={NOTICE_TONE[stepsError] ?? "warning"}
        title={t(`execution.stepsError.${stepsError}`, { defaultValue: t("execution.stepsError.transcript_failed") })}
      />
    )
  }
  if (!empty) return null
  if (execution.status === "running") {
    return (
      <TextShimmer as="p" className="text-sm text-secondary">
        {t("execution.waiting")}
      </TextShimmer>
    )
  }
  return <p className="text-sm text-secondary">{t("execution.noSteps")}</p>
}

function ExecutionFooter({ execution, steps }: { execution: ExecutionDetail; steps: ExecutionStep[] }) {
  const { t } = useTranslation()
  const running = execution.status === "running"
  if (!running && steps.length === 0) return null
  const stats = stepStats(steps)

  return (
    <p className="border-t border-border pt-4 text-xs text-muted">
      {running
        ? t("execution.live")
        : t("execution.stepCount", {
            steps: t("execution.steps", { count: stats.steps }),
            tools: t("execution.toolCalls", { count: stats.tools }),
          })}
    </p>
  )
}
