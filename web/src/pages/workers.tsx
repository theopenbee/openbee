import { useEffect, useMemo, useRef, useState } from "react"
import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { CopyIcon, EllipsisIcon, EyeIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { useWorkers, useDeleteWorker } from "@/hooks/use-workers"
import { useFlatDepartments } from "@/hooks/use-departments"
import { formatEngineLabel, formatRelative, formatTimestamp } from "@/lib/format"
import { getErrorMessage } from "@/lib/utils"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Checkbox } from "@/components/arc/checkbox/checkbox"
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog"
import { DropdownMenu } from "@/components/arc/dropdown-menu/dropdown-menu"
import { Input } from "@/components/arc/input/input"
import { SearchField } from "@/components/arc/search-field/search-field"
import SegmentedControl from "@/components/arc/segmented-control/segmented-control"
import { Select } from "@/components/arc/select/select"
import { EngineIcon } from "@/components/agent-icons/engine-icon"
import { EmptyState } from "@/components/empty-state"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { SkeletonTable } from "@/components/skeleton-loader"
import { StatusBadge } from "@/components/status-badge"
import { WorkerAvatar } from "@/components/worker-avatar"
import { WorkerLink } from "@/components/worker-link"
import type { Worker } from "@/lib/types"

const ALL = "all"
const UNGROUPED = "ungrouped"
const STATUSES = ["working", "idle", "error"] as const

type DeleteTarget = Pick<Worker, "id" | "name" | "work_dir">

const TH = "whitespace-nowrap px-4 py-3 text-left text-xs font-normal text-secondary sm:px-6"
const TD = "px-4 py-4 align-middle sm:px-6"

function DepartmentCell({ worker }: { worker: Worker }) {
  const { t } = useTranslation()
  const departments = worker.departments ?? []
  if (departments.length === 0) return <span className="text-muted">{t("departments.ungrouped")}</span>
  const [first, ...rest] = departments
  return (
    <span className="flex min-w-0 items-center gap-1.5" title={departments.map((d) => d.name).join(", ")}>
      <span className="truncate text-secondary">{first.name}</span>
      {rest.length > 0 && <span className="shrink-0 text-muted tabular">+{rest.length}</span>}
    </span>
  )
}

