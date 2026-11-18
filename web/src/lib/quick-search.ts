import { BEE_CONVERSATION, workerIdOf } from "@/lib/conversations"
import { localize } from "@/lib/plugins"
import type { PanelSection, SettingsSection } from "@/lib/routes"
import type { CommandId } from "@/lib/shortcuts"
import type { ConversationId, ConversationSummary, PluginStatus, Worker } from "@/lib/types"

export type QuickSearchGroup = "conversations" | "actions" | "worker" | "settings" | "plugins"

export type QuickSearchTarget =
  | { kind: "conversation"; conversation: ConversationId }
  | { kind: "panel"; section: PanelSection }
  | { kind: "newWorker" }
  | { kind: "command"; command: CommandId }
  | { kind: "settings"; section: SettingsSection }
  | { kind: "plugin"; plugin: string }

export interface QuickSearchEntry {
  id: string
  group: QuickSearchGroup
  label: string
  description?: string
  keywords?: string[]
  shortcut?: CommandId
  target: QuickSearchTarget
}

export interface QuickSearchInput {
  conversations: ConversationSummary[]
  workers: Map<string, Worker>
  current: ConversationId | null
  plugins: PluginStatus[]
  commands: ReadonlySet<CommandId>
  lang: string
  t: (key: string, options?: Record<string, string>) => string
}

const PANEL_SECTIONS: PanelSection[] = ["constraints", "permissions", "env"]

const SETTINGS: { section: SettingsSection; labelKey: string }[] = [
  { section: "env", labelKey: "envConfig.navTitle" },
  { section: "system", labelKey: "systemSettings.title" },
  { section: "plugins", labelKey: "plugins.title" },
  { section: "about", labelKey: "about.title" },
]

function conversationEntries({ conversations, workers, t }: QuickSearchInput): QuickSearchEntry[] {
  return conversations.flatMap((item): QuickSearchEntry[] => {
    if (item.id === BEE_CONVERSATION) {
      return [{ id: `chat:${item.id}`, group: "conversations", label: t("chat.beeLabel"), target: { kind: "conversation", conversation: item.id } }]
    }
    const worker = item.worker_id ? workers.get(item.worker_id) : undefined
    if (!worker) return []
    return [
      {
        id: `chat:${item.id}`,
        group: "conversations",
        label: worker.name,
        description: worker.description || undefined,
        keywords: worker.engine ? [worker.engine] : undefined,
        target: { kind: "conversation", conversation: item.id },
      },
    ]
  })
}

function actionEntries({ commands, t }: QuickSearchInput): QuickSearchEntry[] {
  const entries: QuickSearchEntry[] = []
  if (commands.has("newConversation")) {
    entries.push({ id: "action:newConversation", group: "actions", label: t("chat.newConversation"), shortcut: "newConversation", target: { kind: "command", command: "newConversation" } })
  }
  entries.push({ id: "action:newWorker", group: "actions", label: t("chat.newWorker"), target: { kind: "newWorker" } })
  entries.push({ id: "action:showShortcuts", group: "actions", label: t("shortcuts.title"), shortcut: "showShortcuts", target: { kind: "command", command: "showShortcuts" } })
  return entries
}

function workerEntries({ current, workers, t }: QuickSearchInput): QuickSearchEntry[] {
  const workerId = current ? workerIdOf(current) : null
  const worker = workerId ? workers.get(workerId) : undefined
  if (!worker) return []
  return PANEL_SECTIONS.map((section) => ({
    id: `panel:${section}`,
    group: "worker",
    label: t(`chat.panelTabs.${section}`),
    keywords: [worker.name],
    target: { kind: "panel", section },
  }))
}

function settingsEntries({ t }: QuickSearchInput): QuickSearchEntry[] {
  return SETTINGS.map(({ section, labelKey }) => ({
    id: `settings:${section}`,
    group: "settings",
    label: t(labelKey),
    keywords: [t("nav.settings")],
    shortcut: section === "env" ? "openSettings" : undefined,
    target: { kind: "settings", section },
  }))
}

function pluginEntries({ plugins, lang }: QuickSearchInput): QuickSearchEntry[] {
  return plugins.map((plugin) => ({
    id: `plugin:${plugin.id}`,
    group: "plugins",
    label: localize(plugin.name, lang) || plugin.id,
    description: localize(plugin.description, lang) || undefined,
    keywords: [plugin.id],
    target: { kind: "plugin", plugin: plugin.id },
  }))
}

export function quickSearchEntries(input: QuickSearchInput): QuickSearchEntry[] {
  return [
    ...conversationEntries(input),
    ...actionEntries(input),
    ...workerEntries(input),
    ...settingsEntries(input),
    ...pluginEntries(input),
  ]
}
