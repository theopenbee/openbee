import { Fragment, useState, type ReactElement } from "react"
import { Link, useLocation } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { AnimatePresence, motion, useReducedMotion } from "motion/react"
import { ChevronDownIcon } from "lucide-react"
import { Tooltip } from "@/components/arc/tooltip/tooltip"
import { motionTokens } from "@/components/arc/lib/motion-tokens"
import { NAV, type NavGroupDef, type NavLeafDef, isNavActive, isNavGroup } from "@/lib/nav"
import { cn } from "@/lib/utils"

const itemClass =
  "flex h-9 w-full min-w-0 items-center gap-3 rounded-control px-3 text-sm text-secondary transition-colors duration-150 hover:bg-surface-muted hover:text-foreground"
const activeClass = "bg-accent-subtle text-foreground hover:bg-accent-subtle"

function Collapsed({ label, children }: { label: string; children: ReactElement }) {
  return <Tooltip content={label} side="right">{children}</Tooltip>
}

function LeafItem({ item, collapsed, onNavigate }: { item: NavLeafDef; collapsed: boolean; onNavigate?: () => void }) {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  const active = isNavActive(pathname, item.url)
  const Icon = item.icon
  const label = t(item.titleKey)
  const link = (
    <Link
      to={item.url}
      onClick={onNavigate}
      aria-current={active ? "page" : undefined}
      aria-label={collapsed ? label : undefined}
      className={cn(itemClass, active && activeClass, collapsed && "justify-center px-0")}
    >
      <Icon size={16} strokeWidth={1.75} aria-hidden="true" className={cn("shrink-0", active && "text-accent-strong")} />
      {!collapsed && <span className="truncate">{label}</span>}
    </Link>
  )
  return collapsed ? <Collapsed label={label}>{link}</Collapsed> : link
}

function GroupItem({ group, collapsed, onNavigate }: { group: NavGroupDef; collapsed: boolean; onNavigate?: () => void }) {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  const reduced = useReducedMotion()
  const childActive = group.items.some((sub) => isNavActive(pathname, sub.url))
  const [open, setOpen] = useState(childActive)
  const [lastPath, setLastPath] = useState(pathname)
  if (lastPath !== pathname) {
    setLastPath(pathname)
    if (childActive && !open) setOpen(true)
  }
  const Icon = group.icon
  const label = t(group.titleKey)

  if (collapsed) {
    const first = group.items[0]
    return (
      <Collapsed label={label}>
        <Link
          to={first.url}
          onClick={onNavigate}
          aria-label={label}
          className={cn(itemClass, "justify-center px-0", childActive && activeClass)}
        >
          <Icon size={16} strokeWidth={1.75} aria-hidden="true" className={cn("shrink-0", childActive && "text-accent-strong")} />
        </Link>
      </Collapsed>
    )
  }

  return (
    <div>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className={cn(itemClass, childActive && !open && activeClass)}
      >
        <Icon size={16} strokeWidth={1.75} aria-hidden="true" className={cn("shrink-0", childActive && "text-accent-strong")} />
        <span className="truncate">{label}</span>
        <ChevronDownIcon
          size={16}
          strokeWidth={1.75}
          aria-hidden="true"
          className={cn("ml-auto shrink-0 text-muted transition-transform duration-200", open && "rotate-180")}
        />
      </button>
      <AnimatePresence initial={false}>
        {open && (
          <motion.div
            key="items"
            className="overflow-hidden"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={reduced ? { duration: 0 } : { height: motionTokens.spring.smooth, opacity: { duration: motionTokens.duration.fast } }}
          >
            <ul className="grid gap-0.5 pt-0.5 pl-7">
              {group.items.map((sub, index) => {
                const active = isNavActive(pathname, sub.url)
                const showSection = sub.sectionKey && sub.sectionKey !== group.items[index - 1]?.sectionKey
                return (
                  <Fragment key={sub.url}>
                    {showSection && <li className="px-3 pt-2 pb-1 text-xs text-muted">{t(sub.sectionKey!)}</li>}
                    <li>
                      <Link
                        to={sub.url}
                        onClick={onNavigate}
                        aria-current={active ? "page" : undefined}
                        className={cn(itemClass, "h-8", active && activeClass)}
                      >
                        <span className="truncate">{t(sub.titleKey)}</span>
                      </Link>
                    </li>
                  </Fragment>
                )
              })}
            </ul>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}

export function NavMain({ collapsed = false, onNavigate }: { collapsed?: boolean; onNavigate?: () => void }) {
  const { t } = useTranslation()
  return (
    <nav aria-label={t("nav.primary", "Primary")}>
      <ul className="grid gap-0.5">
        {NAV.map((entry) => (
          <li key={entry.titleKey}>
            {isNavGroup(entry) ? (
              <GroupItem group={entry} collapsed={collapsed} onNavigate={onNavigate} />
            ) : (
              <LeafItem item={entry} collapsed={collapsed} onNavigate={onNavigate} />
            )}
          </li>
        ))}
      </ul>
    </nav>
  )
}
