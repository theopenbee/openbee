// Permission keys ("resource:action"). Mirrors internal/infra/auth/permissions.go —
// keep in sync with the Go source of truth.

export const Perm = {
  ContactsRead: "contacts:read",
  ContactsWrite: "contacts:write",
  TasksRead: "tasks:read",
  TasksWrite: "tasks:write",
  ChatWrite: "chat:write",
  SessionsRead: "sessions:read",
  SessionsWrite: "sessions:write",
  DashboardRead: "dashboard:read",
  EnvRead: "env:read",
  EnvWrite: "env:write",
  SystemConfigRead: "system_config:read",
  SystemConfigWrite: "system_config:write",
  UsersManage: "users:manage",
  RolesManage: "roles:manage",
} as const

export type PermKey = (typeof Perm)[keyof typeof Perm]

// Wildcard grants every permission (super-admin).
export const PERM_WILDCARD = "*"

// hasPermission reports whether the resolved perm set holds the given key,
// or holds the wildcard "*".
export function hasPermission(perms: string[] | undefined, key: string): boolean {
  if (!perms) return false
  return perms.includes(PERM_WILDCARD) || perms.includes(key)
}
