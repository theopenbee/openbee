import { useEffect, useState, type CSSProperties } from "react"
import { Link, useLocation } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { GithubIcon, MenuIcon } from "lucide-react"
import { Drawer, DrawerContent } from "@/components/arc/drawer/drawer"
import { LogoFull } from "@/components/brand/logo"
import { NavMain } from "@/components/nav-main"
import { NavUser } from "@/components/nav-user"
import { cn } from "@/lib/utils"

export function AppTopbar({ className }: { className?: string }) {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  const [open, setOpen] = useState(false)

  useEffect(() => {
    setOpen(false)
  }, [pathname])

  return (
    <header className={cn("flex h-14 shrink-0 items-center gap-2 border-b border-border bg-background px-3", className)}>
      <button
        type="button"
        onClick={() => setOpen(true)}
        aria-label={t("nav.openNav", "Open navigation")}
        className="grid size-10 place-items-center rounded-control text-secondary transition-colors hover:bg-surface-muted hover:text-foreground"
      >
        <MenuIcon size={20} strokeWidth={1.75} aria-hidden="true" />
      </button>
      <Link to="/" aria-label="OpenBee" className="flex items-center rounded-control">
        <LogoFull className="h-6 w-auto" />
      </Link>
      <div className="ml-auto">
        <NavUser align="end" />
      </div>
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerContent side="left" title={t("nav.menu", "Menu")} style={{ "--drawer-size": "min(18rem, calc(100vw - 3rem))" } as CSSProperties}>
          <div className="grid gap-4">
            <NavMain onNavigate={() => setOpen(false)} />
            <a
              href="https://github.com/theopenbee/openbee"
              target="_blank"
              rel="noreferrer"
              className="flex h-9 items-center gap-3 rounded-control px-3 text-sm text-secondary transition-colors hover:bg-surface-muted hover:text-foreground"
            >
              <GithubIcon size={16} strokeWidth={1.75} aria-hidden="true" />
              GitHub
            </a>
          </div>
        </DrawerContent>
      </Drawer>
    </header>
  )
}
