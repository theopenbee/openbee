import * as React from "react"
import { useTranslation } from "react-i18next"
import { LayoutDashboardIcon, ActivityIcon, ClockIcon, MessageCircleIcon, GithubIcon, ContactIcon, Settings2Icon, PanelLeftIcon } from "lucide-react"

import { NavMain, type NavEntry } from "@/components/nav-main"
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

  // Leaf links sit at the top level; sections with sub-pages become collapsible
  // groups (icon + label + chevron) that reveal indented, plain-text children.
  const navItems = React.useMemo<NavEntry[]>(() => [
    { title: t("nav.dashboard"), url: "/", icon: <LayoutDashboardIcon /> },
    { title: t("localChat.title"), url: "/chat", icon: <MessageCircleIcon /> },
    {
      title: t("nav.directory"),
      icon: <ContactIcon />,
      items: [
        { title: t("nav.workers"), url: "/workers" },
        { title: t("nav.departments"), url: "/departments" },
      ],
    },
    { title: t("nav.sessions"), url: "/sessions", icon: <ActivityIcon /> },
    { title: t("nav.tasks"), url: "/tasks", icon: <ClockIcon /> },
    {
      title: t("nav.systemConfig"),
      icon: <Settings2Icon />,
      items: [
        { title: t("nav.settings"), url: "/env" },
        { title: t("nav.systemSettings"), url: "/settings" },
      ],
    },
  ], [t])

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
              tooltip={t("nav.collapseNav")}
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
