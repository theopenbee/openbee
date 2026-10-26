import { useEffect, useMemo, useState, type ReactNode } from "react"
import { useNavigate, useParams, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  Building2,
  CalendarIcon,
  Clock,
  Copy,
  LayoutDashboard,
  ListTodo,
  Logs,
  Pencil,
  ScrollText,
  Settings2,
  ShieldCheck,
  type LucideIcon,
} from "lucide-react"
import { useWorker, useWorkerExecutions, useUpdateWorker } from "@/hooks/use-workers"
import { DetailSection } from "@/components/detail-primitives"
import { Button } from "@/components/ui/button"
import { CopyButton } from "@/components/copy-button"
import { WorkerAvatar, initials, presenceColor } from "@/components/worker-avatar"
import { EngineIcon } from "@/components/agent-icons/engine-icon"
import { FadeIn } from "@/components/fade-in"
import { SkeletonPage } from "@/components/skeleton-loader"
import { EmptyState } from "@/components/empty-state"
import { PaginationControls } from "@/components/pagination-controls"
import { TaskList } from "@/components/task-list"
import { SessionGroupRow } from "@/components/session-group-row"
import { WorkerConstraintsPanel } from "@/components/worker-constraints-panel"
import { cn } from "@/lib/utils"
import { EYEBROW_LABEL } from "@/lib/styles"
import { formatTimestamp, formatEngineLabel, groupExecutionsBySession } from "@/lib/format"
import type { EnvScope } from "@/lib/types"
import { ScopeToggleCard } from "@/components/scope-toggle-card"
import { KNOWN_SCOPES, parseScopes, serializeScopes, toggleScope } from "@/lib/scopes"
import { EnvConfigPanel } from "@/components/env-config-panel"
import { useEnvList, useDepartmentEnvs } from "@/hooks/use-envs"
import { EditWorkerInfoSheet } from "@/components/edit-worker-info-sheet"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

const PAGE_SIZE = 20

// Left-rail navigation: each entry maps a menu item to the content rendered in
// the right pane. The `key` is mirrored to the URL (`?tab=`) so the active
// section survives refresh and is shareable.
const SECTIONS = [
  { key: "overview", labelKey: "workerDetail.overview", icon: LayoutDashboard },
  { key: "sessions", labelKey: "workerDetail.sessions", icon: Logs },
  { key: "tasks", labelKey: "tasks.title", icon: ListTodo },
  { key: "constraints", labelKey: "workerDetail.constraints", icon: ScrollText },
  { key: "permissions", labelKey: "workerDetail.permissions", icon: ShieldCheck },
  { key: "env", labelKey: "envConfig.title", icon: Settings2 },
] satisfies ReadonlyArray<{ key: string; labelKey: string; icon: LucideIcon }>

type SectionKey = (typeof SECTIONS)[number]["key"]

const revealTab = (el: HTMLButtonElement | null) => {
  el?.scrollIntoView({ block: "nearest", inline: "nearest" })
}

// Env source is a configuration layer, not a presence status, so it stays in the
// achromatic field: muted by default, with the effective (worker-level) override
// lifted to full-weight foreground rather than a non-brand accent color.
const SOURCE_CONFIG: Record<Exclude<EnvScope, "bee">, { color: string; labelKey: string }> = {
  global: { color: "text-muted-foreground", labelKey: "envConfig.sourceGlobal" },
  department: { color: "text-muted-foreground", labelKey: "envConfig.sourceDepartment" },
  worker: { color: "text-foreground", labelKey: "envConfig.sourceWorker" },
}

