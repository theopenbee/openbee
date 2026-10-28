import { describe, expect, it } from "vitest"
import type { ChatMessage, ConversationSummary, Worker } from "../types"
import {
  dismissHint,
  getNotificationChoice,
  isHintDismissed,
  notificationContent,
  notificationHint,
  notificationSupport,
  notificationsActive,
  shouldNotify,
  storeNotificationChoice,
  titleWithUnread,
  totalUnread,
  unavailableReason,
  type NotificationSupport,
} from "../notifications"

function memoryStorage(initial: Record<string, string> = {}) {
  const values = new Map(Object.entries(initial))
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value)
    },
  }
}

const message = (extra: Partial<ChatMessage> = {}): ChatMessage => ({
  id: "m1",
  sender: { type: "worker", id: "w1" },
  kind: "message",
  content: "Build finished",
  ts: 1,
  ...extra,
})

const worker = (id: string, name: string): Worker => ({
  id,
  name,
  description: "",
  constraints: "",
  work_dir: "",
  engine: "pi",
  status: "idle",
  created_at: 0,
  updated_at: 0,
})

const t = (key: string, options?: Record<string, string>) =>
  options ? `${key}(${Object.values(options).join(",")})` : key

const away = { visible: false, focused: false }

describe("notificationSupport", () => {
  it("reports an insecure page before anything else", () => {
    expect(
      notificationSupport({ isSecureContext: false, Notification: { permission: "granted" }, constructible: true }),
    ).toBe("insecure")
  })

  it("reports a browser without the Notification API", () => {
    expect(notificationSupport({ isSecureContext: true, constructible: true })).toBe("unsupported")
  })

  it("reports a browser that cannot construct page notifications", () => {
    expect(
      notificationSupport({ isSecureContext: true, Notification: { permission: "default" }, constructible: false }),
    ).toBe("unsupported")
  })

  it.each<NotificationPermission>(["default", "granted", "denied"])("passes through the %s permission", (permission) => {
    expect(notificationSupport({ isSecureContext: true, Notification: { permission }, constructible: true })).toBe(
      permission,
    )
  })
})

describe("stored choice", () => {
  it("reads nothing before a choice is made", () => {
    expect(getNotificationChoice(memoryStorage())).toBeNull()
  })

  it.each(["on", "off"] as const)("round-trips %s", (choice) => {
    const storage = memoryStorage()
    storeNotificationChoice(choice, storage)

    expect(getNotificationChoice(storage)).toBe(choice)
  })

  it("ignores unknown values", () => {
    expect(getNotificationChoice(memoryStorage({ notifications: "maybe" }))).toBeNull()
  })

  it("remembers a dismissed hint", () => {
    const storage = memoryStorage()
    expect(isHintDismissed(storage)).toBe(false)

    dismissHint(storage)

    expect(isHintDismissed(storage)).toBe(true)
  })
})

describe("notificationsActive", () => {
  it("needs both the permission and the choice", () => {
    expect(notificationsActive("granted", "on")).toBe(true)
    expect(notificationsActive("granted", null)).toBe(false)
    expect(notificationsActive("granted", "off")).toBe(false)
    expect(notificationsActive("default", "on")).toBe(false)
    expect(notificationsActive("denied", "on")).toBe(false)
  })
})

describe("notificationHint", () => {
  it.each<[NotificationSupport, "on" | "off" | null, "prompt" | "blocked" | null]>([
    ["default", null, "prompt"],
    ["default", "on", "prompt"],
    ["granted", null, "prompt"],
    ["granted", "on", null],
    ["denied", null, "blocked"],
    ["denied", "on", "blocked"],
    ["insecure", null, null],
    ["unsupported", null, null],
    ["default", "off", null],
    ["denied", "off", null],
  ])("shows %s / %s as %s", (support, choice, expected) => {
    expect(notificationHint(support, choice, false)).toBe(expected)
  })

  it("stays hidden once dismissed", () => {
    expect(notificationHint("default", null, true)).toBeNull()
    expect(notificationHint("denied", null, true)).toBeNull()
  })
})

