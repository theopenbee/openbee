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

const render = (message: ChatMessage) =>
  renderToStaticMarkup(
    createElement(
      I18nextProvider,
      { i18n: testI18n("en") },
      createElement(MessageBubble, {
        message,
        sender: { name: "System", avatar: null },
        isGroupStart: true,
        onExpandToggle: () => {},
        onShowExecution: () => {},
      }),
    ),
  )

describe("MessageBubble", () => {
  it("links a failure or stop notice to its run", () => {
    expect(render(notice({ execution_id: "e1" }))).toContain("View process")
  })

  it("has no process link on a notice without a run", () => {
    expect(render(notice())).not.toContain("View process")
  })
})