function EffectiveEnvPreview({ workerId, departmentIds }: { workerId: string; departmentIds: string[] }) {
  const { t } = useTranslation()
  const { data: globalEnvs = [] } = useEnvList("global")
  const { data: workerEnvs = [] } = useEnvList("worker", workerId)
  const deptEnvsList = useDepartmentEnvs(departmentIds)

  const rows = useMemo(() => {
    const merged = new Map<string, { masked: string; source: Exclude<EnvScope, "bee"> }>()

    for (const env of globalEnvs) {
      merged.set(env.key, { masked: env.masked, source: "global" })
    }

    for (const deptEnvs of deptEnvsList) {
      for (const env of deptEnvs) {
        merged.set(env.key, { masked: env.masked, source: "department" })
      }
    }

    for (const env of workerEnvs) {
      merged.set(env.key, { masked: env.masked, source: "worker" })
    }

    return Array.from(merged.entries()).sort(([a], [b]) => a.localeCompare(b))
  }, [globalEnvs, workerEnvs, deptEnvsList])

  if (rows.length === 0) {
    return (
      <div className="rounded-sm border border-dashed border-border/80 bg-background/75 px-4 py-8 text-sm leading-6 text-muted-foreground text-center">
        {t("envConfig.noEffective")}
      </div>
    )
  }

  return (
    <div className="rounded-sm border border-border/70 overflow-hidden">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t("envConfig.key")}</TableHead>
            <TableHead>{t("envConfig.masked")}</TableHead>
            <TableHead>{t("envConfig.source")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map(([key, { masked, source }]) => {
            const cfg = SOURCE_CONFIG[source]
            return (
              <TableRow key={key}>
                <TableCell className="font-mono text-sm">{key}</TableCell>
                <TableCell className="font-mono text-sm text-muted-foreground">{masked}</TableCell>
                <TableCell>
                  <span className={cn("text-xs font-medium", cfg?.color)}>
                    {cfg ? t(cfg.labelKey) : source}
                  </span>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}

// Masthead avatar: larger than the roster avatar to anchor the profile, carrying
// the same presence dot and presence-color language. Flat fill, no shadow.
function ProfileAvatar({ name, status }: { name: string; status: string }) {
  const color = presenceColor[status] ?? "bg-muted-foreground"
  return (
    <span className="relative inline-flex shrink-0 self-start">
      <span className="flex size-16 select-none items-center justify-center rounded-full bg-muted text-xl font-medium text-muted-foreground ring-1 ring-border">
        {initials(name)}
      </span>
      <span className="absolute right-1 bottom-1 inline-flex size-3.5 items-center justify-center rounded-full ring-2 ring-background">
        {status === "working" && (
          <span className={cn("absolute inline-flex size-full animate-ping rounded-full opacity-60", color)} />
        )}
        <span className={cn("relative inline-flex size-full rounded-full", color)} />
      </span>
    </span>
  )
}

// One attribute in the profile record: label-left, value-right, hairline-divided.
// The canonical enterprise dossier row, not a card.
function RecordRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1 py-3 sm:flex-row sm:gap-6">
      <dt className="shrink-0 text-xs font-medium text-muted-foreground sm:w-40 sm:pt-0.5">{label}</dt>
      <dd className="min-w-0 flex-1 text-sm text-foreground">{children}</dd>
    </div>
  )
}

function WorkerActions({
  workerId,
  compact,
  className,
  onEdit,
}: {
  workerId: string
  compact: boolean
  className?: string
  onEdit: () => void
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const actions = [
    { key: "edit", icon: Pencil, label: t("common.edit"), onClick: onEdit },
    { key: "copy", icon: Copy, label: t("common.copy"), onClick: () => navigate(`/workers/create?copy=${workerId}`) },
  ]

  return (
    <div className={cn("flex shrink-0 items-center", compact ? "gap-1" : "gap-2", className)}>
      {actions.map(({ key, icon: Icon, label, onClick }) => (
        <Button
          key={key}
          variant={compact ? "ghost" : "outline"}
          size={compact ? "icon-lg" : "sm"}
          onClick={onClick}
          aria-label={compact ? label : undefined}
        >
          <Icon className="size-4" />
          {!compact && label}
        </Button>
      ))}
    </div>
  )
}

export function WorkerDetail() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { data: worker, error: workerError, refetch: refetchWorker } = useWorker(id!)

  const [searchParams, setSearchParams] = useSearchParams()
  const tabParam = searchParams.get("tab")
  const activeSection: SectionKey = SECTIONS.some((s) => s.key === tabParam)
    ? (tabParam as SectionKey)
    : "overview"
  const setActiveSection = (key: SectionKey) => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        next.set("tab", key)
        return next
      },
      { replace: true },
    )
  }
  const activeLabelKey = SECTIONS.find((s) => s.key === activeSection)!.labelKey

  // Sessions are only read on the Overview (count) and Sessions (list) tabs, so
  // skip the request entirely on the other sections.
  const [page, setPage] = useState(1)
  const { data } = useWorkerExecutions(id!, page, PAGE_SIZE, {
    enabled: activeSection === "overview" || activeSection === "sessions",
  })

  const executions = data?.items ?? []
  const totalPages = Math.max(1, Math.ceil((data?.total ?? 0) / PAGE_SIZE))
  const latestExecution = executions[0]

  const sessionGroups = useMemo(() => groupExecutionsBySession(executions), [executions])

  const updateWorker = useUpdateWorker()
  const [localScopes, setLocalScopes] = useState<string[]>([])

  useEffect(() => {
    setLocalScopes(parseScopes(worker?.permission_scopes ?? ""))
  }, [worker?.permission_scopes])

  const [editInfoSheetOpen, setEditInfoSheetOpen] = useState(false)
  const workerDeptIds = useMemo(
    () => worker?.departments?.map((d) => d.id).sort() ?? [],
    [worker?.departments]
  )

  if (workerError && !worker) {
    return (
      <FadeIn className="h-full">
        <div className="flex h-full items-center justify-center p-6">
          <EmptyState
            title={t("common.loadError")}
            description={workerError.message}
            action={
              <Button variant="outline" size="sm" onClick={() => refetchWorker()}>
                {t("common.retry")}
              </Button>
            }
          />
        </div>
      </FadeIn>
    )
  }

  if (!worker) return <SkeletonPage />

  const overviewStats = [
    {
      icon: Logs,
      label: t("workerDetail.sessions"),
      value: data?.total ?? 0,
      valueClass: "text-lg font-medium tabular-nums",
    },
    {
      icon: Clock,
      label: t("workerDetail.lastActive"),
      value: latestExecution
        ? formatTimestamp(latestExecution.started_at)
        : t("sessions.noExecutions"),
      valueClass: "text-sm",
    },
    {
      icon: CalendarIcon,
      label: t("workerDetail.created"),
      value: formatTimestamp(worker.created_at),
      valueClass: "text-sm",
    },
  ]

  const openEditInfoSheet = () => setEditInfoSheetOpen(true)

  return (
    <FadeIn className="h-full">
      <div className="flex h-full flex-col lg:flex-row">
        {/* Left rail: worker identity + vertical section menu. */}
        <aside className="flex shrink-0 flex-col border-b lg:w-60 lg:border-r lg:border-b-0">
          <div className="flex h-14 items-center gap-3 px-4 lg:h-16 lg:border-b">
            <WorkerAvatar name={worker.name} status={worker.status} />
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-semibold tracking-tight">{worker.name}</p>
            </div>
            <WorkerActions workerId={worker.id} compact className="lg:hidden" onEdit={openEditInfoSheet} />
          </div>
          <nav className="scroll-pe-6 overflow-x-auto px-2 pb-2 [mask-image:linear-gradient(to_right,#000_calc(100%-1.5rem),transparent)] [scrollbar-width:none] lg:min-h-0 lg:flex-1 lg:overflow-auto lg:p-2 lg:[mask-image:none]">
            <ul className="flex w-max gap-1 pr-6 lg:block lg:w-auto lg:space-y-1 lg:pr-0">
              {SECTIONS.map((section) => {
                const isActive = section.key === activeSection
                const Icon = section.icon
                return (
                  <li key={section.key}>
                    <button
                      ref={isActive ? revealTab : undefined}
                      type="button"
                      onClick={() => setActiveSection(section.key)}
                      aria-current={isActive ? "page" : undefined}
                      className={cn(
                        "flex w-full items-center gap-2.5 whitespace-nowrap rounded-sm px-3 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
                        isActive
                          ? "bg-secondary text-foreground"
                          : "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
                      )}
                    >
                      <Icon className="size-4 shrink-0" />
                      <span className="truncate">{t(section.labelKey)}</span>
                    </button>
                  </li>
                )
              })}
            </ul>
          </nav>
        </aside>

        {/* Right pane: section header + scrollable content. */}
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          <h1 className="sr-only lg:hidden">{t(activeLabelKey)}</h1>
          <div className="hidden h-16 items-center justify-between gap-4 border-b px-6 lg:flex">
            <h1 className="text-lg font-semibold tracking-tight">{t(activeLabelKey)}</h1>
            <WorkerActions workerId={worker.id} compact={false} onEdit={openEditInfoSheet} />
          </div>

          <div className="min-w-0 flex-1 overflow-auto px-4 py-4 md:px-6 md:py-5">
            <div className="mx-auto max-w-5xl space-y-6">
              {workerError ? (
                <p className="text-destructive">{workerError.message}</p>
              ) : null}

              {activeSection === "overview" && (
                <div className="flex flex-col gap-8">
                  {/* (1) Masthead: the identity hero — avatar with presence, name,
                      status, and the worker's role read as a profile bio. */}
                  <div className="flex flex-col gap-5 sm:flex-row sm:items-start">
                    <ProfileAvatar name={worker.name} status={worker.status} />
                    <div className="min-w-0 flex-1 space-y-2.5">
                      <h2 className="text-2xl font-semibold tracking-tight text-foreground">
                        {worker.name}
                      </h2>
                      <p className="max-w-2xl text-sm leading-6 text-muted-foreground">
                        {worker.description || t("common.noDescription")}
                      </p>
                    </div>
                  </div>

                  {/* (2) Activity band: at-a-glance presence facts, divided by
                      hairlines into equal columns — no cards, no hero metric. */}
                  <div className="grid grid-cols-1 divide-y divide-border/60 border-y border-border/60 sm:grid-cols-3 sm:divide-x sm:divide-y-0">
                    {overviewStats.map(({ icon: Icon, label, value, valueClass }, i) => (
                      <div key={label} className={cn("py-4", i === 0 ? "sm:pr-6" : "sm:px-6")}>
                        <div className={cn(EYEBROW_LABEL, "flex items-center gap-1.5")}>
                          <Icon className="size-3.5" />
                          <span>{label}</span>
                        </div>
                        <p className={cn("mt-2 font-mono text-foreground", valueClass)}>{value}</p>
                      </div>
                    ))}
                  </div>

                  {/* (3) Record: the structured attribute dossier, label-left and
                      value-right, the canonical enterprise profile pattern. */}
                  <div>
                    <p className={EYEBROW_LABEL}>{t("workerDetail.workerInfo")}</p>
                    <dl className="mt-2 divide-y divide-border/60 border-t border-border/60">
                      <RecordRow label={t("workerDetail.id")}>
                        <div className="flex items-center gap-1.5">
                          <span className="min-w-0 break-all font-mono text-xs text-muted-foreground">
                            {worker.id}
                          </span>
                          <CopyButton value={worker.id} />
                        </div>
                      </RecordRow>

                      <RecordRow label={t("workers.form.engine")}>
                        {worker.engine ? (
                          <EngineIcon
                            engine={worker.engine}
                            title={formatEngineLabel(worker.engine, t)}
                            className="size-5 text-foreground"
                          />
                        ) : (
                          "—"
                        )}
                      </RecordRow>

                      <RecordRow label={t("departments.title")}>
                        {worker.departments && worker.departments.length > 0 ? (
                          <div className="flex flex-wrap items-center gap-1.5">
                            {worker.departments.map((d) => (
                              <span
                                key={d.id}
                                className="inline-flex items-center gap-1.5 rounded-sm border border-border bg-background px-2 py-0.5 text-xs text-muted-foreground"
                              >
                                <Building2 className="size-3 shrink-0" />
                                {d.name}
                              </span>
                            ))}
                          </div>
                        ) : (
                          <span className="text-muted-foreground">{t("departments.ungrouped")}</span>
                        )}
                      </RecordRow>

                      <RecordRow label={t("workerDetail.workDir")}>
                        {worker.work_dir ? (
                          <div className="flex items-center gap-1.5">
                            <span className="min-w-0 break-all font-mono text-xs text-muted-foreground">
                              {worker.work_dir}
                            </span>
                            <CopyButton value={worker.work_dir} />
                          </div>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </RecordRow>
                    </dl>
                  </div>
                </div>
              )}

              {activeSection === "sessions" && (
                <div className="space-y-4">
            <DetailSection>
              {sessionGroups.length === 0 ? (
                <div className="px-5 py-10 sm:px-6">
                  <EmptyState title={t("sessions.noExecutions")} />
                </div>
              ) : (
                <div className="divide-y divide-border/70">
                  {sessionGroups.map((group) => (
                    <SessionGroupRow
                      key={group[0].session_id}
                      group={group}
                      tokenStats={data?.token_stats?.[group[0].session_id]}
                    />
                  ))}
                </div>
              )}
            </DetailSection>

            <PaginationControls
              page={page}
              totalPages={totalPages}
              onPageChange={setPage}
              leadingLabel={t("sessions.summary", { count: data?.total ?? 0 })}
            />
                </div>
              )}

              {activeSection === "tasks" && (
                <TaskList workerId={id!} />
              )}

              {activeSection === "constraints" && (
                <WorkerConstraintsPanel worker={worker} />
              )}

              {activeSection === "permissions" && (
                <div className="max-w-2xl space-y-4">
                  <p className="text-sm leading-6 text-muted-foreground">
                    {t("workers.form.permissionsHelper")}
                  </p>

                  <DetailSection className="divide-y divide-border/70">
                    {KNOWN_SCOPES.map((scope) => (
                      <ScopeToggleCard
                        key={scope.id}
                        scope={scope}
                        checked={localScopes.includes(scope.id)}
                        onToggle={(scopeId, val) => {
                          const prevScopes = localScopes
                          const newScopes = toggleScope(localScopes, scopeId, val)
                          setLocalScopes(newScopes)
                          updateWorker.mutate(
                            { id: id!, data: { permission_scopes: serializeScopes(newScopes) } },
                            { onError: () => setLocalScopes(prevScopes) }
                          )
                        }}
                        disabled={updateWorker.isPending}
                      />
                    ))}
                  </DetailSection>
                </div>
              )}

              {activeSection === "env" && (
                <div className="max-w-3xl space-y-6">
                  <DetailSection className="p-5 sm:p-6">
                    <EnvConfigPanel
                      scope="worker"
                      scopeId={id!}
                      title={t("envConfig.workerTitle")}
                    />
                  </DetailSection>

                  <DetailSection className="p-5 sm:p-6 space-y-4">
                    <div>
                      <p className={EYEBROW_LABEL}>
                        {t("envConfig.effectiveTitle")}
                      </p>
                    </div>
                    <EffectiveEnvPreview
                      workerId={id!}
                      departmentIds={workerDeptIds}
                    />
                  </DetailSection>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>

      <EditWorkerInfoSheet
        open={editInfoSheetOpen}
        onOpenChange={setEditInfoSheetOpen}
        worker={worker}
      />
    </FadeIn>
  )
}
