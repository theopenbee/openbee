import { useMemo, useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { PlusIcon, PencilIcon, Trash2Icon, LockIcon, MoreHorizontalIcon } from "lucide-react"
import {
  useRoles,
  usePermissionGroups,
  useCreateRole,
  useUpdateRole,
  useDeleteRole,
} from "@/hooks/use-roles"
import { getErrorMessage } from "@/lib/utils"
import { ALERT_DESTRUCTIVE } from "@/lib/styles"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { EmptyState } from "@/components/empty-state"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Badge } from "@/components/ui/badge"
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
import type { PermissionGroup, Role } from "@/lib/types"

type Mode = "idle" | "create" | "edit" | "delete"

const WILDCARD = "*"

// A super-admin role carries the wildcard permission, granting everything.
function isSuperAdmin(role: Role): boolean {
  return (role.permissions ?? []).includes(WILDCARD)
}

export function Roles() {
  const { t } = useTranslation()
  const { data: roles = [] } = useRoles()
  const { data: groups = [] } = usePermissionGroups()
  const createRole = useCreateRole()
  const updateRole = useUpdateRole()
  const deleteRole = useDeleteRole()

  const allPermissions = useMemo(
    () => groups.flatMap((g) => g.permissions),
    [groups]
  )

  const [mode, setMode] = useState<Mode>("idle")
  const [target, setTarget] = useState<Role | null>(null)
  const [error, setError] = useState("")

  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [perms, setPerms] = useState<string[]>([])

  const resetForm = () => {
    setMode("idle")
    setTarget(null)
    setError("")
    setName("")
    setDescription("")
    setPerms([])
  }

  const openCreate = () => {
    resetForm()
    setMode("create")
  }

  const openEdit = (role: Role) => {
    setError("")
    setTarget(role)
    setName(role.name)
    setDescription(role.description)
    setPerms(role.permissions ?? [])
    setMode("edit")
  }

  const openDelete = (role: Role) => {
    setError("")
    setTarget(role)
    setMode("delete")
  }

  const togglePerm = (perm: string) =>
    setPerms((prev) => (prev.includes(perm) ? prev.filter((p) => p !== perm) : [...prev, perm]))

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    try {
      if (mode === "create") {
        await createRole.mutateAsync({
          name: name.trim(),
          description: description.trim(),
          permissions: perms,
        })
        toast.success(t("roles.created"))
      } else if (target) {
        await updateRole.mutateAsync({
          id: target.id,
          data: { name: name.trim(), description: description.trim(), permissions: perms },
        })
        toast.success(t("roles.updated"))
      }
      resetForm()
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  const handleDelete = async () => {
    if (!target) return
    try {
      await deleteRole.mutateAsync(target.id)
      toast.success(t("roles.deleted"))
      resetForm()
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  const createButton = (
    <Button onClick={openCreate}>
      <PlusIcon className="size-4 mr-1" />
      {t("roles.create")}
    </Button>
  )

  const isFormOpen = mode === "create" || mode === "edit"

  return (
    <FadeIn>
      <div className="mx-auto w-full max-w-3xl space-y-6">
        <PageHeader title={t("nav.roles")} actions={createButton} />

        {roles.length === 0 ? (
          <EmptyState title={t("roles.empty")} action={createButton} />
        ) : (
          <div className="space-y-3">
            {roles.map((role) => {
              const superAdmin = isSuperAdmin(role)
              return (
                <div key={role.id} className="rounded-sm border border-border p-4">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-medium">{role.name}</span>
                        {role.is_system && (
                          <Badge variant="outline">
                            <LockIcon className="size-3" />
                            {t("roles.system")}
                          </Badge>
                        )}
                      </div>
                      <p className="mt-0.5 text-sm text-muted-foreground">
                        {role.description || t("common.noDescription")}
                      </p>
                    </div>
                    {!role.is_system && (
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          render={
                            <Button
                              variant="ghost"
                              size="icon-xs"
                              className="text-muted-foreground"
                              aria-label={t("roles.rowActions")}
                            />
                          }
                        >
                          <MoreHorizontalIcon className="size-4" />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="min-w-40">
                          <DropdownMenuItem onClick={() => openEdit(role)}>
                            <PencilIcon className="size-3.5" />
                            {t("common.edit")}
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem variant="destructive" onClick={() => openDelete(role)}>
                            <Trash2Icon className="size-3.5" />
                            {t("common.delete")}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    )}
                  </div>
                  <div className="mt-3 flex flex-wrap gap-1.5">
                    {superAdmin ? (
                      <Badge variant="secondary">{t("roles.allPermissions")}</Badge>
                    ) : (role.permissions ?? []).length === 0 ? (
                      <span className="text-xs text-muted-foreground">
                        {t("roles.noPermissions")}
                      </span>
                    ) : (
                      (role.permissions ?? []).map((p) => (
                        <Badge key={p} variant="outline" className="font-mono text-[11px]">
                          {p}
                        </Badge>
                      ))
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        )}

        {/* Create / edit */}
        <Dialog open={isFormOpen} onOpenChange={(open) => { if (!open) resetForm() }}>
          <DialogContent className="max-w-lg">
            <DialogHeader>
              <DialogTitle>
                {mode === "create" ? t("roles.create") : t("roles.edit")}
              </DialogTitle>
              <DialogDescription>{t("roles.formDescription")}</DialogDescription>
            </DialogHeader>
            <form onSubmit={handleSubmit} className="space-y-4">
              {error && <div role="alert" className={ALERT_DESTRUCTIVE}>{error}</div>}
              <div className="space-y-1.5">
                <Label htmlFor="role-name">{t("roles.form.name")}</Label>
                <Input
                  id="role-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={t("roles.form.namePlaceholder")}
                  required
                  autoFocus
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="role-description">{t("roles.form.description")}</Label>
                <Textarea
                  id="role-description"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder={t("roles.form.descriptionPlaceholder")}
                  rows={2}
                />
              </div>
              <div className="space-y-1.5">
                <Label>{t("roles.form.permissions")}</Label>
                <PermissionPicker
                  groups={groups}
                  allPermissions={allPermissions}
                  selected={perms}
                  onToggle={togglePerm}
                />
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={resetForm}>
                  {t("common.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={!name.trim() || createRole.isPending || updateRole.isPending}
                >
                  {mode === "create" ? t("roles.create") : t("common.save")}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>

        {/* Delete */}
        <Dialog open={mode === "delete"} onOpenChange={(open) => { if (!open) resetForm() }}>
          <DialogContent className="max-w-sm">
            <DialogHeader>
              <DialogTitle>{t("roles.deleteConfirm.title")}</DialogTitle>
              <DialogDescription>
                {t("roles.deleteConfirm.description", { name: target?.name })}
              </DialogDescription>
            </DialogHeader>
            {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
            <DialogFooter>
              <Button variant="outline" onClick={resetForm}>
                {t("common.cancel")}
              </Button>
              <Button variant="destructive" onClick={handleDelete} disabled={deleteRole.isPending}>
                {t("common.delete")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </FadeIn>
  )
}

function PermissionPicker({
  groups,
  allPermissions,
  selected,
  onToggle,
}: {
  groups: PermissionGroup[]
  allPermissions: string[]
  selected: string[]
  onToggle: (perm: string) => void
}) {
  const { t } = useTranslation()
  if (allPermissions.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("roles.noPermissionsAvailable")}</p>
  }
  return (
    <div className="max-h-72 space-y-3 overflow-y-auto rounded-sm border border-border p-3">
      {groups.map((group) => (
        <div key={group.resource} className="space-y-1">
          <p className="text-xs font-medium uppercase tracking-[0.05em] text-muted-foreground">
            {group.resource}
          </p>
          <div className="grid grid-cols-1 gap-0.5 sm:grid-cols-2">
            {group.permissions.map((perm) => (
              <label
                key={perm}
                className="flex cursor-pointer items-center gap-2 rounded-sm px-1.5 py-1 hover:bg-muted/50"
              >
                <input
                  type="checkbox"
                  className="size-4 shrink-0 accent-primary"
                  checked={selected.includes(perm)}
                  onChange={() => onToggle(perm)}
                />
                <span className="truncate font-mono text-xs">{perm}</span>
              </label>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
