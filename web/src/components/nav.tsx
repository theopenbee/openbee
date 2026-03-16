import { Link, useLocation } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { cn } from "@/lib/utils"
import { LanguageSwitcher } from "./language-switcher"

export function Nav() {
  const { pathname } = useLocation()
  const { t } = useTranslation()

  const links = [
    { href: "/", label: t("nav.dashboard") },
    { href: "/workers", label: t("nav.workers") },
    { href: "/executions", label: t("nav.executions") },
    { href: "/local-chat", label: t("localChat.title") },
  ]

  return (
    <nav className="border-b bg-background">
      <div className="container mx-auto flex h-14 items-center px-4">
        <Link to="/" className="mr-8 text-lg font-bold">
          RoboBee
        </Link>
        <div className="flex gap-4 flex-1">
          {links.map((link) => (
            <Link
              key={link.href}
              to={link.href}
              className={cn(
                "text-sm font-medium transition-colors hover:text-primary",
                (pathname === link.href || (link.href !== "/" && pathname.startsWith(link.href)))
                  ? "text-foreground"
                  : "text-muted-foreground"
              )}
            >
              {link.label}
            </Link>
          ))}
        </div>
        <LanguageSwitcher />
      </div>
    </nav>
  )
}
