import * as React from "react"
import { useTranslation } from "react-i18next"
import { LayoutDashboardIcon, ActivityIcon, ClockIcon, MessageCircleIcon, GithubIcon, ContactIcon, Settings2Icon, PanelLeftIcon, UsersIcon, ShieldCheckIcon } from "lucide-react"

import { NavMain, type NavEntry, type NavSubItem } from "@/components/nav-main"
import { useMe } from "@/hooks/use-me"
import { Perm, hasPermission } from "@/lib/permissions"
import { NavSecondary } from "@/components/nav-secondary"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar"

const navSecondary = [
  {
    title: "GitHub",
    url: "https://github.com/theopenbee/openbee",
    icon: <GithubIcon />,
  },
]

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  const { t } = useTranslation()
  const { toggleSidebar } = useSidebar()
  const { data: me } = useMe()

  // Leaf links sit at the top level; sections with sub-pages become collapsible
  // groups (icon + label + chevron) that reveal indented, plain-text children.
  // Entries (and group sub-items) carry an optional `perm`; those are hidden
  // unless the current user holds the permission. While `me` is loading,
  // permissions are undefined and gated entries stay hidden (no flash).
  const navItems = React.useMemo<NavEntry[]>(() => {
    const allItems: NavEntry[] = [
      { title: t("nav.dashboard"), url: "/", icon: <LayoutDashboardIcon /> },
      { title: t("localChat.title"), url: "/chat", icon: <MessageCircleIcon /> },
      {
        title: t("nav.directory"),
        icon: <ContactIcon />,
        items: [
          { title: t("nav.departments"), url: "/departments", perm: Perm.DepartmentsRead },
          { title: t("nav.workers"), url: "/workers", perm: Perm.WorkersRead },
        ],
      },
      { title: t("nav.sessions"), url: "/sessions", icon: <ActivityIcon />, perm: Perm.SessionsRead },
      { title: t("nav.tasks"), url: "/tasks", icon: <ClockIcon />, perm: Perm.TasksRead },
      {
        title: t("nav.systemConfig"),
        icon: <Settings2Icon />,
        items: [
          { title: t("nav.settings"), url: "/env", perm: Perm.EnvRead },
          { title: t("nav.systemSettings"), url: "/settings", perm: Perm.SystemConfigRead },
        ],
      },
      { title: t("nav.users"), url: "/users", icon: <UsersIcon />, perm: Perm.UsersManage },
      { title: t("nav.roles"), url: "/roles", icon: <ShieldCheckIcon />, perm: Perm.RolesManage },
    ]

    const granted = (perm?: string) => perm === undefined || hasPermission(me?.permissions, perm)

    // Keep ungated entries; for gated ones, hide when the user lacks the perm.
    // Groups keep only the sub-items the user can see, and drop entirely when
    // none remain.
    return allItems.reduce<NavEntry[]>((acc, item) => {
      if ("items" in item) {
        const visibleSubs: NavSubItem[] = item.items.filter((sub) => granted(sub.perm))
        if (visibleSubs.length > 0) acc.push({ ...item, items: visibleSubs })
      } else if (granted(item.perm)) {
        acc.push(item)
      }
      return acc
    }, [])
  }, [t, me])

  return (
    <Sidebar variant="sidebar" collapsible="icon" {...props}>
      <SidebarContent className="pt-2">
        <NavMain items={navItems} />
        <NavSecondary items={navSecondary} className="mt-auto" />
      </SidebarContent>
      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              onClick={toggleSidebar}
              tooltip={t("nav.expandNav")}
            >
              <PanelLeftIcon />
              <span>{t("nav.collapseNav")}</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  )
}
