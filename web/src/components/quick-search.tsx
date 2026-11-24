import { useMemo, type KeyboardEvent, type ReactNode } from "react"
import * as DialogPrimitive from "@radix-ui/react-dialog"
import { AnimatePresence, motion, useReducedMotion, type Transition } from "motion/react"
import { useTranslation } from "react-i18next"
import { useLocation, useMatch, useNavigate } from "react-router-dom"
import {
  BotIcon,
  InfoIcon,
  KeyboardIcon,
  KeyRoundIcon,
  ListChecksIcon,
  PlusIcon,
  PuzzleIcon,
  ShieldCheckIcon,
  SlidersHorizontalIcon,
  SquarePenIcon,
  VariableIcon,
  type LucideIcon,
} from "lucide-react"
import { EngineIcon } from "@/components/agent-icons/engine-icon"
import { CommandPalette, type CommandItem } from "@/components/arc/command-palette/command-palette"
import { motionTokens } from "@/components/arc/lib/motion-tokens"
import { matchesShortcut, shortcutTokens, usePlatform } from "@/components/arc/shortcut-recorder/shortcut-recorder"
import { LogoMarkSmall } from "@/components/brand/logo"
import { useHasCommand } from "@/hooks/use-command"
import { useReturnFocus } from "@/hooks/use-return-focus"
import { useConversations } from "@/hooks/use-conversations"
import { usePluginCatalog, usePlugins } from "@/hooks/use-plugins"
import { useOpenSettings } from "@/hooks/use-settings"
import { useWorkers } from "@/hooks/use-workers"
import { BEE_CONVERSATION, conversationPath, visibleConversations, workerIdOf } from "@/lib/conversations"
import { quickSearchEntries, type QuickSearchEntry, type QuickSearchTarget } from "@/lib/quick-search"
import { CHAT_PATH, panelToSearch, resolveConversationRoute, type PanelSection, type SettingsSection } from "@/lib/routes"
import { requestComposerFocus, runCommand, SHORTCUTS, type CommandId } from "@/lib/shortcuts"
import type { ConversationId, Worker } from "@/lib/types"

const ICON_SIZE = 16

const PANEL_ICONS: Record<PanelSection, LucideIcon> = {
  constraints: ListChecksIcon,
  permissions: ShieldCheckIcon,
  env: VariableIcon,
}

const SETTINGS_ICONS: Record<SettingsSection, LucideIcon> = {
  env: KeyRoundIcon,
  system: SlidersHorizontalIcon,
  plugins: PuzzleIcon,
  about: InfoIcon,
}

const COMMAND_ICONS: Partial<Record<CommandId, LucideIcon>> = {
  newConversation: SquarePenIcon,
  showShortcuts: KeyboardIcon,
}

function lucide(Icon: LucideIcon) {
  return <Icon size={ICON_SIZE} strokeWidth={1.75} />
}

function iconOf({ target, icon }: QuickSearchEntry): ReactNode {
  switch (target.kind) {
    case "conversation":
      return target.conversation === BEE_CONVERSATION ? <LogoMarkSmall /> : lucide(BotIcon)
    case "panel":
      return lucide(PANEL_ICONS[target.section])
    case "newWorker":
      return lucide(PlusIcon)
    case "command":
      return lucide(COMMAND_ICONS[target.command] ?? KeyboardIcon)
    case "settings":
      return lucide(SETTINGS_ICONS[target.section])
    case "plugin":
      return icon ? <EngineIcon icon={icon} /> : lucide(PuzzleIcon)
  }
}

function useCurrentConversation(workers: Map<string, Worker>): ConversationId | null {
  const param = useMatch("/chat/:conversation")?.params.conversation
  return resolveConversationRoute(param, new Set(workers.keys()))
}

interface QuickSearchProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function QuickSearch({ open, onOpenChange }: QuickSearchProps) {
  const { t } = useTranslation()
  const reduced = useReducedMotion()
  const platform = usePlatform()
  const focus = useReturnFocus(open)
  const fade: Transition = reduced
    ? { duration: motionTokens.duration.instant }
    : { duration: motionTokens.duration.standard, ease: [...motionTokens.ease.enter] }
  const leave: Transition = { duration: motionTokens.duration.fast, ease: [...motionTokens.ease.standard] }

  const done = (focusMoved: boolean) => {
    if (focusMoved) focus.keepFocus()
    onOpenChange(false)
  }

