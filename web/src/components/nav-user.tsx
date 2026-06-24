import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  Avatar,
  AvatarFallback,
} from "@/components/ui/avatar"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar"
import { ChevronsUpDownIcon, ChevronDownIcon, LogOutIcon, SunIcon, MoonIcon } from "lucide-react"
import { clearTokens } from "@/lib/auth"
import { useThemeToggle } from "@/hooks/use-theme-toggle"

export function NavUser({
  username,
  variant = "sidebar",
}: {
  username: string
  /** `sidebar` renders a full-width sidebar row; `bar` renders a compact topbar button. */
  variant?: "sidebar" | "bar"
}) {
  const { isMobile } = useSidebar()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const { theme, toggle: toggleTheme } = useThemeToggle()

  const initials = username.slice(0, 2).toUpperCase()
  const avatar = (
    <Avatar className="size-8 rounded-sm">
      <AvatarFallback className="rounded-sm">{initials}</AvatarFallback>
    </Avatar>
  )

  const handleLogout = () => {
    clearTokens()
    navigate("/login", { replace: true })
  }

  // The dropdown opens downward in the topbar and on mobile, but to the side
  // when anchored to the desktop sidebar row.
  const menuSide = variant === "bar" || isMobile ? "bottom" : "right"

  const menuContent = (
    <DropdownMenuContent
      className="min-w-48 rounded-sm"
      side={menuSide}
      align="end"
      sideOffset={4}
    >
      <DropdownMenuGroup>
        <DropdownMenuLabel className="p-0 font-normal">
          <div className="flex items-center gap-2 px-1 py-1.5 text-left text-sm">
            {avatar}
            <span className="truncate font-medium">{username}</span>
          </div>
        </DropdownMenuLabel>
      </DropdownMenuGroup>
      <DropdownMenuSeparator />
      <DropdownMenuItem onClick={toggleTheme}>
        {theme === "dark" ? <SunIcon className="size-4" /> : <MoonIcon className="size-4" />}
        {theme === "dark" ? t("theme.light", "Light mode") : t("theme.dark", "Dark mode")}
      </DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem onClick={handleLogout}>
        <LogOutIcon className="size-4" />
        {t("login.logout", "Log out")}
      </DropdownMenuItem>
    </DropdownMenuContent>
  )

  if (variant === "bar") {
    return (
      <DropdownMenu>
        <DropdownMenuTrigger className="flex items-center gap-2 rounded-sm px-1.5 py-1 text-sm outline-hidden transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring aria-expanded:bg-muted">
          {avatar}
          <span className="hidden max-w-32 truncate font-medium sm:inline-block">{username}</span>
          <ChevronDownIcon className="size-4 text-muted-foreground" />
        </DropdownMenuTrigger>
        {menuContent}
      </DropdownMenu>
    )
  }

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <SidebarMenuButton size="lg" className="aria-expanded:bg-muted" />
            }
          >
            {avatar}
            <div className="grid flex-1 text-left text-sm leading-tight">
              <span className="truncate font-medium">{username}</span>
            </div>
            <ChevronsUpDownIcon className="ml-auto size-4" />
          </DropdownMenuTrigger>
          {menuContent}
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
