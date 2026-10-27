import type { LucideIcon } from "lucide-react"
import {
  ActivityIcon,
  BotIcon,
  ClockIcon,
  LayoutDashboardIcon,
  MessageCircleIcon,
  Settings2Icon,
} from "lucide-react"

export type NavSubDef = { titleKey: string; url: string; sectionKey?: string }
export type NavLeafDef = { titleKey: string; url: string; icon: LucideIcon }
export type NavGroupDef = { titleKey: string; icon: LucideIcon; items: NavSubDef[] }
export type NavDef = NavLeafDef | NavGroupDef

export function isNavGroup(entry: NavDef): entry is NavGroupDef {
  return "items" in entry
}

export function isNavActive(pathname: string, url: string) {
  return url === "/" ? pathname === "/" : pathname === url || pathname.startsWith(`${url}/`)
}

export const NAV: NavDef[] = [
  { titleKey: "nav.dashboard", url: "/", icon: LayoutDashboardIcon },
  { titleKey: "localChat.title", url: "/chat", icon: MessageCircleIcon },
  {
    titleKey: "nav.digitalEmployees",
    icon: BotIcon,
    items: [
      { titleKey: "nav.departments", url: "/departments" },
      { titleKey: "nav.workers", url: "/workers" },
    ],
  },
  { titleKey: "nav.sessions", url: "/sessions", icon: ActivityIcon },
  { titleKey: "nav.tasks", url: "/tasks", icon: ClockIcon },
  {
    titleKey: "nav.systemConfig",
    icon: Settings2Icon,
    items: [
      { titleKey: "nav.settings", url: "/env" },
      { titleKey: "nav.systemSettings", url: "/settings" },
      { titleKey: "nav.plugins", url: "/plugins" },
    ],
  },
]
