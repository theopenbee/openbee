import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { I18nextProvider } from "react-i18next"
import { describe, expect, it } from "vitest"
import type { ExecutionStep, ToolStep } from "@/lib/types"
import { testI18n } from "@/test/i18n"
import { ExecutionSteps, ToolDetail } from "./execution-steps"

const withI18n = (element: ReturnType<typeof createElement>, lng: "en" | "zh") =>
  renderToStaticMarkup(createElement(I18nextProvider, { i18n: testI18n(lng) }, element))

const steps = (list: ExecutionStep[], options: { running?: boolean; failure?: string | null; lng?: "en" | "zh" } = {}) =>
  withI18n(
    createElement(ExecutionSteps, { steps: list, running: options.running ?? false, failure: options.failure ?? null }),
    options.lng ?? "en",
  )

const detail = (step: ToolStep, lng: "en" | "zh" = "en") => withI18n(createElement(ToolDetail, { step }), lng)

const tool = (extra: Partial<ToolStep>): ToolStep => ({ type: "tool", kind: "command", name: "Bash", status: "done", ...extra })

const buttons = (html: string) => html.match(/<button/g)?.length ?? 0

describe("ExecutionSteps titles", () => {
  it.each([
    ["read", "Read src/app.ts", "读取 src/app.ts"],
    ["edit", "Edited src/app.ts", "修改 src/app.ts"],
    ["command", "Ran src/app.ts", "运行 src/app.ts"],
    ["search", "Searched src/app.ts", "搜索 src/app.ts"],
    ["web", "Opened src/app.ts", "访问 src/app.ts"],
    ["message", "Sent a message: src/app.ts", "发送消息：src/app.ts"],
  ] as const)("names a %s step after its summary", (kind, en, zh) => {
    const step = tool({ kind, summary: "src/app.ts" })
    expect(steps([step])).toContain(en)
    expect(steps([step], { lng: "zh" })).toContain(zh)
  })

  it.each([
    ["read", "Read a file", "读取文件"],
    ["command", "Ran a command", "运行命令"],
    ["message", "Sent a message", "发送消息"],
  ] as const)("falls back to the verb for a %s step without a summary", (kind, en, zh) => {
    expect(steps([tool({ kind })])).toContain(en)
    expect(steps([tool({ kind })], { lng: "zh" })).toContain(zh)
  })

  it("names another tool by its name and keeps the summary for the detail", () => {
    const html = steps([tool({ kind: "other", name: "todo_write", summary: "3 todos" })])
    expect(html).toContain("Called todo_write")
    expect(html).not.toContain("3 todos</span>")
    expect(html).toMatch(/<button[^>]*aria-expanded="false"/)
  })

  it("does not expand another tool without a summary or output", () => {
    expect(buttons(steps([tool({ kind: "other", name: "todo_write" })]))).toBe(0)
  })

  it("keeps a multi-line message on one title line", () => {
    const html = steps([tool({ kind: "message", name: "send_message", summary: "Done.\n\nSee the PR" })])
    expect(html).toContain("Sent a message: Done. See the PR")
    expect(html).toMatch(/<button[^>]*aria-expanded="false"/)
  })
})

describe("ExecutionSteps rows", () => {
  it("expands a tool with output or a diff", () => {
    const html = steps([tool({ summary: "ls", output: "a.md" }), tool({ kind: "edit", name: "Edit", summary: "a.md", diff: "+x" })])
    expect(buttons(html)).toBe(2)
    expect(html.match(/aria-expanded="false"/g)).toHaveLength(2)
  })

  it("shows the duration from one second and the done mark", () => {
    const html = steps([tool({ summary: "make", duration_ms: 1_500 }), tool({ summary: "ls", duration_ms: 400 })])
    expect(html).toContain(">2s<")
    expect(html).not.toContain(">0s<")
    expect(html.match(/text-success/g)).toHaveLength(2)
  })

  it("marks a failed tool in danger", () => {
    const html = steps([tool({ summary: "make", status: "error" })])
    expect(html).toContain('<span class="sr-only">Failed</span>')
    expect(html).toContain("shrink-0 text-danger")
  })

  it("shimmers and spins a running tool only while the execution runs", () => {
    const step = tool({ summary: "pnpm test", status: "running" })
    const live = steps([step], { running: true })
    expect(live).toContain("animate-spin")
    expect(live).toContain('aria-busy="true"')
    expect(live).toContain('<span class="sr-only">Running</span>')
    const ended = steps([step], { lng: "zh" })
    expect(ended).not.toContain("animate-spin")
    expect(ended).toContain('<span class="sr-only">已中止</span>')
  })
})

