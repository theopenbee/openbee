import { lazy, Suspense, useEffect } from "react"
import { Navigate, useLocation, useParams, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  ArrowLeftIcon,
  ChevronLeftIcon,
  KeyRoundIcon,
  PuzzleIcon,
  SlidersHorizontalIcon,
  type LucideIcon,
} from "lucide-react"
import { useLeaveSettings, useOpenSettings } from "@/hooks/use-settings"
import { settingsPath, settingsTarget, type SettingsSection, type SettingsTarget } from "@/lib/routes"
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

function revealNavItem(node: HTMLButtonElement | null) {
  node?.scrollIntoView({ block: "nearest", inline: "nearest" })
}

function useEscapeToLeave(leave: () => void) {
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented || event.isComposing) return
      if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return
      leave()
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [leave])
}

export function SettingsPage() {
  const { section, plugin } = useParams()
  const [search] = useSearchParams()
  const { state } = useLocation()
  const target = settingsTarget(plugin ? "plugins" : section, plugin, search)
  if (!target) return <Navigate to={settingsPath({ section: "env" })} replace state={state} />
  return <SettingsLayout target={target} />
}

function SettingsLayout({ target }: { target: SettingsTarget }) {
  const { t } = useTranslation()
  const leave = useLeaveSettings()
  const current = SECTIONS.find((s) => s.id === target.section) ?? SECTIONS[0]
  const backLabel = t("nav.backToChat")
  useEscapeToLeave(leave)

  return (
    <div className="flex h-dvh bg-background">
      <aside className="hidden h-full w-76 shrink-0 flex-col border-r border-border md:flex">
        <div className="flex h-16 shrink-0 items-center px-3">
          <button type="button" onClick={leave} className={navItemClass}>
            <ArrowLeftIcon size={16} strokeWidth={1.75} aria-hidden="true" className="shrink-0" />
            <span className="truncate">{backLabel}</span>
          </button>
        </div>
        <SettingsNav current={current.id} className="flex flex-col gap-0.5 px-3 pb-3" />
      </aside>
      <main className="flex min-h-0 min-w-0 flex-1 flex-col">
        <header className="flex h-16 shrink-0 items-center gap-3 border-b border-border px-3 md:px-5">
          <button
            type="button"
            aria-label={backLabel}
            onClick={leave}
            className="grid size-10 shrink-0 place-items-center rounded-control text-secondary transition-colors hover:bg-surface-muted hover:text-foreground md:hidden"
          >
            <ChevronLeftIcon size={20} strokeWidth={1.75} aria-hidden="true" />
          </button>
          <h1 className="min-w-0 truncate text-base font-medium">{t(current.titleKey)}</h1>
        </header>
        <SettingsNav
          current={current.id}
          strip
          className="flex shrink-0 scroll-px-4 gap-1 overflow-x-auto border-b border-border-subtle px-4 py-3 md:hidden"
        />
        <div key={`${current.id}:${target.plugin ?? ""}`} className="min-h-0 flex-1 overflow-y-auto">
          <div className="@container mx-auto w-full max-w-4xl p-6 md:p-8">
            {!target.plugin && <p className="mb-6 text-sm text-secondary">{t(current.descriptionKey)}</p>}
            <Suspense fallback={null}>
              <SectionBody target={target} />
            </Suspense>
          </div>
        </div>
      </main>
    </div>
  )
}

function SettingsNav({ current, strip, className }: { current: SettingsSection; strip?: boolean; className: string }) {
  const { t } = useTranslation()
  const open = useOpenSettings()
  return (
    <nav aria-label={t("nav.settings")} className={className}>
      {SECTIONS.map(({ id, titleKey, navKey, icon: Icon }) => {
        const active = id === current
        return (
          <button
            key={id}
            ref={active && strip ? revealNavItem : undefined}
            type="button"
            aria-current={active ? "page" : undefined}
            onClick={() => open({ section: id })}
            className={cn(navItemClass, strip && "w-auto shrink-0", active && navActiveClass)}
          >
            <Icon size={16} strokeWidth={1.75} aria-hidden="true" className={cn("shrink-0", active && "text-accent-strong")} />
            <span className="truncate">{t(navKey ?? titleKey)}</span>
          </button>
        )
      })}
    </nav>
  )
}

function SectionBody({ target }: { target: SettingsTarget }) {
  const open = useOpenSettings()
  switch (target.section) {
    case "system":
      return <SystemSettings />
    case "plugins":
      return target.plugin ? (
        <PluginDetailSettings id={target.plugin} />
      ) : (
        <PluginsSettings
          tab={target.tab ?? "market"}
          onTabChange={(tab) => open({ section: "plugins", tab }, { replace: true })}
        />
      )
    default:
      return <EnvSettings />
  }
}
