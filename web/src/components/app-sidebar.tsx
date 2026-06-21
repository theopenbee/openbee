import * as React from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { LayoutDashboardIcon, BotIcon, ActivityIcon, ClockIcon, MessageCircleIcon, GithubIcon, Building2Icon, SettingsIcon } from "lucide-react"

import { LogoFull } from "@/components/brand/logo"
import { NavMain } from "@/components/nav-main"
import { NavSecondary } from "@/components/nav-secondary"
import { NavUser } from "@/components/nav-user"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { getStoredUsername } from "@/lib/auth"

const navSecondary = [
  {
    title: "GitHub",
    url: "https://github.com/theopenbee/openbee",
    icon: <GithubIcon />,
  },
]

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  const { t } = useTranslation()
  const username = getStoredUsername() ?? "User"

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
    <Sidebar variant="inset" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" render={<Link to="/" />}>
              <LogoFull className="!h-8 !w-auto" />
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <NavMain items={navTop} />
        <NavMain label={t("nav.directory")} items={navDirectory} />
        <NavMain items={navMain} />
        <NavMain label={t("nav.systemConfig")} items={navSystemConfig} />
        <NavSecondary items={navSecondary} className="mt-auto" />
      </SidebarContent>
      <SidebarFooter>
        <NavUser username={username} />
      </SidebarFooter>
    </Sidebar>
  )
}
