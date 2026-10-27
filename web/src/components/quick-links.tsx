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
import { useSettingsDialog, type SettingsSection } from "@/hooks/use-settings-dialog"

type QuickLink = { labelKey: string; icon: LucideIcon } & ({ to: string } | { section: SettingsSection })

const LINKS: QuickLink[] = [
  { to: "/workers", labelKey: "nav.workers", icon: UsersIcon },
  { to: "/departments", labelKey: "nav.departments", icon: NetworkIcon },
  { to: "/tasks", labelKey: "nav.tasks", icon: ClockIcon },
  { to: "/sessions", labelKey: "nav.sessions", icon: ActivityIcon },
  { section: "env", labelKey: "envConfig.title", icon: KeyRoundIcon },
  { section: "system", labelKey: "systemSettings.title", icon: SlidersHorizontalIcon },
]

const rowClass =
  "flex min-h-12 w-full items-center gap-3 px-6 text-left text-sm text-foreground transition-colors hover:bg-surface-muted"

export function QuickLinks({ className }: { className?: string }) {
  const { t } = useTranslation()
  const show = useSettingsDialog()

  return (
    <Panel title={t("dashboard.quickAccess")} ariaLabel={t("dashboard.quickAccess")} className={className} flush>
      <nav aria-label={t("dashboard.quickAccess")}>
        <ul className="border-t border-border-subtle">
          {LINKS.map((link) => {
            const Icon = link.icon
            const content = (
              <>
                <Icon size={16} strokeWidth={1.75} aria-hidden="true" className="shrink-0 text-secondary" />
                <span className="min-w-0 flex-1 truncate">{t(link.labelKey)}</span>
                <ChevronRightIcon size={16} strokeWidth={1.75} aria-hidden="true" className="shrink-0 text-muted" />
              </>
            )
            return (
              <li key={link.labelKey} className="border-b border-border-subtle last:border-b-0">
                {"to" in link ? (
                  <Link to={link.to} className={rowClass}>
                    {content}
                  </Link>
                ) : (
                  <button type="button" aria-haspopup="dialog" onClick={() => show({ section: link.section })} className={rowClass}>
                    {content}
                  </button>
                )}
              </li>
            )
          })}
        </ul>
      </nav>
    </Panel>
  )
}
