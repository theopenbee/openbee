import { useEffect, useMemo, useState, type ReactNode } from "react"
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  ArrowLeft,
  Building2,
  CalendarIcon,
  Copy,
  History,
  LayoutDashboard,
  Pencil,
  ScrollText,
  Settings2,
  ShieldCheck,
  type LucideIcon,
} from "lucide-react"
import { useWorker, useUpdateWorker } from "@/hooks/use-workers"
import { Alert } from "@/components/arc/alert/alert"
import { Badge } from "@/components/arc/badge/badge"
import { Button } from "@/components/arc/button/button"
import { DetailOverviewStat, DetailSection } from "@/components/detail-primitives"
import { CopyButton } from "@/components/copy-button"
import { WorkerAvatar } from "@/components/worker-avatar"
import { EngineIcon } from "@/components/agent-icons/engine-icon"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { Panel } from "@/components/panel"
import { StatusBadge } from "@/components/status-badge"
import { WorkerConstraintsPanel } from "@/components/worker-constraints-panel"
import { WorkerEffectiveEnv } from "@/components/worker-effective-env"
import { ScopeToggleCard } from "@/components/scope-toggle-card"
import { EnvConfigPanel } from "@/components/env-config-panel"
import { EditWorkerInfoSheet } from "@/components/edit-worker-info-sheet"
import { cn, getErrorMessage } from "@/lib/utils"
import { formatEngineLabel, formatRelative, formatTimestamp } from "@/lib/format"
import { KNOWN_SCOPES, parseScopes, serializeScopes, toggleScope } from "@/lib/scopes"
import type { Worker } from "@/lib/types"

const SECTIONS = [
  { key: "overview", labelKey: "workerDetail.overview", icon: LayoutDashboard },
  { key: "constraints", labelKey: "workerDetail.constraints", icon: ScrollText },
  { key: "permissions", labelKey: "workerDetail.permissions", icon: ShieldCheck },
  { key: "env", labelKey: "envConfig.title", icon: Settings2 },
] satisfies ReadonlyArray<{ key: string; labelKey: string; icon: LucideIcon }>

type SectionKey = (typeof SECTIONS)[number]["key"]

const PAGE_CONTAINER = "mx-auto w-full max-w-page px-4 py-6 sm:px-6 md:px-8 md:py-8"

const revealTab = (el: HTMLButtonElement | null) => {
  el?.scrollIntoView({ block: "nearest", inline: "nearest" })
}

function BackLink() {
  const { t } = useTranslation()
  return (
    <Link
      to="/workers"
      aria-label={t("workers.backToList")}
      className="inline-flex h-9 items-center gap-2 rounded-control px-3 text-sm text-secondary transition-colors hover:bg-surface-muted hover:text-foreground"
    >
      <ArrowLeft size={16} strokeWidth={1.75} aria-hidden="true" />
      {t("workers.title")}
    </Link>
  )
}

function SectionNav({ active, onSelect }: { active: SectionKey; onSelect: (key: SectionKey) => void }) {
  const { t } = useTranslation()
  return (
    <nav aria-label={t("workerDetail.sectionNav")} className="min-w-0 scroll-px-4 overflow-x-auto overscroll-x-contain [scrollbar-width:none] sm:scroll-px-6 lg:overflow-visible">
      <ul className="flex w-max gap-1 px-4 pb-3 sm:px-6 lg:grid lg:w-auto lg:gap-0.5 lg:px-3 lg:pb-0">
        {SECTIONS.map((section) => {
          const isActive = section.key === active
          const Icon = section.icon
          return (
            <li key={section.key}>
              <button
                ref={isActive ? revealTab : undefined}
                type="button"
                onClick={() => onSelect(section.key)}
                aria-current={isActive ? "page" : undefined}
                className={cn(
                  "flex h-10 w-full min-w-0 items-center gap-3 whitespace-nowrap rounded-control px-3 text-sm transition-colors duration-150 lg:h-9",
                  isActive
                    ? "bg-accent-subtle text-foreground dark:bg-surface-muted"
                    : "text-secondary hover:bg-surface-muted hover:text-foreground",
                )}
              >
                <Icon
                  size={16}
                  strokeWidth={1.75}
                  aria-hidden="true"
                  className={cn("shrink-0", isActive && "text-accent-strong")}
                />
                <span className="truncate">{t(section.labelKey)}</span>
              </button>
            </li>
          )
        })}
      </ul>
    </nav>
  )
}

function InfoRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-1 border-t border-border-subtle py-3 first:border-t-0 sm:min-h-14 sm:grid-cols-[10rem_minmax(0,1fr)] sm:items-center sm:gap-6">
      <dt className="text-sm text-secondary">{label}</dt>
      <dd className="min-w-0 text-sm text-foreground">{children}</dd>
    </div>
  )
}

function MonoValue({ value, copyLabel }: { value: string; copyLabel: string }) {
  return (
    <span className="flex min-w-0 items-center gap-1">
      <span className="min-w-0 break-all font-mono text-sm text-foreground">{value}</span>
      <CopyButton value={value} label={copyLabel} className="shrink-0" />
    </span>
  )
}

function Overview({ worker }: { worker: Worker }) {
  const { t } = useTranslation()
  const departments = worker.departments ?? []
  return (
    <div className="grid gap-6">
      <DetailSection className="p-6">
        <div className="flex min-w-0 items-start gap-4">
          <WorkerAvatar name={worker.name} status={worker.status} size="lg" />
          <div className="grid min-w-0 flex-1 gap-1.5">
            <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
              <h2 className="min-w-0 truncate text-xl font-medium text-foreground">{worker.name}</h2>
              <StatusBadge status={worker.status} />
            </div>
            <p className="max-w-3xl text-sm text-secondary">
              {worker.description || t("common.noDescription")}
            </p>
          </div>
        </div>
        <div className="mt-6 grid grid-cols-2 gap-3">
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
      </DetailSection>

      <Panel title={t("workerDetail.workerInfo")}>
        <dl>
          <InfoRow label={t("workerDetail.id")}>
            <MonoValue value={worker.id} copyLabel={t("workerDetail.copyId")} />
          </InfoRow>
          <InfoRow label={t("workers.form.engine")}>
            {worker.engine ? (
              <span className="inline-flex items-center gap-2">
                <EngineIcon engine={worker.engine} className="size-4" />
                {formatEngineLabel(worker.engine, t)}
              </span>
            ) : (
              <span className="text-secondary">{t("workers.noEngine")}</span>
            )}
          </InfoRow>
          <InfoRow label={t("departments.title")}>
            {departments.length > 0 ? (
              <span className="flex flex-wrap items-center gap-1.5">
                {departments.map((d) => (
                  <Badge key={d.id} size="sm" icon={<Building2 size={12} strokeWidth={1.75} />}>
                    {d.name}
                  </Badge>
                ))}
              </span>
            ) : (
              <span className="text-secondary">{t("departments.ungrouped")}</span>
            )}
          </InfoRow>
          <InfoRow label={t("workerDetail.workDir")}>
            {worker.work_dir ? (
              <MonoValue value={worker.work_dir} copyLabel={t("workerDetail.copyWorkDir")} />
            ) : (
              <span className="text-secondary">{t("workerDetail.defaultWorkDir")}</span>
            )}
          </InfoRow>
        </dl>
      </Panel>
    </div>
  )
}

function DetailSkeleton() {
  return (
    <div className="flex min-h-0 flex-1 flex-col lg:flex-row" aria-busy="true">
      <aside className="shrink-0 border-b border-border lg:w-60 lg:border-r lg:border-b-0">
        <div className="flex items-center gap-3 px-4 py-4 sm:px-6 lg:px-6 lg:pt-6">
          <div className="skeleton size-9 rounded-full" />
          <div className="grid flex-1 gap-2">
            <div className="skeleton h-4 w-24" />
            <div className="skeleton h-3 w-16" />
          </div>
        </div>
        <div className="hidden gap-2 px-3 pt-2 lg:grid">
          {SECTIONS.map((s) => (
            <div key={s.key} className="skeleton h-9 w-full rounded-control" />
          ))}
        </div>
      </aside>
      <div className="min-w-0 flex-1">
        <div className={PAGE_CONTAINER}>
          <div className="skeleton mb-8 h-9 w-40" />
          <div className="grid gap-6">
            <div className="skeleton h-48 w-full rounded-surface" />
            <div className="skeleton h-64 w-full rounded-surface" />
          </div>
        </div>
      </div>
    </div>
  )
}

