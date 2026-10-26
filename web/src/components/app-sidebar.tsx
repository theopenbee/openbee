import * as React from "react"
import { useTranslation } from "react-i18next"
import { useLocation } from "react-router-dom"
import { GithubIcon, PanelLeftIcon } from "lucide-react"

import { NavMain, type NavEntry } from "@/components/nav-main"
import { NAV, isNavGroup } from "@/lib/nav"
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
  const { toggleSidebar, setOpenMobile } = useSidebar()
  const { key: locationKey } = useLocation()

  React.useEffect(() => {
    setOpenMobile(false)
  }, [locationKey, setOpenMobile])

  const navItems = React.useMemo<NavEntry[]>(
    () =>
      NAV.map((entry) =>
        isNavGroup(entry)
          ? {
              title: t(entry.titleKey),
              icon: entry.icon,
              items: entry.items.map((sub) => ({
                title: t(sub.titleKey),
                url: sub.url,
                section: sub.sectionKey ? t(sub.sectionKey) : undefined,
              })),
            }
          : { title: t(entry.titleKey), url: entry.url, icon: entry.icon },
      ),
    [t],
  )

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
