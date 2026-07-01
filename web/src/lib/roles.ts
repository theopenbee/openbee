import type { TFunction } from "i18next"
import type { Role } from "@/lib/types"

// System roles store their name/description as i18n keys (e.g. "roles.superadmin.name");
// user-created roles store literal text entered by the user. Resolve accordingly.
export function roleLabel(role: Role, t: TFunction): string {
  return role.is_system ? t(role.name) : role.name
}

export function roleDescription(role: Role, t: TFunction): string {
  return role.is_system ? t(role.description) : role.description
}
