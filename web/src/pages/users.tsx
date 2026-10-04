import { useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import {
  PlusIcon,
  PencilIcon,
  Trash2Icon,
  KeyRoundIcon,
  ShieldIcon,
  MoreHorizontalIcon,
  UserCheckIcon,
  UserXIcon,
} from "lucide-react"
import {
  useUsers,
  useCreateUser,
  useUpdateUserProfile,
  useSetUserRoles,
  useSetUserStatus,
  useSetUserPassword,
  useDeleteUser,
} from "@/hooks/use-users"
import { useRoles } from "@/hooks/use-roles"
import { getErrorMessage } from "@/lib/utils"
import { roleLabel, roleDescription } from "@/lib/roles"
import { ALERT_DESTRUCTIVE } from "@/lib/styles"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { EmptyState } from "@/components/empty-state"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
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
import type { Role, UserWithRoles } from "@/lib/types"

type Mode = "idle" | "create" | "edit" | "roles" | "password" | "delete"

const MIN_PASSWORD_LENGTH = 6

function RoleCheckboxes({
  roles,
  selected,
  onToggle,
}: {
  roles: Role[]
  selected: string[]
  onToggle: (id: string) => void
}) {
  const { t } = useTranslation()
  if (roles.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("users.noRoles")}</p>
  }
  return (
    <div className="max-h-60 divide-y divide-hairline overflow-y-auto rounded-sm bg-background ring-1 ring-border">
      {roles.map((role) => (
        <label
          key={role.id}
          className="flex cursor-pointer items-start gap-3 px-3 py-2.5 transition-colors hover:bg-elevated"
        >
          <input
            type="checkbox"
            className="mt-0.5 size-4 shrink-0 cursor-pointer accent-primary dark:[color-scheme:dark]"
            checked={selected.includes(role.id)}
            onChange={() => onToggle(role.id)}
          />
          <span className="min-w-0">
            <span className="block text-sm font-medium text-strong">{roleLabel(role, t)}</span>
            {role.description && (
              <span className="mt-0.5 block truncate text-[13px] text-muted-foreground">
                {roleDescription(role, t)}
              </span>
            )}
          </span>
        </label>
      ))}
    </div>
  )
}

