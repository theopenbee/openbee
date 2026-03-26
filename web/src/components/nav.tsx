import { Link, useLocation } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { LayoutDashboard, Bot, Activity, MessageCircle, BookOpen } from "lucide-react"
import { cn } from "@/lib/utils"
import { LanguageSwitcher } from "./language-switcher"
import { ThemeSwitcher } from "./theme-switcher"

export function Nav() {
  const { pathname } = useLocation()
  const { t } = useTranslation()

  const links = [
    { href: "/", label: t("nav.dashboard"), icon: LayoutDashboard },
    { href: "/workers", label: t("nav.workers"), icon: Bot },
    { href: "/executions", label: t("nav.executions"), icon: Activity },
    { href: "/local-chat", label: t("localChat.title"), icon: MessageCircle },
    { href: "/skills", label: t("nav.skills"), icon: BookOpen },
  ]

  const isActive = (href: string) =>
    pathname === href || (href !== "/" && pathname.startsWith(href))

  return (
    <nav className="border-b border-primary/20 bg-card">
      <div className="max-w-7xl mx-auto flex h-16 items-center px-6">
        <Link to="/" className="mr-8 flex items-center gap-2">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" className="text-primary">
            <path
              d="M12 2L20 7V17L12 22L4 17V7L12 2Z"
              stroke="currentColor"
              strokeWidth="1.5"
              fill="currentColor"
              fillOpacity="0.15"
            />
          </svg>
          <span className="text-lg font-bold tracking-tight text-foreground">
            OpenBee
          </span>
        </Link>

        <div className="flex gap-1 flex-1 overflow-x-auto">
          {links.map((link) => {
            const Icon = link.icon
            const active = isActive(link.href)
            return (
              <Link
                key={link.href}
                to={link.href}
                className={cn(
                  "flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-colors whitespace-nowrap",
                  active
                    ? "bg-primary/15 text-primary"
                    : "text-muted-foreground hover:text-foreground hover:bg-foreground/5"
                )}
              >
                <Icon className="h-4 w-4 shrink-0" />
                {link.label}
              </Link>
            )
          })}
        </div>

        <ThemeSwitcher />
        <LanguageSwitcher />
      </div>
    </nav>
  )
}
