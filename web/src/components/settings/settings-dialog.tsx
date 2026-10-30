import { lazy, Suspense, useCallback, useMemo, useState, type CSSProperties, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { KeyRoundIcon, PuzzleIcon, SlidersHorizontalIcon, type LucideIcon } from "lucide-react"
import { PresenceContext } from "motion/react"
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog"
import { SettingsDialogContext, type SettingsSection, type SettingsTarget } from "@/hooks/use-settings-dialog"
import { navActiveClass, navItemClass } from "@/lib/styles"
import { cn } from "@/lib/utils"

const EnvSettings = lazy(() => import("@/components/settings/env-settings").then((m) => ({ default: m.EnvSettings })))
const SystemSettings = lazy(() =>
  import("@/components/settings/system-settings").then((m) => ({ default: m.SystemSettings })),
)
const PluginsSettings = lazy(() =>
  import("@/components/settings/plugins-settings").then((m) => ({ default: m.PluginsSettings })),
)
const PluginDetailSettings = lazy(() =>
  import("@/components/settings/plugin-detail-settings").then((m) => ({ default: m.PluginDetailSettings })),
)

const SECTIONS: { id: SettingsSection; titleKey: string; navKey?: string; descriptionKey: string; icon: LucideIcon }[] = [
  { id: "env", titleKey: "envConfig.title", navKey: "envConfig.navTitle", descriptionKey: "envConfig.pageDescription", icon: KeyRoundIcon },
  { id: "system", titleKey: "systemSettings.title", descriptionKey: "systemSettings.subtitle", icon: SlidersHorizontalIcon },
  { id: "plugins", titleKey: "plugins.title", descriptionKey: "plugins.subtitle", icon: PuzzleIcon },
]

const DIALOG_STYLE: CSSProperties = {
  width: "min(calc(100vw - 2rem), 72rem)",
  height: "min(calc(100dvh - 2rem), 52rem)",
  overflow: "hidden",
}

function revealNavItem(node: HTMLButtonElement | null) {
  node?.scrollIntoView({ block: "nearest", inline: "nearest" })
}

export function SettingsDialogProvider({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false)
  const [target, setTarget] = useState<SettingsTarget>({ section: "env" })

  const show = useCallback((next: SettingsTarget) => {
    setTarget(next)
    setOpen(true)
  }, [])
  const close = useCallback(() => setOpen(false), [])
  const api = useMemo(() => ({ show, close }), [show, close])

  return (
    <SettingsDialogContext.Provider value={api}>
      {children}
      <SettingsDialog open={open} onOpenChange={setOpen} target={target} onNavigate={show} />
    </SettingsDialogContext.Provider>
  )
}

function SettingsDialog({
  open,
  onOpenChange,
  target,
  onNavigate,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  target: SettingsTarget
  onNavigate: (target: SettingsTarget) => void
}) {
  const { t } = useTranslation()
  const current = SECTIONS.find((s) => s.id === target.section) ?? SECTIONS[0]

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        title={t(current.titleKey)}
        description={t(current.descriptionKey)}
        className="grid grid-cols-[minmax(0,1fr)] grid-rows-[auto_minmax(0,1fr)]"
        style={DIALOG_STYLE}
      >
        <div className="-m-6 flex h-[calc(100%+3rem)] flex-col md:flex-row">
          <nav
            aria-label={t("nav.settings")}
            className="flex shrink-0 scroll-px-4 gap-1 overflow-x-auto border-b border-border-subtle px-4 py-3 md:w-56 md:flex-col md:gap-0.5 md:overflow-visible md:border-r md:border-b-0 md:p-4"
          >
            {SECTIONS.map(({ id, titleKey, navKey, icon: Icon }) => {
              const active = id === current.id
              return (
                <button
                  key={id}
                  ref={active ? revealNavItem : undefined}
                  type="button"
                  aria-current={active ? "page" : undefined}
                  onClick={() => onNavigate({ section: id })}
                  className={cn(navItemClass, "w-auto shrink-0 md:w-full", active && navActiveClass)}
                >
                  <Icon size={16} strokeWidth={1.75} aria-hidden="true" className={cn("shrink-0", active && "text-accent-strong")} />
                  <span className="truncate">{t(navKey ?? titleKey)}</span>
                </button>
              )
            })}
          </nav>
          <div
            key={`${current.id}:${target.plugin ?? ""}`}
            className="@container min-h-0 min-w-0 flex-1 overflow-y-auto p-6"
          >
            <PresenceContext.Provider value={null}>
              <Suspense fallback={null}>
                <SectionBody target={target} />
              </Suspense>
            </PresenceContext.Provider>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function SectionBody({ target }: { target: SettingsTarget }) {
  switch (target.section) {
    case "system":
      return <SystemSettings />
    case "plugins":
      return target.plugin ? <PluginDetailSettings id={target.plugin} /> : <PluginsSettings initialTab={target.tab} />
    default:
      return <EnvSettings />
  }
}
