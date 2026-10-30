import { previewOf } from "@/lib/chat-events"
import { previewText, workerIdOf } from "@/lib/conversations"
import type { ChatMessage, ConversationId, ConversationSummary, Worker } from "@/lib/types"

export type NotificationSupport = "insecure" | "unsupported" | NotificationPermission
export type NotificationChoice = "on" | "off"
export type NotificationHint = "prompt" | "blocked"
export type NotificationUnavailable = "insecure" | "unsupported" | "denied"

export interface NotificationEnvironment {
  isSecureContext: boolean
  Notification?: { permission: NotificationPermission }
  constructible: boolean
}

export interface PageAttention {
  visible: boolean
  focused: boolean
}

export interface NotificationContent {
  title: string
  body: string
  tag: string
}

type PreferenceStorage = Pick<Storage, "getItem" | "setItem">
type Translate = (key: string, options?: Record<string, string>) => string

const CHOICE_KEY = "notifications"
const HINT_KEY = "notifications.hint"
const HINT_DISMISSED = "dismissed"
const MAX_TITLE_UNREAD = 99

export function notificationSupport(env: NotificationEnvironment): NotificationSupport {
  if (!env.isSecureContext) return "insecure"
  if (!env.Notification || !env.constructible) return "unsupported"
  return env.Notification.permission
}

export function getNotificationChoice(storage: PreferenceStorage = localStorage): NotificationChoice | null {
  const value = storage.getItem(CHOICE_KEY)
  return value === "on" || value === "off" ? value : null
}

export function storeNotificationChoice(choice: NotificationChoice, storage: PreferenceStorage = localStorage) {
  storage.setItem(CHOICE_KEY, choice)
}

export function isHintDismissed(storage: PreferenceStorage = localStorage): boolean {
  return storage.getItem(HINT_KEY) === HINT_DISMISSED
}

export function dismissHint(storage: PreferenceStorage = localStorage) {
  storage.setItem(HINT_KEY, HINT_DISMISSED)
}

export function notificationsActive(support: NotificationSupport, choice: NotificationChoice | null): boolean {
  return support === "granted" && choice === "on"
}

export function notificationHint(
  support: NotificationSupport,
  choice: NotificationChoice | null,
  dismissed: boolean,
): NotificationHint | null {
  if (dismissed || choice === "off") return null
  if (support === "denied") return "blocked"
  if (support === "default") return "prompt"
  if (support === "granted" && choice === null) return "prompt"
  return null
}

export function unavailableReason(support: NotificationSupport): NotificationUnavailable | null {
  return support === "insecure" || support === "unsupported" || support === "denied" ? support : null
}

export function shouldNotify(conversation: ConversationId, message: ChatMessage, attention: PageAttention): boolean {
  if (message.sender.type === "user" || message.kind === "reset") return false
  if (message.sender.type === "bee" && workerIdOf(conversation)) return false
  return !(attention.visible && attention.focused)
}

export function notificationContent(
  conversation: ConversationId,
  message: ChatMessage,
  workers: Map<string, Worker>,
  t: Translate,
): NotificationContent {
  const workerId = workerIdOf(conversation)
  const title = workerId ? (workers.get(workerId)?.name ?? t("chat.deletedWorker")) : t("chat.beeLabel")
  const body = previewText(
    {
      sender: message.sender,
      kind: message.kind,
      preview: previewOf(message),
      media_paths: message.media_paths,
      handoff_worker_id: message.handoff?.worker_id,
      ts: message.ts,
    },
    workers,
    t,
  )
  return { title, body: body || t("notifications.newMessage"), tag: message.id }
}

export function totalUnread(list: ConversationSummary[] | undefined): number {
  return (list ?? []).reduce((sum, item) => sum + item.unread, 0)
}

export function titleWithUnread(base: string, unread: number): string {
  if (unread <= 0) return base
  return `(${unread > MAX_TITLE_UNREAD ? `${MAX_TITLE_UNREAD}+` : unread}) ${base}`
}
