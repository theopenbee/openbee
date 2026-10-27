import { useState, type ReactElement } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { GithubIcon, PanelLeftCloseIcon, PanelLeftOpenIcon } from "lucide-react"
import { Tooltip } from "@/components/arc/tooltip/tooltip"
import { LogoFull, LogoMark } from "@/components/brand/logo"
import { NavMain } from "@/components/nav-main"
import { NavUser } from "@/components/nav-user"
import { cn } from "@/lib/utils"

const COLLAPSED_KEY = "openbee.nav.collapsed"
const GITHUB_URL = "https://github.com/theopenbee/openbee"

const footerItemClass =
  "flex h-9 w-full min-w-0 items-center gap-3 rounded-control px-3 text-sm text-secondary transition-colors duration-150 hover:bg-surface-muted hover:text-foreground"

function FooterAction({ collapsed, label, children }: { collapsed: boolean; label: string; children: ReactElement }) {
  return collapsed ? <Tooltip content={label} side="right">{children}</Tooltip> : children
}

export function AppSidebar({ className }: { className?: string }) {
  const { t } = useTranslation()
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem(COLLAPSED_KEY) === "1")

  const toggle = () => {
    setCollapsed((value) => {
      localStorage.setItem(COLLAPSED_KEY, value ? "0" : "1")
      return !value
    })
  }

  const toggleLabel = collapsed ? t("nav.expandNav") : t("nav.collapseNav")
  const ToggleIcon = collapsed ? PanelLeftOpenIcon : PanelLeftCloseIcon

  return (
    <aside
      className={cn(
        "flex h-full shrink-0 flex-col border-r border-border bg-background transition-[width] duration-200",
        collapsed ? "w-16" : "w-60",
        className,
      )}
    >
      <div className={cn("flex h-16 shrink-0 items-center", collapsed ? "justify-center" : "px-5")}>
        <Link to="/" aria-label="OpenBee" className="flex items-center rounded-control">
          {collapsed ? <LogoMark className="size-8" /> : <LogoFull className="h-7 w-auto" />}
        </Link>
      </div>
      <div className={cn("min-h-0 flex-1 overflow-y-auto py-2", "px-3")}>
        <NavMain collapsed={collapsed} />
      </div>
      <div className="grid shrink-0 gap-0.5 border-t border-border-subtle px-3 py-3">
        <FooterAction collapsed={collapsed} label="GitHub">
          <a
            href={GITHUB_URL}
            target="_blank"
            rel="noreferrer"
            aria-label={collapsed ? "GitHub" : undefined}
            className={cn(footerItemClass, collapsed && "justify-center px-0")}
          >
            <GithubIcon size={16} strokeWidth={1.75} aria-hidden="true" className="shrink-0" />
            {!collapsed && <span className="truncate">GitHub</span>}
          </a>
        </FooterAction>
        <FooterAction collapsed={collapsed} label={toggleLabel}>
          <button
            type="button"
            onClick={toggle}
            aria-label={collapsed ? toggleLabel : undefined}
            className={cn(footerItemClass, collapsed && "justify-center px-0")}
          >
            <ToggleIcon size={16} strokeWidth={1.75} aria-hidden="true" className="shrink-0" />
            {!collapsed && <span className="truncate">{toggleLabel}</span>}
          </button>
        </FooterAction>
        <div className={cn("pt-2", collapsed && "flex justify-center")}>
          <NavUser showName={!collapsed} align="start" />
        </div>
      </div>
    </aside>
  )
}
