import { useEffect, useMemo, useState, type ReactNode } from "react"
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  Building2,
  Clock,
  Copy,
  Hash,
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
import { Panel } from "@/components/panel"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { StatusBadge } from "@/components/status-badge"
import { CopyButton } from "@/components/copy-button"
import { WorkerAvatar } from "@/components/worker-avatar"
import { EngineIcon } from "@/components/agent-icons/engine-icon"
import { FadeIn } from "@/components/fade-in"
import { SkeletonPage } from "@/components/skeleton-loader"
import { EmptyState } from "@/components/empty-state"
import { PaginationControls } from "@/components/pagination-controls"
import { TaskList } from "@/components/task-list"
import { WorkerConstraintsPanel } from "@/components/worker-constraints-panel"
import { cn } from "@/lib/utils"
import { ALERT_DESTRUCTIVE } from "@/lib/styles"
import { formatTimestamp, formatRelative, formatEngineLabel, groupExecutionsBySession, extractMessageContent } from "@/lib/format"
import type { EnvScope } from "@/lib/types"
import { ScopeToggleCard } from "@/components/scope-toggle-card"
import { KNOWN_SCOPES, parseScopes, serializeScopes, toggleScope } from "@/lib/scopes"
import { EnvConfigPanel } from "@/components/env-config-panel"
import { useEnvList, useDepartmentEnvs } from "@/hooks/use-envs"
import { EditWorkerInfoSheet } from "@/components/edit-worker-info-sheet"
import { Can, ForbiddenBoundary } from "@/components/guard"
import { Perm, hasPermission } from "@/lib/permissions"
import { useMe } from "@/hooks/use-me"
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
// section survives refresh and is shareable. `perm` is the permission needed to
// load that section's data — viewing the worker itself only requires
// contacts:read (the route guard), but Sessions/Tasks/Env read other domains,
// so those tabs are hidden (and their requests skipped) when the user lacks the
// permission. This declarative map is the single source of truth for tab
// visibility, data fetching, and deep-link fallback.
const SECTIONS = [
  { key: "overview", labelKey: "workerDetail.overview", icon: LayoutDashboard },
  { key: "sessions", labelKey: "workerDetail.sessions", icon: Logs, perm: Perm.SessionsRead },
  { key: "tasks", labelKey: "tasks.title", icon: ListTodo, perm: Perm.TasksRead },
  { key: "constraints", labelKey: "workerDetail.constraints", icon: ScrollText },
  { key: "permissions", labelKey: "workerDetail.permissions", icon: ShieldCheck },
  { key: "env", labelKey: "envConfig.title", icon: Settings2, perm: Perm.EnvRead },
] satisfies ReadonlyArray<{ key: string; labelKey: string; icon: LucideIcon; perm?: string }>

type SectionKey = (typeof SECTIONS)[number]["key"]

