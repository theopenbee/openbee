import { useMemo, useState, type ComponentType, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { PlusIcon, PencilIcon, Trash2Icon, FolderIcon, FolderOpenIcon, ChevronRightIcon, KeyRoundIcon, MoreHorizontalIcon } from "lucide-react"
import { useDepartments, useCreateDepartment, useUpdateDepartment, useDeleteDepartment } from "@/hooks/use-departments"
import { useCan } from "@/hooks/use-can"
import { Perm } from "@/lib/permissions"
import { flattenDeptTree } from "@/lib/department-utils"
import { cn, getErrorMessage } from "@/lib/utils"
import { ALERT_DESTRUCTIVE } from "@/lib/styles"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { EmptyState } from "@/components/empty-state"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu"
import type { Department, DepartmentTree } from "@/lib/types"
import { EnvConfigPanel } from "@/components/env-config-panel"

const NO_PARENT_VALUE = "__no_parent__"

type Mode = "idle" | "create" | "edit" | "delete"

// One action in a department row's dropdown. The env entry needs only env:read
// (it opens a panel that gates its own writes); the structural actions need
// contacts:write. The menu is assembled by filtering, so an empty list hides
// the trigger entirely.
type DeptRowAction = {
  key: string
  icon: ComponentType<{ className?: string }>
  label: string
  onClick: () => void
  destructive?: boolean
}

export function Departments() {
  const { t } = useTranslation()
  const canWrite = useCan(Perm.ContactsWrite)
  const { data: departments = [] } = useDepartments()
  const createDept = useCreateDepartment()
  const updateDept = useUpdateDepartment()
  const deleteDept = useDeleteDepartment()

  const flatDepts = useMemo(() => flattenDeptTree(departments), [departments])
  const deptNameById = useMemo(
    () => Object.fromEntries(flatDepts.map(({ dept }) => [dept.id, dept.name])),
    [flatDepts]
  )

  const [mode, setMode] = useState<Mode>("idle")
  const [targetDept, setTargetDept] = useState<Department | null>(null)
  const [envTarget, setEnvTarget] = useState<Department | null>(null)
  const [formName, setFormName] = useState("")
  const [formParentId, setFormParentId] = useState<string | null>(null)
  const [error, setError] = useState("")

  const resetForm = () => {
    setMode("idle")
    setTargetDept(null)
    setFormName("")
    setFormParentId(null)
    setError("")
  }

  const openCreate = (parentId?: string | null) => {
    setTargetDept(null)
    setFormName("")
    setFormParentId(parentId ?? null)
    setError("")
    setMode("create")
  }

  const openEdit = (dept: Department) => {
    setTargetDept(dept)
    setFormName(dept.name)
    setFormParentId(dept.parent_id)
    setError("")
    setMode("edit")
  }

  const openDelete = (dept: Department) => {
    setTargetDept(dept)
    setError("")
    setMode("delete")
  }

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault()
    if (!formName.trim()) return
    try {
      await createDept.mutateAsync({ name: formName.trim(), parent_id: formParentId })
      resetForm()
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  const handleUpdate = async (e: FormEvent) => {
    e.preventDefault()
    if (!targetDept || !formName.trim()) return
    try {
      await updateDept.mutateAsync({
        id: targetDept.id,
        data: { name: formName.trim(), parent_id: formParentId },
      })
      resetForm()
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  const handleDelete = async () => {
    if (!targetDept) return
    try {
      await deleteDept.mutateAsync(targetDept.id)
      resetForm()
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  const isFormOpen = mode === "create" || mode === "edit"
  const isDeleteOpen = mode === "delete"
  const createButton = canWrite ? (
    <Button onClick={() => openCreate()}>
      <PlusIcon className="size-4 mr-1" />
      {t("departments.create")}
    </Button>
  ) : null
  const filteredDepts = useMemo(
    () => flatDepts.filter(({ dept }) => dept.id !== targetDept?.id),
    [flatDepts, targetDept?.id]
  )

  return (
    <FadeIn>
      <div className="mx-auto w-full max-w-3xl space-y-6">
        <PageHeader
          title={t("nav.departments")}
          actions={createButton}
        />

        {departments.length === 0 ? (
          <EmptyState
            title={t("departments.empty")}
            action={createButton}
          />
        ) : (
          <div className="rounded-sm border border-border p-1.5">
            {departments.map((node) => (
              <DepartmentRow
                key={node.id}
                node={node}
                onEnv={setEnvTarget}
                onCreateChild={openCreate}
                onEdit={openEdit}
                onDelete={openDelete}
              />
            ))}
          </div>
        )}

        <Dialog open={isFormOpen} onOpenChange={(open) => { if (!open) resetForm() }}>
          <DialogContent className="max-w-md">
            <DialogHeader>
              <DialogTitle>
                {mode === "create" ? t("departments.create") : t("departments.rename")}
              </DialogTitle>
              <DialogDescription>{t("departments.manageDescription")}</DialogDescription>
            </DialogHeader>
            <form onSubmit={mode === "create" ? handleCreate : handleUpdate} className="space-y-4">
              {error && <div role="alert" className={ALERT_DESTRUCTIVE}>{error}</div>}
              <div className="space-y-1.5">
                <Label htmlFor="dept-name">{t("departments.form.name")}</Label>
                <Input
                  id="dept-name"
                  value={formName}
                  onChange={(e) => setFormName(e.target.value)}
                  placeholder={t("departments.form.namePlaceholder")}
                  required
                  autoFocus
                />
              </div>
              <div className="space-y-1.5">
                <Label>{t("departments.form.parent")}</Label>
                <Select
                  value={formParentId ?? NO_PARENT_VALUE}
                  onValueChange={(v) => setFormParentId(v === NO_PARENT_VALUE ? null : v)}
                >
                  <SelectTrigger>
                    <SelectValue>
                      {(value: string) =>
                        value === NO_PARENT_VALUE
                          ? t("departments.form.noParent")
                          : deptNameById[value] ?? value
                      }
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NO_PARENT_VALUE}>{t("departments.form.noParent")}</SelectItem>
                    {filteredDepts.map(({ dept, depth }) => (
                      <SelectItem key={dept.id} value={dept.id}>
                        <span style={{ paddingLeft: `${depth * 12}px` }}>{dept.name}</span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={resetForm}>
                  {t("common.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={!formName.trim() || createDept.isPending || updateDept.isPending}
                >
                  {mode === "create" ? t("departments.create") : t("common.save")}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>

        <Dialog open={isDeleteOpen} onOpenChange={(open) => { if (!open) resetForm() }}>
          <DialogContent className="max-w-sm">
            <DialogHeader>
              <DialogTitle>{t("departments.deleteConfirm.title")}</DialogTitle>
              <DialogDescription>
                {t("departments.deleteConfirm.description", { name: targetDept?.name })}
              </DialogDescription>
            </DialogHeader>
            {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
            <DialogFooter>
              <Button variant="outline" onClick={resetForm}>
                {t("common.cancel")}
              </Button>
              <Button
                variant="destructive"
                onClick={handleDelete}
                disabled={deleteDept.isPending}
              >
                {t("common.delete")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={!!envTarget} onOpenChange={(open) => { if (!open) setEnvTarget(null) }}>
          <DialogContent className="sm:max-w-2xl">
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <KeyRoundIcon className="size-4 text-muted-foreground" />
                {t("envConfig.depEnvTitle")}
              </DialogTitle>
              <DialogDescription className="flex items-center gap-1.5">
                <FolderIcon className="size-3.5" />
                {envTarget?.name}
              </DialogDescription>
            </DialogHeader>
            <div className="max-h-[60vh] overflow-y-auto -mx-4 px-4">
              {envTarget && (
                <EnvConfigPanel scope="department" scopeId={envTarget.id} />
              )}
            </div>
            <DialogFooter showCloseButton />
          </DialogContent>
        </Dialog>
      </div>
    </FadeIn>
  )
}

interface DepartmentRowProps {
  node: DepartmentTree
  onEnv: (dept: Department) => void
  onCreateChild: (parentId: string) => void
  onEdit: (dept: Department) => void
  onDelete: (dept: Department) => void
}

function DepartmentRow({ node, onEnv, onCreateChild, onEdit, onDelete }: DepartmentRowProps) {
  const { t } = useTranslation()
  const canWrite = useCan(Perm.ContactsWrite)
  const canReadEnv = useCan(Perm.EnvRead)
  const [expanded, setExpanded] = useState(true)
  const hasChildren = node.children.length > 0

  const actions: DeptRowAction[] = [
    ...(canReadEnv
      ? [{ key: "env", icon: KeyRoundIcon, label: t("envConfig.depEnvTitle"), onClick: () => onEnv(node) }]
      : []),
    ...(canWrite
      ? [
          { key: "addChild", icon: PlusIcon, label: t("departments.addChild"), onClick: () => onCreateChild(node.id) },
          { key: "rename", icon: PencilIcon, label: t("departments.rename"), onClick: () => onEdit(node) },
          { key: "delete", icon: Trash2Icon, label: t("common.delete"), destructive: true, onClick: () => onDelete(node) },
        ]
      : []),
  ]
  const normalActions = actions.filter((a) => !a.destructive)
  const dangerActions = actions.filter((a) => a.destructive)

  return (
    <div>
      <div className="group flex items-center gap-2 rounded-sm px-2 py-2 transition-colors hover:bg-muted/50">
        {hasChildren ? (
          <button
            type="button"
            onClick={() => setExpanded((v) => !v)}
            aria-expanded={expanded}
            aria-label={node.name}
            className="grid size-5 shrink-0 place-items-center rounded-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <ChevronRightIcon
              className={cn("size-3.5 transition-transform", expanded && "rotate-90")}
            />
          </button>
        ) : (
          <span className="size-5 shrink-0" />
        )}
        {expanded && hasChildren ? (
          <FolderOpenIcon className="size-4 shrink-0 text-muted-foreground" />
        ) : (
          <FolderIcon className="size-4 shrink-0 text-muted-foreground" />
        )}
        <span className="flex-1 truncate text-sm">{node.name}</span>
        {actions.length > 0 && (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-xs"
                  className="text-muted-foreground"
                  aria-label={t("departments.rowActions")}
                />
              }
            >
              <MoreHorizontalIcon className="size-4" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-40">
              {normalActions.map((action) => (
                <DropdownMenuItem key={action.key} onClick={action.onClick}>
                  <action.icon className="size-3.5" />
                  {action.label}
                </DropdownMenuItem>
              ))}
              {dangerActions.length > 0 && normalActions.length > 0 && <DropdownMenuSeparator />}
              {dangerActions.map((action) => (
                <DropdownMenuItem key={action.key} variant="destructive" onClick={action.onClick}>
                  <action.icon className="size-3.5" />
                  {action.label}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>

      {expanded && hasChildren && (
        <div className="ml-[1.0625rem] border-l border-border pl-2">
          {node.children.map((child) => (
            <DepartmentRow
              key={child.id}
              node={child}
              onEnv={onEnv}
              onCreateChild={onCreateChild}
              onEdit={onEdit}
              onDelete={onDelete}
            />
          ))}
        </div>
      )}
    </div>
  )
}
