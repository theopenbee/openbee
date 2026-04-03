import { Link, useLocation, useNavigate } from "react-router-dom"
import { useState } from "react"
import {
  LayoutDashboard, Bot, Activity, Clock, MessageCircle,
  LogOut, Sun, Moon,
} from "lucide-react"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { clearTokens, getUsername } from "@/lib/auth"
import { cn } from "@/lib/utils"

type Theme = "dark" | "light"

function getStoredTheme(): Theme {
  const stored = localStorage.getItem("theme")
  return stored === "light" ? "light" : "dark"
}

const NAV_ITEMS = [
  { href: "/", label: "Dashboard", icon: LayoutDashboard, exact: true },
  { href: "/workers", label: "Workers", icon: Bot, exact: false },
  { href: "/executions", label: "Sessions", icon: Activity, exact: false },
  { href: "/tasks", label: "Scheduled Tasks", icon: Clock, exact: false },
  { href: "/local-chat", label: "Local Chat", icon: MessageCircle, exact: false },
]

export function AppSidebar() {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const username = getUsername()
  const [theme, setTheme] = useState<Theme>(getStoredTheme)

  function isActive(href: string, exact: boolean) {
    if (exact) return pathname === href
    return pathname === href || pathname.startsWith(href + "/")
  }

  function toggleTheme() {
    const next: Theme = theme === "dark" ? "light" : "dark"
    document.documentElement.classList.remove("dark", "light")
    document.documentElement.classList.add(next)
    localStorage.setItem("theme", next)
    setTheme(next)
  }

  function handleLogout() {
    clearTokens()
    navigate("/login", { replace: true })
  }

  const initial = username[0]?.toUpperCase() ?? "A"

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" render={<Link to="/" />}>
                <svg
                  width="24"
                  height="24"
                  viewBox="0 0 24 24"
                  fill="none"
                  className="shrink-0 text-primary"
                >
                  <path
                    d="M12 2L20 7V17L12 22L4 17V7L12 2Z"
                    stroke="currentColor"
                    strokeWidth="1.5"
                    fill="currentColor"
                    fillOpacity="0.15"
                  />
                </svg>
                <span className="text-base font-bold tracking-tight">OpenBee</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              {NAV_ITEMS.map(({ href, label, icon: Icon, exact }) => (
                <SidebarMenuItem key={href}>
                  <SidebarMenuButton
                    render={<Link to={href} />}
                    isActive={isActive(href, exact)}
                    tooltip={label}
                  >
                    <Icon />
                    <span>{label}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              onClick={toggleTheme}
              tooltip={theme === "dark" ? "Light mode" : "Dark mode"}
            >
              {theme === "dark" ? <Sun /> : <Moon />}
              <span>{theme === "dark" ? "Light mode" : "Dark mode"}</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
          <SidebarMenuItem>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <SidebarMenuButton tooltip={username}>
                    <div
                      className={cn(
                        "flex h-6 w-6 shrink-0 items-center justify-center",
                        "rounded-full bg-primary/20 text-xs font-semibold text-primary"
                      )}
                    >
                      {initial}
                    </div>
                    <span className="truncate font-medium">{username}</span>
                  </SidebarMenuButton>
                }
              />
              <DropdownMenuContent side="top" align="start" className="w-48">
                <DropdownMenuItem onClick={handleLogout}>
                  <LogOut className="mr-2 h-4 w-4" />
                  Logout
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  )
}
