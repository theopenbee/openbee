import type { LucideIcon } from "lucide-react"
import { BotIcon, MessageCircleIcon } from "lucide-react"

export type NavSubDef = { titleKey: string; url: string; sectionKey?: string }
export type NavLeafDef = { titleKey: string; url: string; icon: LucideIcon }
export type NavGroupDef = { titleKey: string; icon: LucideIcon; items: NavSubDef[] }
export type NavDef = NavLeafDef | NavGroupDef

export function isNavGroup(entry: NavDef): entry is NavGroupDef {
  return "items" in entry
}

export function isNavActive(pathname: string, url: string) {
  return pathname === url || pathname.startsWith(`${url}/`)
}

export const NAV: NavDef[] = [
  { titleKey: "localChat.title", url: "/chat", icon: MessageCircleIcon },
  {
    titleKey: "nav.digitalEmployees",
    icon: BotIcon,
    items: [
      { titleKey: "nav.departments", url: "/departments" },
      { titleKey: "nav.workers", url: "/workers" },
    ],
  },
]