// Section rail items mirror the main sidebar: 34px rows, 13px medium labels,
// gray accent fill on hover and for the selection (never blue or orange).
const RAIL_ITEM =
  "flex h-8.5 w-full items-center gap-2.5 rounded-sm px-3 text-left text-[13px] font-medium whitespace-nowrap transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"

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

  return (
    <Panel title={t("envConfig.effectiveTitle")} flush>
      {rows.length === 0 ? (
        <p className="px-4 py-8 text-center text-sm text-muted-foreground">
          {t("envConfig.noEffective")}
        </p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="pl-4">{t("envConfig.key")}</TableHead>
              <TableHead>{t("envConfig.masked")}</TableHead>
              <TableHead className="pr-4">{t("envConfig.source")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map(([key, { masked, source }]) => {
              const cfg = SOURCE_CONFIG[source]
              return (
                <TableRow key={key}>
                  <TableCell className="pl-4 font-mono text-[13px] text-strong">{key}</TableCell>
                  <TableCell className="font-mono text-[13px] text-muted-foreground">{masked}</TableCell>
                  <TableCell className="pr-4">
                    <span className={cn("text-[13px] font-medium", cfg?.color)}>
                      {cfg ? t(cfg.labelKey) : source}
                    </span>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      )}
    </Panel>
  )
}

// One attribute in the profile record: label-left, value-right, hairline-divided.
// The canonical enterprise dossier row, not a card.
function RecordRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex min-h-11 flex-col gap-1 px-4 py-2.5 @md:flex-row @md:items-center @md:gap-6">
      <dt className="shrink-0 text-[13px] text-muted-foreground @md:w-40">{label}</dt>
      <dd className="min-w-0 flex-1 text-sm text-foreground">{children}</dd>
    </div>
  )
}

export function WorkerDetail() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const { data: worker, error: workerError, refetch: refetchWorker } = useWorker(id!)
  const { data: me } = useMe()

  // Tabs the user can actually load, derived from the SECTIONS perm map. Ungated
  // sections (overview/constraints/permissions) always show; the rest appear
  // only with their permission. While `me` loads, gated tabs stay hidden to
  // avoid a flash-then-disappear.
  const visibleSections = useMemo(
    () => SECTIONS.filter((s) => !s.perm || hasPermission(me?.permissions, s.perm)),
    [me?.permissions],
  )
  const canSessions = hasPermission(me?.permissions, Perm.SessionsRead)
  // Editing a worker (permission scopes here, like the header actions) requires
  // contacts:write. Without it the scope switches stay visible as a read-only
  // view of what's granted, but disabled so they can't fire a 403'd write.
  const canWriteContacts = hasPermission(me?.permissions, Perm.ContactsWrite)

  const [searchParams, setSearchParams] = useSearchParams()
  const tabParam = searchParams.get("tab")
  // Fall back to overview when the deep-linked tab is unknown or not permitted.
  const activeSection: SectionKey = visibleSections.some((s) => s.key === tabParam)
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
    enabled: canSessions && (activeSection === "overview" || activeSection === "sessions"),
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

  if (!worker) {
    return (
      <div className="p-6">
        <SkeletonPage />
      </div>
    )
  }

  return (
    <FadeIn className="h-full">
      <div className="flex h-full flex-col md:flex-row">
        {/* Left rail: worker identity + section menu. Below md it stacks above
            the content and the menu scrolls horizontally. */}
        <aside className="flex shrink-0 flex-col border-b border-border bg-background md:w-60 md:border-r md:border-b-0">
          <div className="flex h-18 shrink-0 items-center gap-3 px-4 md:border-b md:border-border">
            <WorkerAvatar name={worker.name} status={worker.status} />
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-semibold text-strong">{worker.name}</p>
              <div className="mt-1">
                <StatusBadge status={worker.status} />
              </div>
            </div>
          </div>
          <nav className="min-h-0 overflow-x-auto px-2 pb-2 md:flex-1 md:overflow-x-hidden md:overflow-y-auto md:py-3">
            <ul className="flex gap-0.5 md:flex-col">
              {visibleSections.map((section) => {
                const isActive = section.key === activeSection
                const Icon = section.icon
                return (
                  <li key={section.key} className="shrink-0">
                    <button
                      type="button"
                      onClick={() => setActiveSection(section.key)}
                      aria-current={isActive ? "page" : undefined}
                      className={cn(
                        RAIL_ITEM,
                        isActive
                          ? "bg-accent font-semibold text-strong"
                          : "text-foreground hover:bg-accent hover:text-strong",
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

        {/* Content pane: section header on the base surface, content on the canvas. */}
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          <header className="flex min-h-18 shrink-0 flex-wrap items-center justify-between gap-x-6 gap-y-3 border-b border-border bg-background px-6 py-3">
            {/* The colleague's name titles the page; the active section is a
                quiet sub-line, since the rail already shows the selection. */}
            <div className="min-w-0">
              <h1 className="truncate text-xl leading-7 font-semibold tracking-[-0.015em] text-strong">
                {worker.name}
              </h1>
              <p className="text-[13px] leading-5 text-muted-foreground">{t(activeLabelKey)}</p>
            </div>
            <Can perm={Perm.ContactsWrite}>
              <div className="flex shrink-0 items-center gap-2">
                <Button variant="outline" onClick={() => setEditInfoSheetOpen(true)}>
                  <Pencil className="size-4" />
                  {t("common.edit")}
                </Button>
                <Button variant="outline" onClick={() => navigate(`/workers/create?copy=${worker.id}`)}>
                  <Copy className="size-4" />
                  {t("common.copy")}
                </Button>
              </div>
            </Can>
          </header>

          <div className="@container min-h-0 min-w-0 flex-1 overflow-auto p-6">
            {/* Section-scoped boundary: a 403 from a cross-domain panel degrades
                only the active section, not the whole page. Keyed by section so
                switching tabs clears a prior forbidden state. */}
            <ForbiddenBoundary key={activeSection}>
            <div className="mx-auto max-w-5xl space-y-6">
              {workerError ? (
                <div role="alert" className={ALERT_DESTRUCTIVE}>{workerError.message}</div>
              ) : null}

              {activeSection === "overview" && (
                <div className="flex flex-col gap-6">
                  {/* One record, label-left / value-right. Identity (avatar,
                      name, presence) already heads the left rail on every
                      section, so the overview doesn't repeat it; activity
                      timestamps read as facts here rather than as KPI tiles. */}
                  <Panel title={t("workerDetail.workerInfo")} flush>
                    <dl className="divide-y divide-hairline">
                      <RecordRow label={t("workers.form.description")}>
                        <span className={cn(!worker.description && "text-muted-foreground")}>
                          {worker.description || t("common.noDescription")}
                        </span>
                      </RecordRow>

                      <RecordRow label={t("workerDetail.id")}>
                        <div className="flex items-center gap-1.5">
                          <span className="min-w-0 font-mono text-[13px] break-all text-foreground">
                            {worker.id}
                          </span>
                          <CopyButton value={worker.id} />
                        </div>
                      </RecordRow>

                      <RecordRow label={t("workers.form.engine")}>
                        {worker.engine ? (
                          <span className="inline-flex items-center gap-2">
                            <EngineIcon engine={worker.engine} className="size-4 text-foreground" />
                            {formatEngineLabel(worker.engine, t)}
                          </span>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </RecordRow>

                      <RecordRow label={t("departments.title")}>
                        {worker.departments && worker.departments.length > 0 ? (
                          <div className="flex flex-wrap items-center gap-1.5">
                            {worker.departments.map((d) => (
                              <Badge key={d.id} variant="outline">
                                <Building2 />
                                {d.name}
                              </Badge>
                            ))}
                          </div>
                        ) : (
                          <span className="text-muted-foreground">{t("departments.ungrouped")}</span>
                        )}
                      </RecordRow>

                      <RecordRow label={t("workerDetail.workDir")}>
                        {worker.work_dir ? (
                          <div className="flex items-center gap-1.5">
                            <span className="min-w-0 font-mono text-[13px] break-all text-foreground">
                              {worker.work_dir}
                            </span>
                            <CopyButton value={worker.work_dir} />
                          </div>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </RecordRow>

                      {/* Session metrics live in the sessions domain, so they
                          only appear with sessions:read. */}
                      {canSessions && (
                        <>
                          <RecordRow label={t("workerDetail.sessions")}>
                            <span className="tabular-nums">{data?.total ?? 0}</span>
                          </RecordRow>
                          <RecordRow label={t("workerDetail.lastActive")}>
                            <span className="tabular-nums">
                              {latestExecution
                                ? formatTimestamp(latestExecution.started_at)
                                : t("sessions.noExecutions")}
                            </span>
                          </RecordRow>
                        </>
                      )}

                      <RecordRow label={t("workerDetail.created")}>
                        <span className="tabular-nums">{formatTimestamp(worker.created_at)}</span>
                      </RecordRow>
                    </dl>
                  </Panel>
                </div>
              )}

              {activeSection === "sessions" && (
                <div>
                  <DetailSection>
                    {sessionGroups.length === 0 ? (
                      <EmptyState title={t("sessions.noExecutions")} />
                    ) : (
                      <div className="divide-y divide-hairline">
                        {sessionGroups.map((group) => {
                          const latest = group[0]
                          const oldest = group[group.length - 1]
                          const intent = extractMessageContent(oldest.trigger_input)
                          const shortId = latest.session_id.slice(0, 8)

                          return (
                            <div
                              key={latest.session_id}
                              className="group relative flex items-center gap-4 px-4 py-3 transition-colors hover:bg-elevated"
                            >
                              {/* Whole-row click target; the copy chip below is raised above it. */}
                              <Link
                                to={`/sessions/detail?session_id=${encodeURIComponent(latest.session_id)}`}
                                aria-label={intent || t("sessions.noTriggerContent")}
                                className="absolute inset-0 rounded-sm focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none focus-visible:ring-inset"
                              />

                              <div className="min-w-0 flex-1">
                                <p className="truncate text-sm font-medium text-strong">
                                  {intent || t("sessions.noTriggerContent")}
                                </p>

                                <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-[13px] text-muted-foreground">
                                  <span
                                    className="inline-flex items-center gap-1 tabular-nums"
                                    title={oldest.started_at ? formatTimestamp(oldest.started_at) : undefined}
                                  >
                                    <Clock className="size-3.5" aria-hidden="true" />
                                    {formatRelative(oldest.started_at, t)}
                                  </span>
                                  <span className="inline-flex items-center gap-1">
                                    <Hash className="size-3.5" aria-hidden="true" />
                                    <span className="font-mono text-xs">{shortId}</span>
                                    <CopyButton
                                      value={latest.session_id}
                                      className="relative z-10 opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
                                    />
                                  </span>
                                </div>
                              </div>

                              <StatusBadge status={latest.status} />
                            </div>
                          )
                        })}
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
                <div className="max-w-3xl space-y-3">
                  <p className="text-[13px] leading-5 text-muted-foreground">
                    {canWriteContacts
                      ? t("workers.form.permissionsHelper")
                      : t("workerDetail.permissionsReadonly")}
                  </p>

                  <DetailSection className="divide-y divide-hairline">
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
                        disabled={!canWriteContacts || updateWorker.isPending}
                      />
                    ))}
                  </DetailSection>
                </div>
              )}

              {activeSection === "env" && (
                <div className="max-w-4xl space-y-6">
                  <EnvConfigPanel
                    scope="worker"
                    scopeId={id!}
                    title={t("envConfig.workerTitle")}
                  />

                  <EffectiveEnvPreview
                    workerId={id!}
                    departmentIds={workerDeptIds}
                  />
                </div>
              )}
            </div>
            </ForbiddenBoundary>
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
