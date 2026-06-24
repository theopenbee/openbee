import * as React from "react"
import { useTranslation } from "react-i18next"
import { LayoutDashboardIcon, BotIcon, ActivityIcon, ClockIcon, MessageCircleIcon, GithubIcon, Building2Icon, SettingsIcon, PanelLeftIcon } from "lucide-react"

import { NavMain } from "@/components/nav-main"
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

  const navTop = React.useMemo(() => [
    { title: t("nav.dashboard"), url: "/", icon: <LayoutDashboardIcon /> },
    { title: t("localChat.title"), url: "/chat", icon: <MessageCircleIcon /> },
  ], [t])

  const navDirectory = React.useMemo(() => [
    { title: t("nav.workers"), url: "/workers", icon: <BotIcon /> },
    { title: t("nav.departments"), url: "/departments", icon: <Building2Icon /> },
  ], [t])

  const navMain = React.useMemo(() => [
    { title: t("nav.sessions"), url: "/sessions", icon: <ActivityIcon /> },
    { title: t("nav.tasks"), url: "/tasks", icon: <ClockIcon /> },
  ], [t])

  const navSystemConfig = React.useMemo(() => [
    { title: t("nav.settings"), url: "/env", icon: <SettingsIcon /> },
    { title: t("nav.systemSettings"), url: "/settings", icon: <SettingsIcon /> },
  ], [t])

  return (
    <Sidebar variant="sidebar" collapsible="icon" {...props}>
      <SidebarContent className="pt-2">
        <NavMain items={navTop} />
        <NavMain label={t("nav.directory")} items={navDirectory} />
        <NavMain items={navMain} />
        <NavMain label={t("nav.systemConfig")} items={navSystemConfig} />
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
