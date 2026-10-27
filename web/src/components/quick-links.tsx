import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  ActivityIcon,
  ChevronRightIcon,
  ClockIcon,
  KeyRoundIcon,
  NetworkIcon,
  SlidersHorizontalIcon,
  UsersIcon,
  type LucideIcon,
} from "lucide-react"
import { Panel } from "@/components/panel"

const LINKS: { to: string; labelKey: string; icon: LucideIcon }[] = [
  { to: "/workers", labelKey: "nav.workers", icon: UsersIcon },
  { to: "/departments", labelKey: "nav.departments", icon: NetworkIcon },
  { to: "/tasks", labelKey: "nav.tasks", icon: ClockIcon },
  { to: "/sessions", labelKey: "nav.sessions", icon: ActivityIcon },
  { to: "/env", labelKey: "nav.settings", icon: KeyRoundIcon },
  { to: "/settings", labelKey: "nav.systemSettings", icon: SlidersHorizontalIcon },
]

export function QuickLinks({ className }: { className?: string }) {
  const { t } = useTranslation()

  return (
    <Panel title={t("dashboard.quickAccess")} ariaLabel={t("dashboard.quickAccess")} className={className} flush>
      <nav aria-label={t("dashboard.quickAccess")}>
        <ul className="border-t border-border-subtle">
          {LINKS.map(({ to, labelKey, icon: Icon }) => (
            <li key={to} className="border-b border-border-subtle last:border-b-0">
              <Link
                to={to}
                className="flex min-h-12 items-center gap-3 px-6 text-sm text-foreground transition-colors hover:bg-surface-muted"
              >
                <Icon size={16} strokeWidth={1.75} aria-hidden="true" className="shrink-0 text-secondary" />
                <span className="min-w-0 flex-1 truncate">{t(labelKey)}</span>
                <ChevronRightIcon size={16} strokeWidth={1.75} aria-hidden="true" className="shrink-0 text-muted" />
              </Link>
            </li>
          ))}
        </ul>
      </nav>
    </Panel>
  )
}
