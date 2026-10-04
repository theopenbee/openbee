import { useState, type FormEvent } from "react"
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
import { roleLabel, roleDescription } from "@/lib/roles"
import { ALERT_DESTRUCTIVE, SURFACE } from "@/lib/styles"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { EmptyState } from "@/components/empty-state"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
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
import { PERM_WILDCARD } from "@/lib/permissions"
import type { PermissionGroup, Role } from "@/lib/types"

type Mode = "idle" | "create" | "edit" | "delete"

// A super-admin role carries the wildcard permission, granting everything.
function isSuperAdmin(role: Role): boolean {
  return (role.permissions ?? []).includes(PERM_WILDCARD)
}

// PermissionBadges renders a role's permission summary: "all" for super-admin,
// a muted hint when empty, otherwise one badge per permission.
function PermissionBadges({ role, t }: { role: Role; t: (key: string) => string }) {
  if (isSuperAdmin(role)) {
    return <Badge variant="secondary">{t("roles.allPermissions")}</Badge>
  }
  const perms = role.permissions ?? []
  if (perms.length === 0) {
    return <span className="text-body-sm text-muted-foreground">{t("roles.noPermissions")}</span>
  }
  return (
    <>
      {perms.map((p) => (
        <Badge key={p} variant="outline" className="font-mono font-normal">
          {p}
        </Badge>
      ))}
    </>
  )
}

export function Roles() {
  const { t } = useTranslation()
  const { data: roles = [] } = useRoles()
  const { data: groups = [] } = usePermissionGroups()
  const createRole = useCreateRole()
  const updateRole = useUpdateRole()
  const deleteRole = useDeleteRole()

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
      <PlusIcon />
      {t("roles.create")}
    </Button>
  )

  const isFormOpen = mode === "create" || mode === "edit"

  return (
    <FadeIn>
      <div className="w-full">
        <PageHeader title={t("nav.roles")} actions={createButton} />

        {roles.length === 0 ? (
          <EmptyState framed title={t("roles.empty")} action={createButton} />
        ) : (
          <div className={SURFACE}>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="pl-4">{t("roles.form.name")}</TableHead>
                  <TableHead>{t("roles.form.permissions")}</TableHead>
                  <TableHead className="w-0 pr-4">
                    <span className="sr-only">{t("roles.rowActions")}</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {roles.map((role) => (
                  <TableRow key={role.id}>
                    <TableCell className="w-[38%] py-3 pl-4 align-top whitespace-normal md:min-w-44">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-sm font-medium text-strong">{roleLabel(role, t)}</span>
                        {role.is_system && (
                          <Badge variant="outline">
                            <LockIcon aria-hidden="true" />
                            {t("roles.system")}
                          </Badge>
                        )}
                      </div>
                      <p className="mt-0.5 text-body-sm text-muted-foreground">
                        {roleDescription(role, t) || t("common.noDescription")}
                      </p>
                    </TableCell>
                    <TableCell className="py-3 align-top whitespace-normal md:min-w-56">
                      <div className="flex w-full min-w-0 flex-wrap gap-1">
                        <PermissionBadges role={role} t={t} />
                      </div>
                    </TableCell>
                    <TableCell className="py-2.5 pr-4 text-right align-top">
                      {!role.is_system && (
                        <DropdownMenu>
                          <DropdownMenuTrigger
                            render={
                              <Button
                                variant="ghost"
                                size="icon-sm"
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
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}

        {/* Create / edit */}
        <Dialog open={isFormOpen} onOpenChange={(open) => { if (!open) resetForm() }}>
          <DialogContent className="sm:max-w-lg">
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
              <div role="group" aria-labelledby="role-permissions-label" className="space-y-1.5">
                <Label id="role-permissions-label">{t("roles.form.permissions")}</Label>
                <PermissionPicker
                  groups={groups}
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
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("roles.deleteConfirm.title")}</DialogTitle>
              <DialogDescription>
                {t("roles.deleteConfirm.description", { name: target?.name })}
              </DialogDescription>
            </DialogHeader>
            {error && <div role="alert" className={ALERT_DESTRUCTIVE}>{error}</div>}
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
  selected,
  onToggle,
}: {
  groups: PermissionGroup[]
  selected: string[]
  onToggle: (perm: string) => void
}) {
  const { t } = useTranslation()
  if (groups.every((g) => g.permissions.length === 0)) {
    return <p className="text-sm text-muted-foreground">{t("roles.noPermissionsAvailable")}</p>
  }
  // Grouped by resource: each group is a gray caption strip over its
  // permission rows, all hairline-divided inside one outlined well.
  return (
    <div className="max-h-80 overflow-y-auto rounded-sm bg-background ring-1 ring-border">
      {groups.map((group) => (
        <div key={group.resource} className="border-b border-hairline last:border-b-0">
          <p className="sticky top-0 z-10 border-b border-hairline bg-elevated px-3 py-1.5 text-xs font-medium text-muted-foreground">
            {t(`roles.permissionGroups.${group.resource}`, group.resource)}
          </p>
          <div className="divide-y divide-hairline">
            {group.permissions.map((perm) => (
              <label
                key={perm}
                className="flex cursor-pointer items-center gap-3 px-3 py-2 transition-colors hover:bg-elevated"
              >
                <input
                  type="checkbox"
                  className="size-4 shrink-0 cursor-pointer accent-primary"
                  checked={selected.includes(perm)}
                  onChange={() => onToggle(perm)}
                />
                <span className="min-w-0 flex-1 truncate text-sm text-foreground">
                  {t(`roles.permissionItems.${perm.replace(":", ".")}`, perm)}
                </span>
                <span className="shrink-0 font-mono text-xs text-muted-foreground">
                  {perm}
                </span>
              </label>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
