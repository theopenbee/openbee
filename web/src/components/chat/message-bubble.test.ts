import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { I18nextProvider } from "react-i18next"
import { describe, expect, it } from "vitest"
import type { ChatMessage } from "@/lib/types"
import { testI18n } from "@/test/i18n"
import { MessageBubble } from "./message-bubble"

const notice = (extra: Partial<ChatMessage> = {}): ChatMessage => ({
  id: "n1",
  sender: { type: "system" },
  kind: "message",
  content: "Task execution failed",
  ts: 1_000,
  ...extra,
})

const render = (message: ChatMessage, sender: { name: string; avatar: null } | null = { name: "System", avatar: null }) =>
  renderToStaticMarkup(
    createElement(
      I18nextProvider,
      { i18n: testI18n("en") },
      createElement(MessageBubble, {
        message,
        sender,
        isGroupStart: true,
        onExpandToggle: () => {},
        onShowExecution: () => {},
        onChoose: async () => {},
      }),
    ),
  )

const buttons = [{ label: "Stop Lin" }, { label: "Stop Mia" }]

describe("MessageBubble", () => {
  it("links a failure or stop notice to its run", () => {
    expect(render(notice({ execution_id: "e1" }))).toContain("View process")
  })

  it("has no process link on a notice without a run", () => {
    expect(render(notice())).not.toContain("View process")
  })

  it("shows the buttons of a reply under its bubble", () => {
    const html = render(notice({ sender: { type: "bee" }, content: "Running tasks", buttons }))

    expect(html).toContain("Stop Lin")
    expect(html).toContain("Stop Mia")
    expect(html.indexOf("Stop Lin")).toBeGreaterThan(html.indexOf("Running tasks"))
  })

  it("has no button row without buttons", () => {
    expect(render(notice({ buttons: [] }))).not.toContain("flex-wrap gap-2")
  })

  it("never shows buttons on the user's own message", () => {
    expect(render(notice({ sender: { type: "user" }, content: "hi", buttons }), null)).not.toContain("Stop Lin")
  })
})
