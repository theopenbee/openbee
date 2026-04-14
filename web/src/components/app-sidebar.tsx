import * as React from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { LayoutDashboardIcon, BotIcon, ActivityIcon, ClockIcon, MessageCircleIcon, GithubIcon, Building2Icon } from "lucide-react"

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

  const navDirectory = React.useMemo(() => [
    { title: t("nav.workers"), url: "/workers", icon: <BotIcon /> },
    { title: t("nav.departments"), url: "/departments", icon: <Building2Icon /> },
  ], [t])

  const navMain = React.useMemo(() => [
    { title: t("nav.dashboard"), url: "/", icon: <LayoutDashboardIcon /> },
    { title: t("localChat.title"), url: "/chat", icon: <MessageCircleIcon /> },
    { title: t("nav.executions"), url: "/sessions", icon: <ActivityIcon /> },
    { title: t("nav.tasks"), url: "/tasks", icon: <ClockIcon /> },
  ], [t])

  return (
    <Sidebar variant="inset" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" render={<Link to="/" />}>
              <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none">
                  <path
                    d="M12 2L20 7V17L12 22L4 17V7L12 2Z"
                    stroke="currentColor"
                    strokeWidth="1.5"
                    fill="currentColor"
                    fillOpacity="0.3"
                  />
                </svg>
              </div>
              <div className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-semibold">OpenBee</span>
                <span className="truncate text-xs text-muted-foreground">AI Workers</span>
              </div>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <NavMain label={t("nav.directory")} items={navDirectory} />
        <NavMain items={navMain} />
        <NavSecondary items={navSecondary} className="mt-auto" />
      </SidebarContent>
      <SidebarFooter>
        <NavUser username={username} />
      </SidebarFooter>
    </Sidebar>
  )
}
