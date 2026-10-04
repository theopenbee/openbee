import { useMemo, type ReactNode } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  Users,
  Network,
  MessagesSquare,
  CalendarClock,
  KeyRound,
  SlidersHorizontal,
  ChevronRight,
} from "lucide-react"
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
  const links = useMemo(
    () => LINKS.filter((link) => granted(me?.permissions, permForPath(link.to))),
    [me?.permissions],
  )
  if (links.length === 0) return null

  // Compact link cells divided by hairlines inside one flush body (no outlined
  // tiles nested in the panel). Each cell draws only its top and left edge and
  // the grid is pulled up/left by 1px under overflow-hidden, so the outer edges
  // vanish into the body's ring and a partial last row leaves no stray lines.
  return (
    <Panel title={t("dashboard.quickAccess")} ariaLabel={t("dashboard.quickAccess")} flush>
      <nav aria-label={t("dashboard.quickAccess")} className="overflow-hidden rounded-sm">
        <ul className="-mt-px -ml-px grid grid-cols-2 sm:grid-cols-3">
          {links.map(({ to, labelKey, icon }) => (
            <li key={to} className="min-w-0 border-t border-l border-hairline">
              <Link
                to={to}
                className="group/quick flex h-11 items-center gap-2.5 px-4 text-sm font-medium text-foreground transition-colors hover:bg-accent hover:text-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
              >
                <span className="shrink-0 text-muted-foreground transition-colors group-hover/quick:text-foreground [&_svg]:size-4">
                  {icon}
                </span>
                <span className="min-w-0 flex-1 truncate">{t(labelKey)}</span>
                <ChevronRight
                  className="size-4 shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover/quick:opacity-100 group-focus-visible/quick:opacity-100"
                  aria-hidden
                />
              </Link>
            </li>
          ))}
        </ul>
      </nav>
    </Panel>
  )
}