export function Users() {
  const { t } = useTranslation()
  const { data: users = [] } = useUsers()
  const { data: roles = [] } = useRoles()
  const createUser = useCreateUser()
  const updateProfile = useUpdateUserProfile()
  const setRoles = useSetUserRoles()
  const setStatus = useSetUserStatus()
  const setPassword = useSetUserPassword()
  const deleteUser = useDeleteUser()

  const [mode, setMode] = useState<Mode>("idle")
  const [target, setTarget] = useState<UserWithRoles | null>(null)
  const [error, setError] = useState("")

  // Create form
  const [username, setUsername] = useState("")
  const [newUserPassword, setNewUserPassword] = useState("")
  const [displayName, setDisplayName] = useState("")
  const [roleIds, setRoleIds] = useState<string[]>([])

  // Reset password form
  const [newPassword, setNewPassword] = useState("")

  const resetForm = () => {
    setMode("idle")
    setTarget(null)
    setError("")
    setUsername("")
    setNewUserPassword("")
    setDisplayName("")
    setRoleIds([])
    setNewPassword("")
  }

  const openCreate = () => {
    resetForm()
    setMode("create")
  }

  const openEdit = (user: UserWithRoles) => {
    setError("")
    setTarget(user)
    setUsername(user.username)
    setDisplayName(user.display_name)
    setMode("edit")
  }

  const openRoles = (user: UserWithRoles) => {
    setError("")
    setTarget(user)
    setRoleIds(user.roles.map((r) => r.id))
    setMode("roles")
  }

  const openPassword = (user: UserWithRoles) => {
    setError("")
    setTarget(user)
    setNewPassword("")
    setMode("password")
  }

  const openDelete = (user: UserWithRoles) => {
    setError("")
    setTarget(user)
    setMode("delete")
  }

  const toggleRole = (id: string) =>
    setRoleIds((prev) => (prev.includes(id) ? prev.filter((r) => r !== id) : [...prev, id]))

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault()
    if (!username.trim() || newUserPassword.length < MIN_PASSWORD_LENGTH) return
    try {
      await createUser.mutateAsync({
        username: username.trim(),
        password: newUserPassword,
        display_name: displayName.trim() || undefined,
        role_ids: roleIds,
      })
      toast.success(t("users.created"))
      resetForm()
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  const handleSaveProfile = async (e: FormEvent) => {
    e.preventDefault()
    if (!target || !username.trim()) return
    try {
      await updateProfile.mutateAsync({
        id: target.id,
        username: username.trim(),
        displayName: displayName.trim() || undefined,
      })
      toast.success(t("users.profileUpdated"))
      resetForm()
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  const handleSaveRoles = async (e: FormEvent) => {
    e.preventDefault()
    if (!target) return
    try {
      await setRoles.mutateAsync({ id: target.id, roleIds })
      toast.success(t("users.rolesUpdated"))
      resetForm()
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  const handleSavePassword = async (e: FormEvent) => {
    e.preventDefault()
    if (!target || newPassword.length < MIN_PASSWORD_LENGTH) return
    try {
      await setPassword.mutateAsync({ id: target.id, password: newPassword })
      toast.success(t("users.passwordReset"))
      resetForm()
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  const handleToggleStatus = async (user: UserWithRoles) => {
    const next = user.status === "active" ? "disabled" : "active"
    try {
      await setStatus.mutateAsync({ id: user.id, status: next })
      toast.success(next === "active" ? t("users.enabled") : t("users.disabled"))
    } catch (err) {
      toast.error(getErrorMessage(err))
    }
  }

  const handleDelete = async () => {
    if (!target) return
    try {
      await deleteUser.mutateAsync(target.id)
      toast.success(t("users.deleted"))
      resetForm()
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  const createButton = (
    <Button onClick={openCreate}>
      <PlusIcon />
      {t("users.create")}
    </Button>
  )

  const canCreate = username.trim().length > 0 && newUserPassword.length >= MIN_PASSWORD_LENGTH

  return (
    <FadeIn>
      <div className="w-full">
        <PageHeader title={t("nav.users")} actions={createButton} />

        {users.length === 0 ? (
          <div className="rounded-sm bg-card ring-1 ring-border">
            <EmptyState title={t("users.empty")} action={createButton} />
          </div>
        ) : (
          <div className="overflow-hidden rounded-sm bg-card ring-1 ring-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="pl-4">{t("nav.users")}</TableHead>
                  <TableHead>{t("users.table.roles")}</TableHead>
                  <TableHead>{t("users.table.status")}</TableHead>
                  <TableHead className="w-0 pr-4">
                    <span className="sr-only">{t("users.rowActions")}</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {users.map((user) => (
                  <TableRow key={user.id}>
                    <TableCell className="py-2.5 pl-4">
                      <div className="min-w-0">
                        <div className="text-sm font-medium text-strong">
                          {user.display_name || user.username}
                        </div>
                        {user.display_name && (
                          <div className="mt-0.5 text-[13px] text-muted-foreground">
                            {user.username}
                          </div>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {user.roles.length === 0 ? (
                          <span className="text-[13px] text-muted-foreground">—</span>
                        ) : (
                          user.roles.map((r) => (
                            <Badge key={r.id} variant="outline">
                              {roleLabel(r, t)}
                            </Badge>
                          ))
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge variant={user.status === "active" ? "success" : "secondary"}>
                        <span
                          aria-hidden="true"
                          className="size-1.5 rounded-full bg-current"
                        />
                        {user.status === "active"
                          ? t("users.status.active")
                          : t("users.status.disabled")}
                      </Badge>
                    </TableCell>
                    <TableCell className="py-2 pr-4 text-right">
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          render={
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              className="text-muted-foreground"
                              aria-label={t("users.rowActions")}
                            />
                          }
                        >
                          <MoreHorizontalIcon className="size-4" />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="min-w-44">
                          <DropdownMenuItem onClick={() => openEdit(user)}>
                            <PencilIcon className="size-3.5" />
                            {t("users.editProfile")}
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => openRoles(user)}>
                            <ShieldIcon className="size-3.5" />
                            {t("users.editRoles")}
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => openPassword(user)}>
                            <KeyRoundIcon className="size-3.5" />
                            {t("users.resetPassword")}
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => handleToggleStatus(user)}>
                            {user.status === "active" ? (
                              <>
                                <UserXIcon className="size-3.5" />
                                {t("users.disable")}
                              </>
                            ) : (
                              <>
                                <UserCheckIcon className="size-3.5" />
                                {t("users.enable")}
                              </>
                            )}
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            variant="destructive"
                            onClick={() => openDelete(user)}
                          >
                            <Trash2Icon className="size-3.5" />
                            {t("common.delete")}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}

        {/* Create user */}
        <Dialog open={mode === "create"} onOpenChange={(open) => { if (!open) resetForm() }}>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>{t("users.create")}</DialogTitle>
              <DialogDescription>{t("users.createDescription")}</DialogDescription>
            </DialogHeader>
            <form onSubmit={handleCreate} className="space-y-4">
              {error && <div role="alert" className={ALERT_DESTRUCTIVE}>{error}</div>}
              <div className="space-y-1.5">
                <Label htmlFor="user-username">{t("users.form.username")}</Label>
                <Input
                  id="user-username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder={t("users.form.usernamePlaceholder")}
                  required
                  autoFocus
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="user-password">{t("users.form.password")}</Label>
                <Input
                  id="user-password"
                  type="password"
                  value={newUserPassword}
                  onChange={(e) => setNewUserPassword(e.target.value)}
                  placeholder={t("users.form.passwordPlaceholder")}
                  required
                  minLength={MIN_PASSWORD_LENGTH}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="user-display-name">{t("users.form.displayName")}</Label>
                <Input
                  id="user-display-name"
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  placeholder={t("users.form.displayNamePlaceholder")}
                />
              </div>
              <div role="group" aria-labelledby="user-roles-label" className="space-y-1.5">
                <Label id="user-roles-label">{t("users.form.roles")}</Label>
                <RoleCheckboxes roles={roles} selected={roleIds} onToggle={toggleRole} />
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={resetForm}>
                  {t("common.cancel")}
                </Button>
                <Button type="submit" disabled={!canCreate || createUser.isPending}>
                  {t("users.create")}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>

        {/* Edit profile */}
        <Dialog open={mode === "edit"} onOpenChange={(open) => { if (!open) resetForm() }}>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>{t("users.editProfile")}</DialogTitle>
              <DialogDescription>
                {t("users.editProfileDescription", { name: target?.username })}
              </DialogDescription>
            </DialogHeader>
            <form onSubmit={handleSaveProfile} className="space-y-4">
              {error && <div role="alert" className={ALERT_DESTRUCTIVE}>{error}</div>}
              <div className="space-y-1.5">
                <Label htmlFor="edit-user-username">{t("users.form.username")}</Label>
                <Input
                  id="edit-user-username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder={t("users.form.usernamePlaceholder")}
                  required
                  autoFocus
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="edit-user-display-name">{t("users.form.displayName")}</Label>
                <Input
                  id="edit-user-display-name"
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  placeholder={t("users.form.displayNamePlaceholder")}
                />
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={resetForm}>
                  {t("common.cancel")}
                </Button>
                <Button type="submit" disabled={!username.trim() || updateProfile.isPending}>
                  {t("common.save")}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>

        {/* Edit roles */}
        <Dialog open={mode === "roles"} onOpenChange={(open) => { if (!open) resetForm() }}>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>{t("users.editRoles")}</DialogTitle>
              <DialogDescription>
                {t("users.editRolesDescription", { name: target?.username })}
              </DialogDescription>
            </DialogHeader>
            <form onSubmit={handleSaveRoles} className="space-y-4">
              {error && <div role="alert" className={ALERT_DESTRUCTIVE}>{error}</div>}
              <RoleCheckboxes roles={roles} selected={roleIds} onToggle={toggleRole} />
              <DialogFooter>
                <Button type="button" variant="outline" onClick={resetForm}>
                  {t("common.cancel")}
                </Button>
                <Button type="submit" disabled={setRoles.isPending}>
                  {t("common.save")}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>

        {/* Reset password */}
        <Dialog open={mode === "password"} onOpenChange={(open) => { if (!open) resetForm() }}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("users.resetPassword")}</DialogTitle>
              <DialogDescription>
                {t("users.resetPasswordDescription", { name: target?.username })}
              </DialogDescription>
            </DialogHeader>
            <form onSubmit={handleSavePassword} className="space-y-4">
              {error && <div role="alert" className={ALERT_DESTRUCTIVE}>{error}</div>}
              <div className="space-y-1.5">
                <Label htmlFor="reset-password">{t("users.form.password")}</Label>
                <Input
                  id="reset-password"
                  type="password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  placeholder={t("users.form.passwordPlaceholder")}
                  required
                  minLength={MIN_PASSWORD_LENGTH}
                  autoFocus
                />
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={resetForm}>
                  {t("common.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={newPassword.length < MIN_PASSWORD_LENGTH || setPassword.isPending}
                >
                  {t("common.save")}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>

        {/* Delete */}
        <Dialog open={mode === "delete"} onOpenChange={(open) => { if (!open) resetForm() }}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("users.deleteConfirm.title")}</DialogTitle>
              <DialogDescription>
                {t("users.deleteConfirm.description", { name: target?.username })}
              </DialogDescription>
            </DialogHeader>
            {error && <div role="alert" className={ALERT_DESTRUCTIVE}>{error}</div>}
            <DialogFooter>
              <Button variant="outline" onClick={resetForm}>
                {t("common.cancel")}
              </Button>
              <Button variant="destructive" onClick={handleDelete} disabled={deleteUser.isPending}>
                {t("common.delete")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </FadeIn>
  )
}
