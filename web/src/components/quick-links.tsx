import type { ReactNode } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { Users, Network, MessagesSquare, CalendarClock, KeyRound, SlidersHorizontal } from "lucide-react"
import { Panel } from "@/components/panel"
import { useMe } from "@/hooks/use-me"
import { granted, permForPath } from "@/lib/nav"

type QuickLink = {
  to: string
  labelKey: string
  icon: ReactNode
}

const LINKS: QuickLink[] = [
  { to: "/workers", labelKey: "nav.workers", icon: <Users /> },
  { to: "/departments", labelKey: "nav.departments", icon: <Network /> },
  { to: "/tasks", labelKey: "nav.tasks", icon: <CalendarClock /> },
  { to: "/sessions", labelKey: "nav.sessions", icon: <MessagesSquare /> },
  { to: "/env", labelKey: "nav.settings", icon: <KeyRound /> },
  { to: "/settings", labelKey: "nav.systemSettings", icon: <SlidersHorizontal /> },
]

export function QuickLinks() {
  const { t } = useTranslation()
  const { data: me } = useMe()

  // Only surface shortcuts the user can actually open — a link to a page that
  // would render PermissionDenied is just a dead end.
  const links = LINKS.filter((link) => granted(me?.permissions, permForPath(link.to)))
  if (links.length === 0) return null

  return (
    <Panel title={t("dashboard.quickAccess")} ariaLabel={t("dashboard.quickAccess")}>
      <nav className="grid grid-cols-2 gap-2 sm:grid-cols-3">
        {links.map(({ to, labelKey, icon }) => (
          <Link
            key={to}
            to={to}
            className="group/quick flex items-center gap-3 rounded-sm border border-border/70 px-3.5 py-3 transition-colors hover:border-brand/40 hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background"
          >
            <span className="text-muted-foreground transition-colors group-hover/quick:text-brand [&_svg]:size-[18px]">
              {icon}
            </span>
            <span className="text-sm font-medium">{t(labelKey)}</span>
          </Link>
        ))}
      </nav>
    </Panel>
  )
}
