import type { ReactNode } from "react"
import {
  LayoutDashboardIcon,
  ActivityIcon,
  ClockIcon,
  MessageCircleIcon,
  ContactIcon,
  BotIcon,
  Settings2Icon,
} from "lucide-react"

import { Perm, hasPermission } from "@/lib/permissions"

// The navigation tree is the single source of truth for both the sidebar and
// the home-landing resolver. Each entry stores an i18n `titleKey` (translated
// at render time) and an optional `perm`; an absent perm means the entry is
// always reachable. Keep this in sync with the route guards in App.tsx — both
// reference the same `Perm` constants.
//
// `sectionKey` (optional) lets a group split its sub-items into labelled
// sections; consecutive sub-items sharing a key render under one small header.
// Items without a key render plain (no header), so existing flat groups are
// unaffected.
export type NavSubDef = { titleKey: string; url: string; perm?: string; sectionKey?: string }
export type NavLeafDef = { titleKey: string; url: string; icon: ReactNode; perm?: string }
export type NavGroupDef = { titleKey: string; icon: ReactNode; items: NavSubDef[] }
export type NavDef = NavLeafDef | NavGroupDef

export function isNavGroup(entry: NavDef): entry is NavGroupDef {
  return "items" in entry
}

export const NAV: NavDef[] = [
  { titleKey: "nav.dashboard", url: "/", icon: <LayoutDashboardIcon />, perm: Perm.StatsRead },
  { titleKey: "localChat.title", url: "/chat", icon: <MessageCircleIcon />, perm: Perm.ChatWrite },
  {
    titleKey: "nav.digitalEmployees",
    icon: <BotIcon />,
    items: [
      { titleKey: "nav.departments", url: "/departments", perm: Perm.ContactsRead },
      { titleKey: "nav.workers", url: "/workers", perm: Perm.ContactsRead },
    ],
  },
  {
    titleKey: "nav.directory",
    icon: <ContactIcon />,
    items: [
      { titleKey: "nav.users", url: "/users", perm: Perm.UsersManage },
      { titleKey: "nav.roles", url: "/roles", perm: Perm.RolesManage },
    ],
  },
  { titleKey: "nav.sessions", url: "/sessions", icon: <ActivityIcon />, perm: Perm.SessionsRead },
  { titleKey: "nav.tasks", url: "/tasks", icon: <ClockIcon />, perm: Perm.TasksRead },
  {
    titleKey: "nav.systemConfig",
    icon: <Settings2Icon />,
    items: [
      { titleKey: "nav.settings", url: "/env", perm: Perm.EnvRead },
      { titleKey: "nav.systemSettings", url: "/settings", perm: Perm.SystemConfigRead },
    ],
  },
]

// granted reports whether an entry with the given perm is reachable: ungated
// entries (perm === undefined) always are.
export function granted(perms: string[] | undefined, perm?: string): boolean {
  return perm === undefined || hasPermission(perms, perm)
}

// firstAccessiblePath returns the first navigation destination the user may
// open, in sidebar order, flattening groups to their first reachable sub-item.
// Returns undefined when nothing is accessible — the caller then shows the
// no-access landing instead of redirecting (which would loop).
export function firstAccessiblePath(perms: string[] | undefined): string | undefined {
  for (const entry of NAV) {
    if (isNavGroup(entry)) {
      const sub = entry.items.find((s) => granted(perms, s.perm))
      if (sub) return sub.url
    } else if (granted(perms, entry.perm)) {
      return entry.url
    }
  }
  return undefined
}
