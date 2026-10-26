import type { ReactNode } from "react"
import {
  LayoutDashboardIcon,
  ActivityIcon,
  ClockIcon,
  MessageCircleIcon,
  BotIcon,
  Settings2Icon,
} from "lucide-react"

// `sectionKey` (optional) lets a group split its sub-items into labelled
// sections; consecutive sub-items sharing a key render under one small header.
// Items without a key render plain (no header), so existing flat groups are
// unaffected.
export type NavSubDef = { titleKey: string; url: string; sectionKey?: string }
export type NavLeafDef = { titleKey: string; url: string; icon: ReactNode }
export type NavGroupDef = { titleKey: string; icon: ReactNode; items: NavSubDef[] }
export type NavDef = NavLeafDef | NavGroupDef

export function isNavGroup(entry: NavDef): entry is NavGroupDef {
  return "items" in entry
}

export const NAV: NavDef[] = [
  { titleKey: "nav.dashboard", url: "/", icon: <LayoutDashboardIcon /> },
  { titleKey: "localChat.title", url: "/chat", icon: <MessageCircleIcon /> },
  {
    titleKey: "nav.digitalEmployees",
    icon: <BotIcon />,
    items: [
      { titleKey: "nav.departments", url: "/departments" },
      { titleKey: "nav.workers", url: "/workers" },
    ],
  },
  { titleKey: "nav.sessions", url: "/sessions", icon: <ActivityIcon /> },
  { titleKey: "nav.tasks", url: "/tasks", icon: <ClockIcon /> },
  {
    titleKey: "nav.systemConfig",
    icon: <Settings2Icon />,
    items: [
      { titleKey: "nav.settings", url: "/env" },
      { titleKey: "nav.systemSettings", url: "/settings" },
      { titleKey: "nav.plugins", url: "/plugins" },
    ],
  },
]
