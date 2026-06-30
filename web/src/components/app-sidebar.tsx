import * as React from "react"
import { useTranslation } from "react-i18next"
import { GithubIcon, PanelLeftIcon } from "lucide-react"

import { NavMain, type NavEntry, type NavSubItem } from "@/components/nav-main"
import { useMe } from "@/hooks/use-me"
import { NAV, granted, isNavGroup } from "@/lib/nav"
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

  // The shared NAV (lib/nav) is the single source of truth — also used by the
  // home resolver. Here we translate each entry's titleKey and apply gating:
  // ungated entries always show; gated ones are hidden unless the user holds
  // the permission. Groups keep only the sub-items the user can see and drop
  // entirely when none remain. While `me` is loading, permissions are undefined
  // and gated entries stay hidden (no flash).
  const navItems = React.useMemo<NavEntry[]>(() => {
    return NAV.reduce<NavEntry[]>((acc, entry) => {
      if (isNavGroup(entry)) {
        const visibleSubs: NavSubItem[] = entry.items
          .filter((sub) => granted(me?.permissions, sub.perm))
          .map((sub) => ({
            title: t(sub.titleKey),
            url: sub.url,
            perm: sub.perm,
            section: sub.sectionKey ? t(sub.sectionKey) : undefined,
          }))
        if (visibleSubs.length > 0) {
          acc.push({ title: t(entry.titleKey), icon: entry.icon, items: visibleSubs })
        }
      } else if (granted(me?.permissions, entry.perm)) {
        acc.push({ title: t(entry.titleKey), url: entry.url, icon: entry.icon, perm: entry.perm })
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
