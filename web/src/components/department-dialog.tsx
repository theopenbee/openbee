import { useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { PlusIcon, PencilIcon, Trash2Icon, FolderIcon } from "lucide-react"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"
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
import { useDepartments, useCreateDepartment, useUpdateDepartment, useDeleteDepartment } from "@/hooks/use-departments"
import type { Department, DepartmentTree } from "@/lib/types"

interface DepartmentManageDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function DepartmentManageDialog({ open, onOpenChange }: DepartmentManageDialogProps) {
  const { t } = useTranslation()
  const { data: departments = [] } = useDepartments()
  const createDept = useCreateDepartment()
  const updateDept = useUpdateDepartment()
  const deleteDept = useDeleteDepartment()

  const [mode, setMode] = useState<"list" | "create" | "edit">("list")
  const [editingDept, setEditingDept] = useState<Department | null>(null)
  const [name, setName] = useState("")
  const [parentId, setParentId] = useState<string | null>(null)
  const [error, setError] = useState("")

  const flatDepts = flattenTree(departments)

  const resetForm = () => {
    setName("")
    setParentId(null)
    setError("")
    setEditingDept(null)
    setMode("list")
  }

  const handleCreate = async (e?: FormEvent) => {
    e?.preventDefault()
    if (!name.trim()) return
    try {
      await createDept.mutateAsync({ name: name.trim(), parent_id: parentId })
      resetForm()
    } catch (err: any) {
      setError(err.message)
    }
  }

  const handleUpdate = async (e?: FormEvent) => {
    e?.preventDefault()
    if (!editingDept || !name.trim()) return
    try {
      await updateDept.mutateAsync({
        id: editingDept.id,
        data: { name: name.trim(), parent_id: parentId },
      })
      resetForm()
    } catch (err: any) {
      setError(err.message)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await deleteDept.mutateAsync(id)
    } catch (err: any) {
      setError(err.message)
    }
  }

  const startEdit = (dept: Department) => {
    setEditingDept(dept)
    setName(dept.name)
    setParentId(dept.parent_id)
    setError("")
    setMode("edit")
  }

  const startCreate = (parentIdVal?: string | null) => {
    setName("")
    setParentId(parentIdVal ?? null)
    setError("")
    setMode("create")
  }

  return (
    <Dialog open={open} onOpenChange={(o) => { if (!o) resetForm(); onOpenChange(o) }}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t("departments.manageTitle")}</DialogTitle>
          <DialogDescription>{t("departments.manageDescription")}</DialogDescription>
        </DialogHeader>

        {error && <p className="text-sm text-destructive">{error}</p>}

        {mode === "list" && (
          <div className="space-y-2">
            {flatDepts.length === 0 ? (
              <p className="text-sm text-muted-foreground text-center py-4">
                {t("departments.empty")}
              </p>
            ) : (
              <div className="max-h-64 overflow-y-auto space-y-0.5">
                {flatDepts.map(({ dept, depth }) => (
                  <div
                    key={dept.id}
                    className="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-muted group"
                    style={{ paddingLeft: `${depth * 16 + 8}px` }}
                  >
                    <FolderIcon className="size-4 shrink-0 text-muted-foreground" />
                    <span className="text-sm flex-1 truncate">{dept.name}</span>
                    <div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => startCreate(dept.id)}
                        title={t("departments.addChild")}
                      >
                        <PlusIcon className="size-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => startEdit(dept)}
                        title={t("departments.rename")}
                      >
                        <PencilIcon className="size-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => handleDelete(dept.id)}
                        title={t("common.delete")}
                      >
                        <Trash2Icon className="size-3.5" />
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}

            <DialogFooter className="pt-2">
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                {t("common.cancel")}
              </Button>
              <Button onClick={() => startCreate()}>
                <PlusIcon className="size-4 mr-1" />
                {t("departments.create")}
              </Button>
            </DialogFooter>
          </div>
        )}

        {(mode === "create" || mode === "edit") && (
          <form onSubmit={mode === "create" ? handleCreate : handleUpdate} className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="dept-name">{t("departments.form.name")}</Label>
              <Input
                id="dept-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={t("departments.form.namePlaceholder")}
                required
                autoFocus
              />
            </div>

            <div className="space-y-1.5">
              <Label>{t("departments.form.parent")}</Label>
              <Select
                value={parentId ?? "__none__"}
                onValueChange={(v) => setParentId(v === "__none__" ? null : v)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__none__">{t("departments.form.noParent")}</SelectItem>
                  {flatDepts
                    .filter(({ dept }) => dept.id !== editingDept?.id)
                    .map(({ dept, depth }) => (
                      <SelectItem key={dept.id} value={dept.id}>
                        {"  ".repeat(depth)}{dept.name}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={resetForm}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={!name.trim()}>
                {mode === "create" ? t("departments.create") : t("common.save")}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}

function flattenTree(
  tree: DepartmentTree[],
  depth = 0
): { dept: Department; depth: number }[] {
  const result: { dept: Department; depth: number }[] = []
  for (const node of tree) {
    result.push({ dept: node, depth })
    if (node.children.length > 0) {
      result.push(...flattenTree(node.children, depth + 1))
    }
  }
  return result
}