describe("ExecutionSteps thinking", () => {
  it("titles thinking with its first line and labels it for screen readers", () => {
    const html = steps([{ type: "thinking", text: "\n**Plan the change**\nCheck the routes first." }], { lng: "zh" })
    expect(html).toContain('<span class="sr-only">思考</span>')
    expect(html).toContain("Plan the change</span>")
    expect(html).not.toContain("**")
    expect(html).not.toContain("Check the routes first.")
    expect(html).toMatch(/<button[^>]*aria-expanded="false"/)
  })

  it("does not expand a one-line thought", () => {
    const html = steps([{ type: "thinking", text: "Short plan" }])
    expect(html).toContain("Short plan")
    expect(buttons(html)).toBe(0)
  })
})

describe("ExecutionSteps text and errors", () => {
  it("renders assistant text as markdown without row chrome", () => {
    const html = steps([{ type: "text", text: "**Done**\nAll good" }])
    expect(html).toContain('data-streamdown="strong">Done</span><br/>')
    expect(html).toContain("All good")
    expect(buttons(html)).toBe(0)
    expect(html).not.toContain("rounded-inset")
  })

  it("shows an error step in danger", () => {
    const html = steps([{ type: "error", text: "rate limited" }])
    expect(html).toContain("text-danger")
    expect(html).toContain("rate limited")
  })

  it("boxes the run failure under its title", () => {
    const html = steps([], { failure: "timed out" })
    expect(html).toContain("Run failed")
    expect(html).toMatch(/rounded-inset border border-danger\/25 bg-danger\/10"><p[^>]*text-danger[^>]*>timed out</)
  })
})

describe("ToolDetail", () => {
  it("shows the summary in mono over the output", () => {
    const html = detail(tool({ summary: "grep -rn limit src", output: "No matches" }))
    expect(html).toMatch(/font-mono[^>]*>grep -rn limit src</)
    expect(html).toContain("border-t border-border")
    expect(html).toMatch(/<pre[^>]*text-secondary[^>]*>No matches<\/pre>/)
  })

  it("shows a failed tool's output in danger", () => {
    expect(detail(tool({ summary: "make", output: "boom", status: "error" }))).toMatch(/<pre[^>]*text-danger[^>]*>boom</)
  })

  it("shows a message in the body font without a body section", () => {
    const html = detail(tool({ kind: "message", name: "send_message", summary: "On it\nsoon" }))
    expect(html).toMatch(/class="[^"]*text-sm[^"]*">On it\nsoon</)
    expect(html).not.toContain("font-mono")
    expect(html).not.toContain("border-t")
  })

  it("leaves out the header without a summary", () => {
    const html = detail(tool({ output: "ok" }))
    expect(html).not.toContain("border-t")
    expect(html).toContain(">ok</pre>")
  })

  it("renders a diff instead of the output", () => {
    const html = detail(
      tool({ kind: "edit", name: "Edit", summary: "a.ts", output: "ignored", diff: " keep\n-old\n+new\n+more\n…" }),
    )
    expect(html).not.toContain("ignored")
    expect(html).toContain('<span class="text-success">+2</span>')
    expect(html).toContain('<span class="text-danger">-1</span>')
    expect(html).toMatch(/bg-danger\/10[^>]*><span[^>]*text-danger[^>]*>-<\/span><span[^>]*>old<\/span>/)
    expect(html.match(/bg-success\/10/g)).toHaveLength(2)
    expect(html).toMatch(/text-secondary[^>]*><span[^>]*> <\/span><span[^>]*>keep</)
    expect(html).toMatch(/text-muted[^>]*><span[^>]*> <\/span><span[^>]*>…</)
  })
})
