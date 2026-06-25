import { useEffect, useMemo, useState, type ReactNode } from "react"
import { Link, useParams, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  Building2,
  CalendarIcon,
  Check,
  Clock,
  Copy,
  LayoutDashboard,
  ListTodo,
  Logs,
  Pencil,
  ScrollText,
  Settings2,
  ShieldCheck,
  X,
  type LucideIcon,
} from "lucide-react"
import { useWorker, useWorkerExecutions, useUpdateWorker } from "@/hooks/use-workers"
import { DetailSection } from "@/components/detail-primitives"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { StatusBadge } from "@/components/status-badge"
import { CopyButton } from "@/components/copy-button"
import { WorkerAvatar } from "@/components/worker-avatar"
import { EngineIcon } from "@/components/agent-icons/engine-icon"
import { FadeIn } from "@/components/fade-in"
import { SkeletonPage } from "@/components/skeleton-loader"
import { EmptyState } from "@/components/empty-state"
import { PaginationControls } from "@/components/pagination-controls"
import { TaskList } from "@/components/task-list"
import { cn } from "@/lib/utils"
import { EYEBROW_LABEL } from "@/lib/styles"
import { formatTimestamp, formatRelative, formatEngineLabel, groupExecutionsBySession, extractMessageContent } from "@/lib/format"
import type { EnvScope } from "@/lib/types"
import { ScopeToggleCard } from "@/components/scope-toggle-card"
import { KNOWN_SCOPES, parseScopes, serializeScopes, toggleScope } from "@/lib/scopes"
import { EnvConfigPanel } from "@/components/env-config-panel"
import { useEnvList, useDepartmentEnvs } from "@/hooks/use-envs"
import { CreateWorkerSheet, workerToInitialValues } from "@/components/create-worker-sheet"
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

// Presence colors mirror WorkerAvatar so the profile masthead speaks the same
// employee-presence language as the roster: green idle, purple working, red error.
const PRESENCE_COLOR: Record<string, string> = {
  idle: "bg-status-idle",
  working: "bg-status-working",
  error: "bg-status-error",
}

