import { useEffect, useMemo, useState, type ReactNode } from "react"
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  Building2,
  CalendarIcon,
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
import { Button } from "@/components/ui/button"
import { StatusBadge } from "@/components/status-badge"
import { CopyButton } from "@/components/copy-button"
import { WorkerAvatar, initials, presenceColor } from "@/components/worker-avatar"
import { EngineIcon } from "@/components/agent-icons/engine-icon"
import { FadeIn } from "@/components/fade-in"
import { SkeletonPage } from "@/components/skeleton-loader"
import { EmptyState } from "@/components/empty-state"
import { PaginationControls } from "@/components/pagination-controls"
import { TaskList } from "@/components/task-list"
import { WorkerConstraintsPanel } from "@/components/worker-constraints-panel"
import { cn } from "@/lib/utils"
import { EYEBROW_LABEL } from "@/lib/styles"
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

// Static Tailwind class lookup for the overview activity band, whose column
// count shrinks when the session metrics are hidden (no sessions:read).
const GRID_COLS: Record<number, string> = {
  1: "sm:grid-cols-1",
  2: "sm:grid-cols-2",
  3: "sm:grid-cols-3",
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
    <span className="relative inline-flex shrink-0">
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

  if (!worker) return <SkeletonPage />

  // The session metrics live in the sessions domain, so they only appear with
  // sessions:read; the band collapses to the remaining columns otherwise.
  const overviewStats = [
    ...(canSessions
      ? [
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
        ]
      : []),
    {
      icon: CalendarIcon,
      label: t("workerDetail.created"),
      value: formatTimestamp(worker.created_at),
      valueClass: "text-sm",
    },
  ]

  return (
    <FadeIn className="h-full">
      <div className="flex h-full">
        {/* Left rail: worker identity + vertical section menu. */}
        <aside className="flex w-60 shrink-0 flex-col border-r">
          <div className="flex h-16 items-center gap-3 border-b px-4">
            <WorkerAvatar name={worker.name} status={worker.status} />
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold tracking-tight">{worker.name}</p>
            </div>
          </div>
          <nav className="min-h-0 flex-1 overflow-auto p-2">
            <ul className="space-y-1">
              {visibleSections.map((section) => {
                const isActive = section.key === activeSection
                const Icon = section.icon
                return (
                  <li key={section.key}>
                    <button
                      type="button"
                      onClick={() => setActiveSection(section.key)}
                      aria-current={isActive ? "page" : undefined}
                      className={cn(
                        "flex w-full items-center gap-2.5 rounded-sm px-3 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
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
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="flex h-16 items-center justify-between gap-4 border-b px-6">
            <h1 className="text-lg font-semibold tracking-tight">{t(activeLabelKey)}</h1>
            <Can perm={Perm.ContactsWrite}>
              <div className="flex shrink-0 items-center gap-2">
                <Button variant="outline" size="sm" onClick={() => setEditInfoSheetOpen(true)}>
                  <Pencil className="size-4" />
                  {t("common.edit")}
                </Button>
                <Button variant="outline" size="sm" onClick={() => navigate(`/workers/create?copy=${worker.id}`)}>
                  <Copy className="size-4" />
                  {t("common.copy")}
                </Button>
              </div>
            </Can>
          </div>

          <div className="min-w-0 flex-1 overflow-auto px-6 py-5">
            {/* Section-scoped boundary: a 403 from a cross-domain panel degrades
                only the active section, not the whole page. Keyed by section so
                switching tabs clears a prior forbidden state. */}
            <ForbiddenBoundary key={activeSection}>
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
                  <div className={cn("grid grid-cols-1 divide-y divide-border/60 border-y border-border/60 sm:divide-x sm:divide-y-0", GRID_COLS[overviewStats.length])}>
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
                  {sessionGroups.map((group) => {
                    const latest = group[0]
                    const oldest = group[group.length - 1]
                    const isRunning = latest.status === "running"
                    const intent = extractMessageContent(oldest.trigger_input)
                    const shortId = latest.session_id.slice(0, 8)

                    return (
                      <div
                        key={latest.session_id}
                        className={cn(
                          "group relative flex items-center gap-4 px-5 py-4 transition-colors hover:bg-primary/5 sm:px-6",
                          isRunning && "bg-status-working/[0.04]"
                        )}
                      >
                        {/* Whole-row click target; the copy chip below is raised above it. */}
                        <Link
                          to={`/sessions/detail?session_id=${encodeURIComponent(latest.session_id)}`}
                          aria-label={intent || t("sessions.noTriggerContent")}
                          className="absolute inset-0 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
                        />

                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-medium text-foreground">
                            {intent || t("sessions.noTriggerContent")}
                          </p>

                          <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                            <span
                              className="inline-flex items-center gap-1"
                              title={oldest.started_at ? formatTimestamp(oldest.started_at) : undefined}
                            >
                              <Clock className="size-3.5 text-muted-foreground/70" aria-hidden="true" />
                              {formatRelative(oldest.started_at, t)}
                            </span>
                            <span className="inline-flex items-center gap-1">
                              <Hash className="size-3.5 text-muted-foreground/70" aria-hidden="true" />
                              <span className="font-mono">{shortId}</span>
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
