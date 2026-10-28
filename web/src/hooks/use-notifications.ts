import { useCallback, useSyncExternalStore } from "react"
import {
  dismissHint,
  getNotificationChoice,
  isHintDismissed,
  notificationHint,
  notificationsActive,
  notificationSupport,
  storeNotificationChoice,
  unavailableReason,
  type NotificationSupport,
} from "@/lib/notifications"

const listeners = new Set<() => void>()
let watchingPermission = false

function emit() {
  for (const listener of listeners) listener()
}

function watchPermission() {
  if (watchingPermission || !navigator.permissions) return
  watchingPermission = true
  navigator.permissions
    .query({ name: "notifications" })
    .then((status) => status.addEventListener("change", emit))
    .catch(() => undefined)
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  if (listeners.size === 1) {
    window.addEventListener("focus", emit)
    window.addEventListener("storage", emit)
    document.addEventListener("visibilitychange", emit)
  }
  watchPermission()
  return () => {
    listeners.delete(listener)
    if (listeners.size > 0) return
    window.removeEventListener("focus", emit)
    window.removeEventListener("storage", emit)
    document.removeEventListener("visibilitychange", emit)
  }
}

export function currentNotificationSupport(): NotificationSupport {
  return notificationSupport({
    isSecureContext: window.isSecureContext,
    Notification: typeof Notification === "undefined" ? undefined : Notification,
  })
}

function requestPermission(): Promise<unknown> {
  return Promise.resolve()
    .then(() => Notification.requestPermission())
    .catch(() => undefined)
}

export function useNotificationSettings() {
  const support = useSyncExternalStore(subscribe, currentNotificationSupport)
  const choice = useSyncExternalStore(subscribe, () => getNotificationChoice())
  const dismissed = useSyncExternalStore(subscribe, () => isHintDismissed())

  const enable = useCallback(async () => {
    if (currentNotificationSupport() === "default") await requestPermission()
    if (currentNotificationSupport() === "granted") storeNotificationChoice("on")
    emit()
  }, [])

  const disable = useCallback(() => {
    storeNotificationChoice("off")
    emit()
  }, [])

  const dismiss = useCallback(() => {
    dismissHint()
    emit()
  }, [])

  return {
    active: notificationsActive(support, choice),
    hint: notificationHint(support, choice, dismissed),
    unavailable: unavailableReason(support),
    enable,
    disable,
    dismiss,
  }
}