describe("unavailableReason", () => {
  it.each<[NotificationSupport, string | null]>([
    ["insecure", "insecure"],
    ["unsupported", "unsupported"],
    ["denied", "denied"],
    ["default", null],
    ["granted", null],
  ])("maps %s to %s", (support, expected) => {
    expect(unavailableReason(support)).toBe(expected)
  })
})

describe("shouldNotify", () => {
  it("notifies an agent reply while the page is in the background", () => {
    expect(shouldNotify("worker:w1", message(), away)).toBe(true)
  })

  it("notifies while the page is visible but the window has lost focus", () => {
    expect(shouldNotify("worker:w1", message(), { visible: true, focused: false })).toBe(true)
  })

  it("stays quiet while the page is in front", () => {
    expect(shouldNotify("worker:w1", message(), { visible: true, focused: true })).toBe(false)
  })

  it("never notifies the user's own messages", () => {
    expect(shouldNotify("bee", message({ sender: { type: "user" } }), away)).toBe(false)
  })

  it("notifies Bee and system messages", () => {
    expect(shouldNotify("bee", message({ sender: { type: "bee" } }), away)).toBe(true)
    expect(shouldNotify("worker:w1", message({ sender: { type: "system" } }), away)).toBe(true)
  })

  it("skips the Bee's handoff copy in a worker chat", () => {
    expect(shouldNotify("worker:w1", message({ sender: { type: "bee" } }), away)).toBe(false)
  })
})

describe("notificationContent", () => {
  const workers = new Map([
    ["w1", worker("w1", "Joe")],
    ["w2", worker("w2", "Ann")],
  ])

  it("titles a worker chat with the worker name", () => {
    expect(notificationContent("worker:w1", message(), workers, t)).toEqual({
      title: "Joe",
      body: "Build finished",
      tag: "m1",
    })
  })

  it("titles the Bee chat with the Bee label", () => {
    expect(notificationContent("bee", message({ sender: { type: "bee" } }), workers, t).title).toBe("chat.beeLabel")
  })

  it("falls back for a deleted worker", () => {
    expect(notificationContent("worker:gone", message(), workers, t).title).toBe("chat.deletedWorker")
  })

  it("describes a handoff card", () => {
    const handoff = message({
      sender: { type: "bee" },
      kind: "handoff",
      handoff: { worker_id: "w2", task_id: "t1", task_type: "immediate", conversation_id: "worker:w2" },
    })

    expect(notificationContent("bee", handoff, workers, t).body).toBe("chat.handedOffTo(Ann)")
  })

  it("flattens whitespace in the body", () => {
    expect(notificationContent("worker:w1", message({ content: "line one\n\nline   two" }), workers, t).body).toBe(
      "line one line two",
    )
  })

  it("uses a generic body when there is no text", () => {
    expect(notificationContent("worker:w1", message({ content: "[file] /tmp/a.png\n" }), workers, t).body).toBe(
      "notifications.newMessage",
    )
  })
})

describe("unread title", () => {
  const summary = (unread: number): ConversationSummary => ({
    id: "bee",
    kind: "bee",
    unread,
    activity: { state: "idle" },
  })

  it("sums unread across conversations", () => {
    expect(totalUnread([summary(2), summary(0), summary(3)])).toBe(5)
    expect(totalUnread(undefined)).toBe(0)
  })

  it("leaves the title alone with nothing unread", () => {
    expect(titleWithUnread("OpenBee", 0)).toBe("OpenBee")
  })

  it("prefixes the unread count", () => {
    expect(titleWithUnread("OpenBee", 3)).toBe("(3) OpenBee")
  })

  it("caps a large count", () => {
    expect(titleWithUnread("OpenBee", 100)).toBe("(99+) OpenBee")
  })
})