function DeleteWorkerDialog({ target, onClose }: { target: DeleteTarget | null; onClose: () => void }) {
  const { t } = useTranslation()
  const deleteWorker = useDeleteWorker()
  const { reset } = deleteWorker
  const inputRef = useRef<HTMLInputElement>(null)
  const [confirmation, setConfirmation] = useState("")
  const [deleteWorkDir, setDeleteWorkDir] = useState(false)
  const [lastTarget, setLastTarget] = useState(target)
  if (target && target !== lastTarget) {
    setLastTarget(target)
    setConfirmation("")
    setDeleteWorkDir(false)
  }
  useEffect(() => {
    if (target) reset()
  }, [target, reset])
  const shown = target ?? lastTarget
  const confirmed = !!shown && confirmation === shown.name

  const handleConfirm = async () => {
    if (!shown || !confirmed) return
    try {
      await deleteWorker.mutateAsync({ id: shown.id, deleteWorkDir })
      onClose()
    } catch {
      return
    }
  }

  return (
    <Dialog open={!!target} onOpenChange={(open) => { if (!open && !deleteWorker.isPending) onClose() }}>
      <DialogContent
        title={t("workers.deleteDialog.title", { name: shown?.name ?? "" })}
        description={t("workers.deleteDialog.description")}
        onOpenAutoFocus={(event) => {
          event.preventDefault()
          inputRef.current?.focus()
        }}
      >
        <form
          className="grid gap-4"
          onSubmit={(event) => {
            event.preventDefault()
            void handleConfirm()
          }}
        >
          <Input
            ref={inputRef}
            label={t("workers.deleteDialog.confirmNameLabel", { name: shown?.name ?? "" })}
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
            placeholder={t("workers.deleteDialog.confirmNamePlaceholder")}
            autoComplete="off"
            spellCheck={false}
            disabled={deleteWorker.isPending}
          />
          <Checkbox
            label={t("workers.deleteDialog.deleteWorkDir")}
            description={shown?.work_dir ? t("workers.deleteDialog.deleteWorkDirHint", { path: shown.work_dir }) : undefined}
            checked={deleteWorkDir}
            onCheckedChange={(state) => setDeleteWorkDir(state === true)}
            disabled={deleteWorker.isPending}
          />
          {deleteWorker.error && (
            <Alert tone="danger" title={t("workers.deleteDialog.error")}>
              {getErrorMessage(deleteWorker.error)}
            </Alert>
          )}
          <div className="mt-2 flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={onClose} disabled={deleteWorker.isPending}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="danger" loading={deleteWorker.isPending} disabled={!confirmed}>
              {t("workers.deleteDialog.confirm")}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function Workers() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const flatDepartments = useFlatDepartments()
  const [departmentFilter, setDepartmentFilter] = useState<string>(ALL)
  const [statusFilter, setStatusFilter] = useState<string>(ALL)
  const [query, setQuery] = useState("")
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null)

  const departmentExists = flatDepartments.some(({ dept }) => dept.id === departmentFilter)
  const activeDepartment =
    departmentFilter === UNGROUPED || departmentExists ? departmentFilter : ALL
  const serverDepartment =
    activeDepartment === ALL || activeDepartment === UNGROUPED ? undefined : activeDepartment
  const { data: workers = [], error, isLoading, refetch } = useWorkers(serverDepartment)

  const departmentOptions = useMemo(() => {
    const names = new Map(flatDepartments.map(({ dept }) => [dept.id, dept.name]))
    const parents = new Map(flatDepartments.map(({ dept }) => [dept.id, dept.parent_id]))
    const path = (id: string): string => {
      const parent = parents.get(id)
      const name = names.get(id) ?? id
      return parent && names.has(parent) ? `${path(parent)} / ${name}` : name
    }
    return [
      { value: ALL, label: t("workers.filters.allDepartments") },
      { value: UNGROUPED, label: t("departments.ungrouped") },
      ...flatDepartments.map(({ dept }) => ({ value: dept.id, label: path(dept.id) })),
    ]
  }, [flatDepartments, t])

  const inDepartment = useMemo(
    () =>
      activeDepartment === UNGROUPED
        ? workers.filter((w) => !w.departments || w.departments.length === 0)
        : workers,
    [workers, activeDepartment],
  )

  const statusOptions = useMemo(() => {
    const count = (status: string) => inDepartment.filter((w) => w.status === status).length
    const accessory = (n: number) => <span className="text-muted tabular">{n}</span>
    return [
      { value: ALL, label: t("workers.filters.allStatuses"), accessory: accessory(inDepartment.length) },
      ...STATUSES.map((status) => ({
        value: status,
        label: t(`statuses.${status}`),
        accessory: accessory(count(status)),
      })),
    ]
  }, [inDepartment, t])

  const needle = query.trim().toLowerCase()
  const displayed = inDepartment.filter(
    (w) =>
      (statusFilter === ALL || w.status === statusFilter) &&
      (!needle ||
        w.name.toLowerCase().includes(needle) ||
        (w.description ?? "").toLowerCase().includes(needle)),
  )

  const activeCount = inDepartment.filter((w) => w.status === "working").length
  const hasLocalFilters = needle !== "" || statusFilter !== ALL
  const hasAnyFilter = hasLocalFilters || activeDepartment !== ALL

  const clearFilters = () => {
    setQuery("")
    setStatusFilter(ALL)
    setDepartmentFilter(ALL)
  }

  const createButton = (
    <Button onClick={() => navigate("/workers/create")}>
      <PlusIcon size={16} strokeWidth={1.75} aria-hidden="true" />
      {t("workers.createWorker")}
    </Button>
  )

  const renderBody = () => {
    if (error) {
      return (
        <Alert tone="danger" title={t("workers.loadError")}>
          <span className="grid justify-items-start gap-3">
            <span>{getErrorMessage(error)}</span>
            <Button variant="secondary" size="sm" onClick={() => refetch()}>
              {t("common.retry")}
            </Button>
          </span>
        </Alert>
      )
    }
    if (isLoading) return <SkeletonTable rows={6} columns={5} />
    if (displayed.length === 0) {
      return (
        <div className="rounded-surface border border-border bg-surface">
          {hasAnyFilter ? (
            <EmptyState
              title={hasLocalFilters ? t("workers.noMatches") : t("emptyState.noWorkersInGroup")}
              description={hasLocalFilters ? t("workers.noMatchesDesc") : t("emptyState.noWorkersInGroupDesc")}
              action={
                <Button variant="secondary" onClick={clearFilters}>
                  {t("workers.filters.clear")}
                </Button>
              }
            />
          ) : (
            <EmptyState
              title={t("emptyState.noWorkers")}
              description={t("emptyState.noWorkersDesc")}
              action={createButton}
            />
          )}
        </div>
      )
    }

    return (
      <div className="overflow-hidden rounded-surface border border-border bg-surface">
        <div className="overflow-x-auto overscroll-x-contain">
          <table className="w-full text-sm">
            <caption className="sr-only">{t("workers.title")}</caption>
            <thead>
              <tr>
                <th scope="col" className={TH}>{t("workers.columns.name")}</th>
                <th scope="col" className={`${TH} hidden md:table-cell`}>{t("workers.columns.engine")}</th>
                <th scope="col" className={`${TH} hidden sm:table-cell`}>{t("workers.columns.status")}</th>
                <th scope="col" className={`${TH} hidden lg:table-cell`}>{t("workers.columns.department")}</th>
                <th scope="col" className={`${TH} hidden xl:table-cell`}>{t("workers.columns.activeTime")}</th>
                <th scope="col" className={`${TH} w-16`}>
                  <span className="sr-only">{t("workers.columns.actions")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {displayed.map((w) => (
                <tr key={w.id} className="border-t border-border-subtle transition-colors hover:bg-surface-muted">
                  <td className={TD}>
                    <div className="flex min-w-0 items-start gap-3 sm:items-center">
                      <WorkerAvatar workerId={w.id} name={w.name} status={w.status} />
                      <div className="grid min-w-0 gap-0.5">
                        <WorkerLink
                          id={w.id}
                          name={w.name}
                          className="truncate font-medium text-foreground hover:text-accent-strong"
                        />
                        <p className="line-clamp-1 max-w-xl text-sm text-secondary">
                          {w.description || t("common.noDescription")}
                        </p>
                        <div className="pt-1 sm:hidden">
                          <StatusBadge status={w.status} />
                        </div>
                      </div>
                    </div>
                  </td>
                  <td className={`${TD} hidden whitespace-nowrap md:table-cell`}>
                    {w.engine ? (
                      <span className="inline-flex items-center gap-2 text-foreground">
                        <EngineIcon engine={w.engine} className="size-4" />
                        {formatEngineLabel(w.engine, t)}
                      </span>
                    ) : (
                      <span className="text-muted">{t("workers.noEngine")}</span>
                    )}
                  </td>
                  <td className={`${TD} hidden whitespace-nowrap sm:table-cell`}>
                    <StatusBadge status={w.status} />
                  </td>
                  <td className={`${TD} hidden max-w-48 lg:table-cell`}>
                    <DepartmentCell worker={w} />
                  </td>
                  <td className={`${TD} hidden whitespace-nowrap text-secondary tabular xl:table-cell`}>
                    <span title={w.updated_at ? formatTimestamp(w.updated_at) : undefined}>
                      {formatRelative(w.updated_at, t)}
                    </span>
                  </td>
                  <td className={`${TD} text-right`}>
                    <DropdownMenu
                      iconOnly
                      icon={<EllipsisIcon size={16} strokeWidth={1.75} />}
                      label={t("workers.rowActions", { name: w.name })}
                      items={[
                        {
                          label: t("common.view"),
                          icon: <EyeIcon size={16} strokeWidth={1.75} />,
                          onSelect: () => navigate(`/workers/${w.id}`),
                        },
                        {
                          label: t("workers.copyWorker"),
                          icon: <CopyIcon size={16} strokeWidth={1.75} />,
                          onSelect: () => navigate(`/workers/create?copy=${w.id}`),
                        },
                        {
                          label: t("workers.deleteWorker"),
                          icon: <Trash2Icon size={16} strokeWidth={1.75} />,
                          destructive: true,
                          separatorBefore: true,
                          onSelect: () => setDeleteTarget({ id: w.id, name: w.name, work_dir: w.work_dir }),
                        },
                      ]}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    )
  }

  return (
    <FadeIn className="min-h-0 flex-1 overflow-y-auto">
      <div className="mx-auto w-full max-w-page px-4 py-6 sm:px-6 md:px-8 md:py-8">
        <PageHeader
          title={t("workers.title")}
          subtitle={
            !isLoading && !error
              ? t("workers.summary", { count: inDepartment.length, active: activeCount })
              : undefined
          }
          actions={createButton}
        />

        <div className="mb-6 flex flex-wrap items-end gap-3">
          <div className="w-full sm:w-72">
            <SearchField
              label={t("workers.searchLabel")}
              placeholder={t("workers.searchPlaceholder")}
              value={query}
              onValueChange={setQuery}
            />
          </div>
          <div className="w-full sm:w-60">
            <Select
              label={t("workers.filters.department")}
              options={departmentOptions}
              value={activeDepartment}
              onValueChange={setDepartmentFilter}
            />
          </div>
          <div className="max-w-full lg:ml-auto">
            <SegmentedControl
              label={t("workers.filters.status")}
              options={statusOptions}
              value={statusFilter}
              onValueChange={setStatusFilter}
            />
          </div>
        </div>

        {renderBody()}
      </div>

      <DeleteWorkerDialog target={deleteTarget} onClose={() => setDeleteTarget(null)} />
    </FadeIn>
  )
}
