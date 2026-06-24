import { useMemo, useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { PlusIcon, PencilIcon, Trash2Icon, FolderIcon, ChevronRightIcon, KeyRoundIcon } from "lucide-react"
import { useDepartments, useCreateDepartment, useUpdateDepartment, useDeleteDepartment } from "@/hooks/use-departments"
import { flattenDeptTree } from "@/lib/department-utils"
import { getErrorMessage } from "@/lib/utils"
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
import type { Department } from "@/lib/types"
import { EnvConfigPanel } from "@/components/env-config-panel"

const NO_PARENT_VALUE = "__no_parent__"

type Mode = "idle" | "create" | "edit" | "delete"

export function Departments() {
  const { t } = useTranslation()
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
  const createButton = (
    <Button onClick={() => openCreate()}>
      <PlusIcon className="size-4 mr-1" />
      {t("departments.create")}
    </Button>
  )
  const filteredDepts = useMemo(
    () => flatDepts.filter(({ dept }) => dept.id !== targetDept?.id),
    [flatDepts, targetDept?.id]
  )

  return (
    <FadeIn>
      <div className="space-y-6">
        <PageHeader
          title={t("nav.departments")}
          actions={createButton}
        />

        {flatDepts.length === 0 ? (
          <EmptyState
            title={t("departments.empty")}
            action={createButton}
          />
        ) : (
          <div className="rounded-sm border border-border">
            {flatDepts.map(({ dept, depth }) => (
              <div
                key={dept.id}
                className="flex items-center gap-2 px-3 py-2.5 border-b border-border/60 last:border-b-0 hover:bg-muted/50 group transition-colors"
                style={{ paddingLeft: `${depth * 20 + 12}px` }}
              >
                {depth > 0 && (
                  <ChevronRightIcon className="size-3 shrink-0 text-muted-foreground/50" />
                )}
                <FolderIcon className="size-4 shrink-0 text-muted-foreground" />
                <span className="flex-1 text-sm">{dept.name}</span>
                <div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => setEnvTarget(dept)}
                    title={t("envConfig.depEnvTitle")}
                  >
                    <KeyRoundIcon className="size-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => openCreate(dept.id)}
                    title={t("departments.addChild")}
                  >
                    <PlusIcon className="size-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => openEdit(dept)}
                    title={t("departments.rename")}
                  >
                    <PencilIcon className="size-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => openDelete(dept)}
                    title={t("common.delete")}
                  >
                    <Trash2Icon className="size-3.5" />
                  </Button>
                </div>
              </div>
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
              {error && <div role="alert" className="rounded-sm border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">{error}</div>}
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