export function WorkerDetail() {
  const { t } = useTranslation()
  const navigate = useNavigate()
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

  const updateWorker = useUpdateWorker()
  const [localScopes, setLocalScopes] = useState<string[]>([])
  const [scopeError, setScopeError] = useState("")

  useEffect(() => {
    setLocalScopes(parseScopes(worker?.permission_scopes ?? ""))
  }, [worker?.permission_scopes])

  const [editInfoSheetOpen, setEditInfoSheetOpen] = useState(false)
  const workerDeptIds = useMemo(
    () => worker?.departments?.map((d) => d.id).sort() ?? [],
    [worker?.departments],
  )

  if (workerError && !worker) {
    return (
      <FadeIn className="min-h-0 flex-1 overflow-y-auto">
        <div className={PAGE_CONTAINER}>
          <div className="mb-6 -ml-3">
            <BackLink />
          </div>
          <Alert tone="danger" title={t("workerDetail.loadError")}>
            <span className="grid justify-items-start gap-3">
              <span>{getErrorMessage(workerError)}</span>
              <Button variant="secondary" size="sm" onClick={() => refetchWorker()}>
                {t("common.retry")}
              </Button>
            </span>
          </Alert>
        </div>
      </FadeIn>
    )
  }

  if (!worker) return <DetailSkeleton />

  const toggleScopeValue = (scopeId: string, val: boolean) => {
    const prevScopes = localScopes
    const newScopes = toggleScope(localScopes, scopeId, val)
    setLocalScopes(newScopes)
    setScopeError("")
    updateWorker.mutate(
      { id: id!, data: { permission_scopes: serializeScopes(newScopes) } },
      {
        onError: (err) => {
          setLocalScopes(prevScopes)
          setScopeError(getErrorMessage(err))
        },
      },
    )
  }

  const headerActions = (
    <>
      <Button variant="secondary" onClick={() => setEditInfoSheetOpen(true)}>
        <Pencil size={16} strokeWidth={1.75} aria-hidden="true" />
        {t("workerDetail.editInfo")}
      </Button>
      <Button variant="secondary" onClick={() => navigate(`/workers/create?copy=${worker.id}`)}>
        <Copy size={16} strokeWidth={1.75} aria-hidden="true" />
        {t("workers.copyWorker")}
      </Button>
    </>
  )

  return (
    <FadeIn className="flex min-h-0 flex-1 flex-col lg:flex-row">
      <aside className="shrink-0 border-b border-border lg:flex lg:w-60 lg:flex-col lg:border-r lg:border-b-0">
        <div className="hidden px-3 pt-4 lg:block">
          <BackLink />
        </div>
        <div className="flex min-w-0 items-center gap-3 px-4 py-4 sm:px-6 lg:px-6 lg:pt-4 lg:pb-5">
          <WorkerAvatar name={worker.name} status={worker.status} />
          <div className="grid min-w-0 flex-1">
            <p className="truncate text-sm font-medium text-foreground">{worker.name}</p>
            <p className="truncate text-xs text-secondary">
              {worker.engine ? formatEngineLabel(worker.engine, t) : t("workers.noEngine")}
            </p>
          </div>
        </div>
        <SectionNav active={activeSection} onSelect={setActiveSection} />
      </aside>

      <div className="min-h-0 min-w-0 flex-1 overflow-y-auto">
        <div className={PAGE_CONTAINER}>
          <PageHeader title={t(activeLabelKey)} actions={headerActions} />

          {activeSection === "overview" && (
            <Overview worker={worker} />
          )}

          {activeSection === "constraints" && <WorkerConstraintsPanel worker={worker} />}

          {activeSection === "permissions" && (
            <section className="grid max-w-form gap-4" aria-label={t("workerDetail.permissions")}>
              <p className="text-sm text-secondary">{t("workers.form.permissionsHelper")}</p>
              {scopeError && (
                <Alert tone="danger" title={t("workerDetail.permissionsSaveError")}>
                  {scopeError}
                </Alert>
              )}
              <DetailSection className="divide-y divide-border-subtle">
                {KNOWN_SCOPES.map((scope) => (
                  <ScopeToggleCard
                    key={scope.id}
                    scope={scope}
                    checked={localScopes.includes(scope.id)}
                    onToggle={toggleScopeValue}
                    disabled={updateWorker.isPending}
                  />
                ))}
              </DetailSection>
            </section>
          )}

          {activeSection === "env" && (
            <div className="grid gap-6">
              <DetailSection className="p-6">
                <EnvConfigPanel scope="worker" scopeId={id!} title={t("envConfig.workerTitle")} inset />
              </DetailSection>
              <WorkerEffectiveEnv workerId={id!} departmentIds={workerDeptIds} />
            </div>
          )}
        </div>
      </div>

      <EditWorkerInfoSheet open={editInfoSheetOpen} onOpenChange={setEditInfoSheetOpen} worker={worker} />
    </FadeIn>
  )
}