  const closeOnToggle = (event: KeyboardEvent) => {
    if (!matchesShortcut(event.nativeEvent, SHORTCUTS.quickSearch, platform)) return
    event.preventDefault()
    done(false)
  }

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <AnimatePresence>
        {open && (
          <DialogPrimitive.Portal forceMount>
            <DialogPrimitive.Overlay asChild forceMount>
              <motion.div
                className="fixed inset-0 z-50 bg-background/70 backdrop-blur-sm"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0, transition: reduced ? { duration: 0 } : leave }}
                transition={fade}
              />
            </DialogPrimitive.Overlay>
            <DialogPrimitive.Content
              asChild
              forceMount
              aria-describedby={undefined}
              onEscapeKeyDown={(event) => {
                if (event.target instanceof HTMLInputElement) event.preventDefault()
              }}
              onCloseAutoFocus={focus.onCloseAutoFocus}
              onKeyDown={closeOnToggle}
            >
              <motion.div
                className="fixed inset-x-4 top-[12vh] z-50 mx-auto max-w-xl"
                exit={{ opacity: 0, transition: reduced ? { duration: 0 } : leave }}
              >
                <DialogPrimitive.Title className="sr-only">{t("quickSearch.title")}</DialogPrimitive.Title>
                <QuickSearchPalette onDone={done} onClose={() => done(false)} />
              </motion.div>
            </DialogPrimitive.Content>
          </DialogPrimitive.Portal>
        )}
      </AnimatePresence>
    </DialogPrimitive.Root>
  )
}

function QuickSearchPalette({ onDone, onClose }: { onDone: (focusMoved: boolean) => void; onClose: () => void }) {
  const { t, i18n } = useTranslation()
  const platform = usePlatform()
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const openSettings = useOpenSettings()
  const conversations = useConversations()
  const workers = useWorkers()
  const plugins = usePlugins()
  const catalog = usePluginCatalog({ enabled: false })
  const canStartOver = useHasCommand("newConversation")

  const workerMap = useMemo(
    () => new Map<string, Worker>((workers.data ?? []).map((worker) => [worker.id, worker])),
    [workers.data],
  )
  const current = useCurrentConversation(workerMap)

  const entries = useMemo(
    () =>
      quickSearchEntries({
        conversations: visibleConversations(conversations.data ?? [], workerMap),
        workers: workerMap,
        current,
        plugins: plugins.data ?? [],
        catalog: catalog.data?.plugins,
        commands: new Set<CommandId>(canStartOver ? ["newConversation"] : []),
        lang: i18n.language,
        t,
      }),
    [conversations.data, workerMap, current, plugins.data, catalog.data, canStartOver, i18n.language, t],
  )

  const items = useMemo(() => {
    const currentWorker = current ? workerMap.get(workerIdOf(current) ?? "") : undefined
    const groupLabel = (entry: QuickSearchEntry) =>
      entry.group === "worker" ? (currentWorker?.name ?? "") : t(`quickSearch.groups.${entry.group}`)
    return entries.map(
      (entry): CommandItem => ({
        id: entry.id,
        label: entry.label,
        description: entry.description,
        keywords: entry.keywords,
        group: groupLabel(entry),
        icon: iconOf(entry),
        shortcut: entry.shortcut
          ? shortcutTokens(SHORTCUTS[entry.shortcut], platform)
              .map((token) => token.label)
              .join(" ")
          : undefined,
      }),
    )
  }, [entries, current, workerMap, platform, t])

  const chatPath = pathname.startsWith(CHAT_PATH) ? pathname : conversationPath(BEE_CONVERSATION)

  const run = (target: QuickSearchTarget) => {
    switch (target.kind) {
      case "conversation":
        onDone(true)
        navigate(conversationPath(target.conversation))
        requestComposerFocus(target.conversation)
        return
      case "panel":
        onDone(false)
        if (current) {
          navigate({
            pathname: conversationPath(current),
            search: panelToSearch({ mode: "worker", section: target.section }, new URLSearchParams()).toString(),
          })
        }
        return
      case "newWorker":
        onDone(false)
        navigate({ pathname: chatPath, search: panelToSearch({ mode: "new" }, new URLSearchParams()).toString() })
        return
      case "command":
        onDone(true)
        requestAnimationFrame(() => runCommand(target.command))
        return
      case "settings":
        onDone(false)
        openSettings({ section: target.section })
        return
      case "plugin":
        onDone(false)
        openSettings({ section: "plugins", plugin: target.plugin })
        return
    }
  }

  const targets = useMemo(() => new Map(entries.map((entry) => [entry.id, entry.target])), [entries])

  return (
    <CommandPalette
      items={items}
      placeholder={t("quickSearch.placeholder")}
      label={t("quickSearch.title")}
      autoFocus
      onClose={onClose}
      onSelect={(item) => {
        const target = targets.get(item.id)
        if (target) run(target)
      }}
    />
  )
}