function profileInitials(name: string): string {
  const trimmed = name.trim()
  if (!trimmed) return "?"
  if (/[一-鿿]/.test(trimmed[0])) return trimmed.slice(0, 1)
  const parts = trimmed.split(/[\s_-]+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return trimmed.slice(0, 2).toUpperCase()
}

// Masthead avatar: larger than the roster avatar to anchor the profile, carrying
// the same presence dot. Flat fill, no shadow.
function ProfileAvatar({ name, status }: { name: string; status: string }) {
  const color = PRESENCE_COLOR[status] ?? "bg-muted-foreground"
  return (
    <span className="relative inline-flex shrink-0">
      <span className="flex size-16 select-none items-center justify-center rounded-full bg-muted text-xl font-medium text-muted-foreground ring-1 ring-border">
        {profileInitials(name)}
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

  const [isEditingConstraints, setIsEditingConstraints] = useState(false)
  const [editConstraints, setEditConstraints] = useState("")
  const updateWorker = useUpdateWorker()
  const [localScopes, setLocalScopes] = useState<string[]>([])

  useEffect(() => {
    setLocalScopes(parseScopes(worker?.permission_scopes ?? ""))
  }, [worker?.permission_scopes])

  const [copySheetOpen, setCopySheetOpen] = useState(false)
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
              {SECTIONS.map((section) => {
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
            <div className="flex shrink-0 items-center gap-2">
              <Button variant="outline" size="sm" onClick={() => setEditInfoSheetOpen(true)}>
                <Pencil className="size-4" />
                {t("common.edit")}
              </Button>
              <Button variant="outline" size="sm" onClick={() => setCopySheetOpen(true)}>
                <Copy className="size-4" />
                {t("common.copy")}
              </Button>
            </div>
          </div>

          <div className="min-w-0 flex-1 overflow-auto px-6 py-5">
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
                    <div className="py-4 sm:pr-6">
                      <div className={cn(EYEBROW_LABEL, "flex items-center gap-1.5")}>
                        <Logs className="size-3.5" />
                        <span>{t("workerDetail.sessions")}</span>
                      </div>
                      <p className="mt-2 font-mono text-lg font-medium tabular-nums text-foreground">
                        {data?.total ?? 0}
                      </p>
                    </div>

                    <div className="py-4 sm:px-6">
                      <div className={cn(EYEBROW_LABEL, "flex items-center gap-1.5")}>
                        <Clock className="size-3.5" />
                        <span>{t("workerDetail.lastActive")}</span>
                      </div>
                      <p className="mt-2 font-mono text-sm text-foreground">
                        {latestExecution ? formatTimestamp(latestExecution.started_at) : t("sessions.noExecutions")}
                      </p>
                    </div>

                    <div className="py-4 sm:px-6">
                      <div className={cn(EYEBROW_LABEL, "flex items-center gap-1.5")}>
                        <CalendarIcon className="size-3.5" />
                        <span>{t("workerDetail.created")}</span>
                      </div>
                      <p className="mt-2 font-mono text-sm text-foreground">
                        {formatTimestamp(worker.created_at)}
                      </p>
                    </div>
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
              <div className="border-b border-border/70 px-5 py-4 sm:px-6">
                <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
                  <div>
                    <p className={EYEBROW_LABEL}>
                      {t("workerDetail.sessions")}
                    </p>
                    <p className="mt-1 text-sm leading-6 text-muted-foreground">
                      {t("sessions.turnCount", { count: data?.total ?? 0 })}
                    </p>
                  </div>

                  <span className="font-mono text-xs text-muted-foreground">
                    {t("sessions.pagination.page", { page, totalPages })}
                  </span>
                </div>
              </div>

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

                          <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                            <span title={oldest.started_at ? formatTimestamp(oldest.started_at) : undefined}>
                              {formatRelative(oldest.started_at, t)}
                            </span>
                            <span aria-hidden="true" className="text-border">·</span>
                            <span>{t("sessions.turnCount", { count: group.length })}</span>
                            <span aria-hidden="true" className="text-border">·</span>
                            <span className="inline-flex items-center gap-1">
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

            <PaginationControls page={page} totalPages={totalPages} onPageChange={setPage} />
                </div>
              )}

              {activeSection === "tasks" && (
                <TaskList workerId={id!} />
              )}

              {activeSection === "constraints" && (
                <DetailSection className="p-5 sm:p-6">
              <div className="flex flex-col gap-6">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <p className={EYEBROW_LABEL}>
                      {t("workerDetail.constraints")}
                    </p>
                    <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
                      {t("workers.form.constraintsHelper")}
                    </p>
                  </div>

                  {!isEditingConstraints ? (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        setEditConstraints(worker.constraints || "")
                        setIsEditingConstraints(true)
                      }}
                      aria-label={t("workerDetail.editConstraints")}
                    >
                      <Pencil className="size-4" />
                      {t("workerDetail.editConstraints")}
                    </Button>
                  ) : null}
                </div>

                {isEditingConstraints ? (
                  <div className="space-y-3">
                    <Textarea
                      value={editConstraints}
                      onChange={(event) => setEditConstraints(event.target.value)}
                      onKeyDown={(event) => {
                        if (event.key === "Escape") {
                          setIsEditingConstraints(false)
                          setEditConstraints(worker.constraints || "")
                        }
                      }}
                      autoFocus
                      rows={12}
                      className="font-mono text-sm"
                    />

                    <div className="flex flex-wrap gap-2">
                      <Button
                        size="sm"
                        onClick={async () => {
                          await updateWorker.mutateAsync({ id: id!, data: { constraints: editConstraints } })
                          setIsEditingConstraints(false)
                        }}
                      >
                        <Check className="size-4" />
                        {t("common.save")}
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => {
                          setIsEditingConstraints(false)
                          setEditConstraints(worker.constraints || "")
                        }}
                      >
                        <X className="size-4" />
                        {t("common.cancel")}
                      </Button>
                    </div>
                  </div>
                ) : worker.constraints ? (
                  <div className="rounded-sm border border-border/70 bg-background/80 p-4">
                    <pre className="whitespace-pre-wrap break-words font-mono text-sm leading-6 text-foreground">
                      {worker.constraints}
                    </pre>
                  </div>
                ) : (
                  <div className="rounded-sm border border-dashed border-border/80 bg-background/75 px-4 py-8 text-sm leading-6 text-muted-foreground">
                    {t("workerDetail.noConstraints")}
                  </div>
                )}
              </div>
                </DetailSection>
              )}

              {activeSection === "permissions" && (
                <DetailSection className="space-y-6 p-5 sm:p-6">
              <p className={EYEBROW_LABEL}>
                {t("workerDetail.permissions")}
              </p>

              <div className="space-y-2">
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
              </div>
                </DetailSection>
              )}

              {activeSection === "env" && (
                <div className="space-y-6">
                <DetailSection className="p-5 sm:p-6 space-y-4">
              <p className={EYEBROW_LABEL}>
                {t("envConfig.title")}
              </p>
              <EnvConfigPanel scope="worker" scopeId={id!} />
            </DetailSection>

            <DetailSection className="p-5 sm:p-6 space-y-4">
              <div>
                <p className={EYEBROW_LABEL}>
                  {t("envConfig.effectiveTitle")}
                </p>
                <p className="mt-1 text-sm leading-6 text-muted-foreground">
                  {t("envConfig.effectiveHint")}
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
      <CreateWorkerSheet
        open={copySheetOpen}
        onOpenChange={setCopySheetOpen}
        initialValues={workerToInitialValues(worker)}
      />
    </FadeIn>
  )
}
