import { useMemo, useState } from "react"
import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { keepPreviousData, useQuery } from "@tanstack/react-query"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Select } from "@/components/arc/select/select"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { PaginationControls } from "@/components/pagination-controls"
import { SessionGroupRow, SessionGroupRowSkeleton } from "@/components/session-group-row"
import { useWorkers } from "@/hooks/use-workers"
import { api } from "@/lib/api"
import { groupExecutionsBySession, isActiveStatus } from "@/lib/format"

const PAGE_SIZE = 20
const ALL_WORKERS = "all"
const SKELETON_ROWS = 6

export function Sessions() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [page, setPage] = useState(1)
  const [workerFilter, setWorkerFilter] = useState(ALL_WORKERS)
  const workerId = workerFilter === ALL_WORKERS ? undefined : workerFilter

  const { data, error, isLoading, refetch, isRefetching } = useQuery({
    queryKey: ["executions", page, PAGE_SIZE, workerId ?? null],
    queryFn: () => api.sessions.list(page, PAGE_SIZE, workerId),
    placeholderData: keepPreviousData,
    refetchInterval: (query) => {
      const items = query.state.data?.items ?? []
      return items.some((execution) => isActiveStatus(execution.status)) ? 500 : false
    },
  })
  const { data: workers } = useWorkers()

  const executions = useMemo(() => data?.items ?? [], [data])
  const sessionGroups = useMemo(() => groupExecutionsBySession(executions), [executions])
  const totalSessions = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(totalSessions / PAGE_SIZE))
  const activeCount = sessionGroups.filter((group) => isActiveStatus(group[0].status)).length

  const workerOptions = useMemo(
    () => [
      { value: ALL_WORKERS, label: t("sessions.allWorkers") },
      ...[...(workers ?? [])]
        .sort((a, b) => a.name.localeCompare(b.name))
        .map((worker) => ({ value: worker.id, label: worker.name })),
    ],
    [workers, t]
  )

  const subtitle =
    totalSessions > 0
      ? activeCount > 0
        ? t("sessions.summaryWithActive", { count: totalSessions, active: activeCount })
        : t("sessions.summary", { count: totalSessions })
      : undefined

  const changeWorker = (value: string) => {
    setWorkerFilter(value)
    setPage(1)
  }

  const filtered = workerId !== undefined

  return (
    <FadeIn>
      <PageHeader
        title={t("sessions.title")}
        subtitle={subtitle}
        actions={
          <div className="w-60 max-w-full">
            <Select
              label={t("sessions.filterWorker")}
              options={workerOptions}
              value={workerFilter}
              onValueChange={changeWorker}
            />
          </div>
        }
      />

      {error && (
        <div className="mb-4">
          <Alert tone="danger" title={t("sessions.loadError")}>
            {error.message}
            <span className="mt-3 flex">
              <Button variant="secondary" size="sm" loading={isRefetching} onClick={() => void refetch()}>
                {t("common.retry")}
              </Button>
            </span>
          </Alert>
        </div>
      )}

      {isLoading ? (
        <div
          aria-busy="true"
          className="divide-y divide-border-subtle overflow-hidden rounded-surface border border-border bg-surface"
        >
          {Array.from({ length: SKELETON_ROWS }).map((_, index) => (
            <SessionGroupRowSkeleton key={index} />
          ))}
        </div>
      ) : sessionGroups.length === 0 ? (
        error ? null : filtered ? (
          <EmptyState
            title={t("sessions.noWorkerSessions")}
            description={t("sessions.noWorkerSessionsDesc")}
            action={
              <Button variant="secondary" onClick={() => changeWorker(ALL_WORKERS)}>
                {t("sessions.showAllWorkers")}
              </Button>
            }
          />
        ) : (
          <EmptyState
            title={t("emptyState.noExecutions")}
            description={t("sessions.emptyDescription")}
            action={
              <Button variant="secondary" onClick={() => navigate("/chat")}>
                {t("sessions.openChat")}
              </Button>
            }
          />
        )
      ) : (
        <>
          <section
            aria-label={t("sessions.title")}
            className="divide-y divide-border-subtle overflow-hidden rounded-surface border border-border bg-surface"
          >
            {sessionGroups.map((group) => (
              <SessionGroupRow
                key={group[0].session_id}
                group={group}
                tokenStats={data?.token_stats?.[group[0].session_id]}
                showWorker
              />
            ))}
          </section>

          <PaginationControls page={page} totalPages={totalPages} onPageChange={setPage} />
        </>
      )}
    </FadeIn>
  )
}
