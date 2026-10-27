import { useMemo, useRef, useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { Link } from "react-router-dom"
import { useReducedMotion } from "motion/react"
import { EllipsisIcon, FolderTreeIcon, PencilIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { useCreateDepartment, useDeleteDepartment, useDepartments, useUpdateDepartment } from "@/hooks/use-departments"
import { useWorkers } from "@/hooks/use-workers"
import { flattenDeptTree } from "@/lib/department-utils"
import { formatRelative, formatTimestamp } from "@/lib/format"
import { conversationPath, workerConversationId } from "@/lib/conversations"
import { getErrorMessage } from "@/lib/utils"
import { useCloseSettingsDialog } from "@/hooks/use-settings-dialog"
import { Panel } from "@/components/panel"
import { FadeIn } from "@/components/fade-in"
import { EmptyState } from "@/components/empty-state"
import { SkeletonCard } from "@/components/skeleton-loader"
import { DetailOverviewStat, DetailSection } from "@/components/detail-primitives"
import { StatusBadge } from "@/components/status-badge"
import { WorkerAvatar } from "@/components/worker-avatar"
import { EnvConfigPanel } from "@/components/env-config-panel"
import { DepartmentTree } from "@/components/department-tree"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog"
import { DropdownMenu } from "@/components/arc/dropdown-menu/dropdown-menu"
import { Input } from "@/components/arc/input/input"
import { Select } from "@/components/arc/select/select"
import type { Department, DepartmentTree as DepartmentNode } from "@/lib/types"

const NO_PARENT_VALUE = "__no_parent__"

type FormMode = "create" | "edit"

interface FormState {
  mode: FormMode
  target: DepartmentNode | null
  name: string
  parentId: string | null
}

interface DeleteState {
  target: DepartmentNode
  workers: number | null
}

function indexTree(nodes: DepartmentNode[]) {
  const map = new Map<string, DepartmentNode>()
  const walk = (list: DepartmentNode[]) =>
    list.forEach((node) => {
      map.set(node.id, node)
      walk(node.children)
    })
  walk(nodes)
  return map
}

function subtreeIds(node: DepartmentNode): Set<string> {
  const ids = new Set<string>([node.id])
  const walk = (list: DepartmentNode[]) =>
    list.forEach((child) => {
      ids.add(child.id)
      walk(child.children)
    })
  walk(node.children)
  return ids
}

export function DepartmentsSettings() {
  const { t } = useTranslation()
  const reduced = useReducedMotion()
  const departmentsQuery = useDepartments()
  const departments = useMemo(() => departmentsQuery.data ?? [], [departmentsQuery.data])
  const createDept = useCreateDepartment()
  const updateDept = useUpdateDepartment()
  const deleteDept = useDeleteDepartment()

  const flat = useMemo(() => flattenDeptTree(departments), [departments])
  const nodes = useMemo(() => indexTree(departments), [departments])

  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [form, setForm] = useState<FormState | null>(null)
  const [formError, setFormError] = useState("")
  const [deleting, setDeleting] = useState<DeleteState | null>(null)
  const [deleteError, setDeleteError] = useState("")
  const detailsRef = useRef<HTMLDivElement>(null)

  const selected = (selectedId ? nodes.get(selectedId) : undefined) ?? departments[0] ?? null

  const pathOf = (dept: Department): string[] => {
    const names: string[] = []
    let parentId = dept.parent_id
    while (parentId) {
      const parent = nodes.get(parentId)
      if (!parent) break
      names.unshift(parent.name)
      parentId = parent.parent_id
    }
    return names
  }

  const select = (id: string) => {
    setSelectedId(id)
    requestAnimationFrame(() =>
      detailsRef.current?.scrollIntoView({ behavior: reduced ? "auto" : "smooth", block: "start" }),
    )
  }

  const openCreate = (parentId: string | null = null) => {
    setFormError("")
    setForm({ mode: "create", target: null, name: "", parentId })
  }

  const openEdit = (dept: DepartmentNode) => {
    setFormError("")
    setForm({ mode: "edit", target: dept, name: dept.name, parentId: dept.parent_id })
  }

  const openDelete = (dept: DepartmentNode, workers: number | null) => {
    setDeleteError("")
    setDeleting({ target: dept, workers })
  }

  const submitForm = async (event: FormEvent) => {
    event.preventDefault()
    if (!form) return
    const name = form.name.trim()
    if (!name) return
    try {
      if (form.mode === "create") {
        const created = await createDept.mutateAsync({ name, parent_id: form.parentId })
        if (created?.id) setSelectedId(created.id)
      } else if (form.target) {
        await updateDept.mutateAsync({ id: form.target.id, data: { name, parent_id: form.parentId } })
      }
      setForm(null)
    } catch (err) {
      setFormError(getErrorMessage(err))
    }
  }

  const confirmDelete = async () => {
    if (!deleting) return
    try {
      await deleteDept.mutateAsync(deleting.target.id)
      if (selected?.id === deleting.target.id) setSelectedId(deleting.target.parent_id)
      setDeleting(null)
    } catch (err) {
      setDeleteError(getErrorMessage(err))
    }
  }

  const excludedParents = form?.target ? subtreeIds(form.target) : new Set<string>()
  const parentOptions = [
    { value: NO_PARENT_VALUE, label: t("departments.form.noParent") },
    ...flat
      .filter(({ dept }) => !excludedParents.has(dept.id))
      .map(({ dept }) => ({ value: dept.id, label: [...pathOf(dept), dept.name].join(" / ") })),
  ]

  const createButton = (
    <Button onClick={() => openCreate()}>
      <PlusIcon size={16} strokeWidth={1.75} aria-hidden="true" />
      {t("departments.create")}
    </Button>
  )

  const formParent = form?.parentId ? nodes.get(form.parentId) : undefined
  const formTitle =
    form?.mode === "edit"
      ? t("departments.form.editTitle")
      : form?.parentId
        ? t("departments.form.createChildTitle")
        : t("departments.create")
  const formDescription =
    form?.mode === "edit"
      ? t("departments.form.editDescription", { name: form.target?.name ?? "" })
      : formParent
        ? t("departments.form.createChildDescription", { name: formParent.name })
        : t("departments.form.createDescription")
  const saving = createDept.isPending || updateDept.isPending

  const deleteBlocker = deleting
    ? deleting.target.children.length > 0
      ? t("departments.deleteDialog.hasChildren", { count: deleting.target.children.length })
      : deleting.workers
        ? t("departments.deleteDialog.hasWorkers", { count: deleting.workers })
        : ""
    : ""

  const isEmpty = !departmentsQuery.isLoading && !departmentsQuery.error && !selected

  const renderBody = () => {
    if (departmentsQuery.isLoading) {
      return (
        <div className="grid gap-6 @3xl:grid-cols-[18rem_minmax(0,1fr)]" aria-busy="true">
          <SkeletonCard className="h-80" />
          <div className="grid content-start gap-6">
            <SkeletonCard />
            <SkeletonCard />
          </div>
        </div>
      )
    }
    if (departmentsQuery.error) {
      return (
        <Alert tone="danger" title={t("departments.loadError")}>
          <span className="mt-1 flex flex-wrap items-center gap-3">
            <span className="wrap-anywhere">{getErrorMessage(departmentsQuery.error)}</span>
            <Button variant="secondary" size="sm" onClick={() => departmentsQuery.refetch()}>
              {t("common.retry")}
            </Button>
          </span>
        </Alert>
      )
    }
    if (!selected) {
      return (
        <div className="rounded-surface border border-border bg-surface">
          <EmptyState
            icon={<FolderTreeIcon size={24} strokeWidth={1.5} />}
            title={t("departments.emptyTitle")}
            description={t("departments.emptyDescription")}
            action={createButton}
          />
        </div>
      )
    }
    return (
      <div className="grid gap-6 @3xl:grid-cols-[18rem_minmax(0,1fr)] @3xl:items-start">
        <div className="min-w-0 @3xl:sticky @3xl:top-6">
          <Panel
            title={t("departments.tree.title")}
            action={<span className="text-xs text-secondary tabular">{flat.length}</span>}
            flush
          >
            <div className="px-3 @3xl:max-h-[calc(min(100dvh-2rem,52rem)-11rem)] @3xl:overflow-y-auto">
              <DepartmentTree
                departments={departments}
                selectedId={selected.id}
                onSelect={select}
                label={t("departments.tree.title")}
              />
            </div>
          </Panel>
        </div>
        <div ref={detailsRef} className="@container min-w-0 scroll-mt-6">
          <DepartmentDetails
            key={selected.id}
            node={selected}
            path={pathOf(selected)}
            onEdit={() => openEdit(selected)}
            onAddChild={() => openCreate(selected.id)}
            onDelete={(workers) => openDelete(selected, workers)}
          />
        </div>
      </div>
    )
  }

  return (
    <FadeIn>
      {!isEmpty && (
        <div className="mb-6 flex items-center gap-3">
          <p className="min-w-0 flex-1 text-sm text-secondary">
            {flat.length > 0 ? t("departments.summary", { count: flat.length }) : null}
          </p>
          {createButton}
        </div>
      )}

      {renderBody()}

      <Dialog open={form !== null} onOpenChange={(open) => !open && setForm(null)}>
        <DialogContent title={formTitle} description={formDescription}>
          <form onSubmit={submitForm} className="grid gap-5">
            <Input
              label={t("departments.form.name")}
              value={form?.name ?? ""}
              onChange={(e) => setForm((prev) => (prev ? { ...prev, name: e.target.value } : prev))}
              placeholder={t("departments.form.namePlaceholder")}
              required
              autoFocus
            />
            <Select
              label={t("departments.form.parent")}
              options={parentOptions}
              value={form?.parentId ?? NO_PARENT_VALUE}
              onValueChange={(value) =>
                setForm((prev) => (prev ? { ...prev, parentId: value === NO_PARENT_VALUE ? null : value } : prev))
              }
            />
            {formError && (
              <Alert tone="danger" title={t("departments.form.saveError")}>
                <span className="wrap-anywhere">{formError}</span>
              </Alert>
            )}
            <div className="mt-1 flex justify-end gap-2">
              <Button type="button" variant="secondary" onClick={() => setForm(null)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" loading={saving} disabled={!form?.name.trim()}>
                {form?.mode === "edit" ? t("departments.form.save") : t("departments.create")}
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <DialogContent
          title={t("departments.deleteDialog.title", { name: deleting?.target.name ?? "" })}
          description={t("departments.deleteDialog.description")}
        >
          <div className="grid gap-4">
            {deleteBlocker && (
              <Alert tone="warning" title={t("departments.deleteDialog.blockedTitle")}>
                {deleteBlocker}
              </Alert>
            )}
            {deleteError && (
              <Alert tone="danger" title={t("departments.deleteDialog.error")}>
                <span className="wrap-anywhere">{deleteError}</span>
              </Alert>
            )}
          </div>
          <div className="mt-6 flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              {t("common.cancel")}
            </Button>
            <Button variant="danger" loading={deleteDept.isPending} disabled={Boolean(deleteBlocker)} onClick={confirmDelete}>
              {t("departments.deleteDialog.confirm")}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </FadeIn>
  )
}

interface DepartmentDetailsProps {
  node: DepartmentNode
  path: string[]
  onEdit: () => void
  onAddChild: () => void
  onDelete: (workers: number | null) => void
}

function DepartmentDetails({ node, path, onEdit, onAddChild, onDelete }: DepartmentDetailsProps) {
  const { t } = useTranslation()
  const workers = useWorkers(node.id)
  const workerCount = workers.data ? workers.data.length : null
  const closeSettings = useCloseSettingsDialog()

  return (
    <div className="grid gap-6">
      <Panel
        title={node.name}
        description={path.length > 0 ? path.join(" / ") : t("departments.detail.topLevel")}
        action={
          <>
            <Button variant="secondary" size="sm" onClick={onEdit}>
              <PencilIcon size={16} strokeWidth={1.75} aria-hidden="true" />
              {t("departments.detail.edit")}
            </Button>
            <DropdownMenu
              iconOnly
              icon={<EllipsisIcon size={16} strokeWidth={1.75} />}
              label={t("departments.detail.more", { name: node.name })}
              items={[
                {
                  label: t("departments.addChild"),
                  icon: <PlusIcon size={16} strokeWidth={1.75} />,
                  onSelect: onAddChild,
                },
                {
                  label: t("departments.detail.delete"),
                  icon: <Trash2Icon size={16} strokeWidth={1.75} />,
                  destructive: true,
                  separatorBefore: true,
                  onSelect: () => onDelete(workerCount),
                },
              ]}
            />
          </>
        }
      >
        <div className="grid grid-cols-2 gap-4 @md:grid-cols-3">
          <DetailOverviewStat
            label={t("departments.detail.subDepartments")}
            value={node.children.length}
            valueClassName="tabular"
          />
          <DetailOverviewStat
            label={t("departments.detail.workers")}
            value={workerCount ?? (workers.error ? "–" : <span className="skeleton inline-block h-5 w-8 align-middle" />)}
            valueClassName="tabular"
          />
          <DetailOverviewStat
            className="col-span-2 @md:col-span-1"
            label={t("departments.detail.updated")}
            value={formatRelative(node.updated_at, t)}
            hint={t("departments.detail.created", { time: formatTimestamp(node.created_at) })}
          />
        </div>
      </Panel>

      <Panel
        title={t("departments.detail.workersTitle")}
        description={t("departments.detail.workersDescription")}
        flush
      >
        {workers.isLoading ? (
          <ul aria-busy="true">
            {Array.from({ length: 3 }, (_, i) => (
              <li key={i} className="flex items-center gap-3 border-t border-border-subtle px-6 py-3">
                <div className="skeleton size-7 rounded-full" />
                <div className="skeleton h-4 w-40" />
              </li>
            ))}
          </ul>
        ) : workers.error ? (
          <div className="px-6 pb-4">
            <Alert tone="danger" title={t("departments.detail.workersError")}>
              <span className="mt-1 flex flex-wrap items-center gap-3">
                <span className="wrap-anywhere">{getErrorMessage(workers.error)}</span>
                <Button variant="secondary" size="sm" onClick={() => workers.refetch()}>
                  {t("common.retry")}
                </Button>
              </span>
            </Alert>
          </div>
        ) : (workers.data ?? []).length === 0 ? (
          <p className="border-t border-border-subtle px-6 py-5 text-sm text-secondary">
            {t("departments.detail.noWorkers")}
          </p>
        ) : (
          <ul>
            {(workers.data ?? []).map((worker) => (
              <li key={worker.id} className="border-t border-border-subtle">
                <Link
                  to={conversationPath(workerConversationId(worker.id))}
                  onClick={closeSettings}
                  className="flex min-w-0 items-center gap-3 px-6 py-3 transition-colors hover:bg-surface-muted"
                >
                  <WorkerAvatar workerId={worker.id} name={worker.name} status={worker.status} size="sm" />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium text-foreground">{worker.name}</div>
                    {worker.description && (
                      <div className="truncate text-xs text-secondary">{worker.description}</div>
                    )}
                  </div>
                  <StatusBadge status={worker.status} />
                </Link>
              </li>
            ))}
          </ul>
        )}
      </Panel>

      <DetailSection className="px-6 pt-5 pb-6">
        <EnvConfigPanel
          scope="department"
          scopeId={node.id}
          title={t("departments.detail.envTitle")}
          description={t("departments.detail.envDescription")}
          inset
        />
      </DetailSection>
    </div>
  )
}
