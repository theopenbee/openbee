import { useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { ChevronDown, ChevronUp } from "lucide-react"
import { useTranslation } from "react-i18next"
import { Streamdown } from "streamdown"
import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"
import type { ExecutionStatus } from "@/lib/types"
import { isActiveStatus } from "@/lib/format"
import { cn } from "@/lib/utils"

type ParsedEntry =
  | { kind: "text"; text: string }
  | {
      kind: "tool"
      id: string
      name: string
      input: unknown
      result?: string
      isError?: boolean
    }
  | { kind: "result"; text: string; subtype: string }
  | { kind: "raw"; content: string; logType: string }

type LogFilter = "all" | "text" | "tool" | "raw"
type LogViewerVariant = "standalone" | "embedded"

interface ClaudeStreamEvent {
  type: string
  subtype?: string
  message?: {
    content: Array<{
      type: string
      text?: string
      id?: string
      name?: string
      input?: unknown
      tool_use_id?: string
      content?: string | unknown
      is_error?: boolean
    }>
  }
  result?: string
}

interface LogViewerProps {
  executionId: string
  status: ExecutionStatus
  onComplete?: () => void
  autoScroll?: boolean
  variant?: LogViewerVariant
}

function parseStreamLine(line: string): ClaudeStreamEvent | null {
  try {
    const obj = JSON.parse(line)
    if (obj && typeof obj.type === "string") return obj as ClaudeStreamEvent
    return null
  } catch {
    return null
  }
}

function truncate(value: string, max = 96) {
  return value.length > max ? `${value.slice(0, max)}…` : value
}

function stringify(value: unknown) {
  if (typeof value === "string") return value
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

function getToolMeta(name: string) {
  switch (name) {
    case "Bash":
      return {
        label: "SH",
        summary: (input: unknown) =>
          truncate(
            (input as { command?: string; cmd?: string })?.command ??
              (input as { command?: string; cmd?: string })?.cmd ??
              stringify(input)
          ),
      }
    case "Read":
    case "Write":
    case "Edit":
    case "Glob":
    case "Grep":
      return {
        label: "FS",
        summary: (input: unknown) => {
          const record = input as Record<string, string>
          return truncate(
            record?.file_path ??
              record?.path ??
              record?.pattern ??
              record?.glob ??
              stringify(input)
          )
        },
      }
    case "WebSearch":
    case "WebFetch":
      return {
        label: "WEB",
        summary: (input: unknown) => {
          const record = input as Record<string, string>
          return truncate(record?.query ?? record?.url ?? stringify(input))
        },
      }
    default:
      return {
        label: "TOOL",
        summary: (input: unknown) => truncate(stringify(input)),
      }
  }
}

function extractToolResultText(content: unknown): string {
  if (typeof content === "string") return content
  if (Array.isArray(content)) {
    const texts = content
      .filter((chunk): chunk is { type: string; text: string } => {
        return (
          typeof chunk === "object" &&
          chunk !== null &&
          "text" in chunk &&
          typeof (chunk as Record<string, unknown>).text === "string"
        )
      })
      .map((chunk) => chunk.text)

    if (texts.length > 0) return texts.join("\n")
  }
  return stringify(content)
}

function appendTextEntry(text: string, entries: ParsedEntry[]) {
  const last = entries[entries.length - 1]
  if (last?.kind === "text") {
    last.text = `${last.text}\n\n${text}`
    return
  }
  entries.push({ kind: "text", text })
}

function appendRawEntry(content: string, logType: string, entries: ParsedEntry[]) {
  const last = entries[entries.length - 1]
  if (last?.kind === "raw" && last.logType === logType) {
    last.content = `${last.content}\n${content}`
    return
  }
  entries.push({ kind: "raw", content, logType })
}

function appendEntry(content: string, logType: string, entries: ParsedEntry[], toolMap: Map<string, number>) {
  if (logType === "stdout") {
    const event = parseStreamLine(content)
    if (event) {
      if (event.type === "assistant" && event.message?.content) {
        for (const block of event.message.content) {
          if (block.type === "text" && block.text) {
            appendTextEntry(block.text, entries)
          } else if (block.type === "tool_use" && block.id && block.name) {
            toolMap.set(block.id, entries.length)
            entries.push({
              kind: "tool",
              id: block.id,
              name: block.name,
              input: block.input,
            })
          }
        }
        return
      }

      if (event.type === "user" && event.message?.content) {
        for (const block of event.message.content) {
          if (block.type === "tool_result" && block.tool_use_id) {
            const idx = toolMap.get(block.tool_use_id)
            if (idx === undefined) continue

            const existing = entries[idx]
            if (existing?.kind === "tool") {
              entries[idx] = {
                ...existing,
                result: extractToolResultText(block.content),
                isError: block.is_error,
              }
            }
          }
        }
        return
      }

      if (event.type === "result") {
        entries.push({
          kind: "result",
          text: event.result ?? "",
          subtype: event.subtype ?? "",
        })
        return
      }

      if (event.type === "system" || event.type === "rate_limit_event") {
        return
      }
    }
  }

  appendRawEntry(content, logType, entries)
}

function MetricChip({ label, value }: { label: string; value: number }) {
  return (
    <span className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-background/80 px-3 py-1.5 text-xs">
      <span className="font-mono text-foreground">{value}</span>
      <span className="text-muted-foreground">{label}</span>
    </span>
  )
}

function TimelineRow({
  markerClassName,
  children,
}: {
  markerClassName: string
  children: ReactNode
}) {
  return (
    <div className="grid grid-cols-[0.75rem_minmax(0,1fr)] gap-3">
      <div className="flex justify-center pt-5">
        <span className={cn("size-2.5 rounded-full ring-4 ring-background", markerClassName)} />
      </div>
      <div className="min-w-0">{children}</div>
    </div>
  )
}

function AssistantEntry({ text }: { text: string }) {
  const { t } = useTranslation()

  return (
    <TimelineRow markerClassName="bg-primary/70">
      <article className="overflow-hidden rounded-2xl border border-border/70 bg-background/80">
        <div className="px-4 pt-4">
          <p className="text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
            {t("logViewer.narrative")}
          </p>
          <p className="mt-1 text-sm font-medium text-foreground">{t("logViewer.assistant")}</p>
        </div>

        <div className="px-4 pb-4 pt-3">
          <div className="rounded-xl bg-muted/35 p-4">
            <div className="prose prose-sm max-w-none text-foreground dark:prose-invert prose-p:leading-6 prose-headings:font-medium">
              <Streamdown mode="static">{text}</Streamdown>
            </div>
          </div>
        </div>
      </article>
    </TimelineRow>
  )
}

function ToolEntry({
  entry,
}: {
  entry: Extract<ParsedEntry, { kind: "tool" }>
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(Boolean(entry.isError))
  const meta = getToolMeta(entry.name)
  const summary = meta.summary(entry.input)

  return (
    <TimelineRow markerClassName={entry.isError ? "bg-destructive" : entry.result ? "bg-status-idle" : "bg-primary/55"}>
      <article className="overflow-hidden rounded-2xl border border-border/70 bg-background/80">
        <button
          type="button"
          aria-expanded={open}
          aria-label={open ? t("logViewer.collapse", { name: entry.name }) : t("logViewer.expand", { name: entry.name })}
          onClick={() => setOpen((current) => !current)}
          className="flex w-full items-start gap-3 px-4 py-4 text-left transition-colors hover:bg-muted/25"
        >
          <span className="inline-flex h-7 shrink-0 items-center rounded-full border border-border/70 bg-background px-2.5 font-mono text-[11px] tracking-[0.18em] text-muted-foreground">
            {meta.label}
          </span>

          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <p className="text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
                {t("logViewer.toolCall")}
              </p>
            </div>

            <div className="mt-1 flex flex-wrap items-baseline gap-2">
              <p className="text-sm font-medium text-foreground">{entry.name}</p>
              <p className="min-w-0 flex-1 truncate text-sm text-muted-foreground">{summary}</p>
            </div>
          </div>

          <span className="mt-0.5 shrink-0 text-muted-foreground" aria-hidden="true">
            {open ? <ChevronUp className="size-4" /> : <ChevronDown className="size-4" />}
          </span>
        </button>

        {open && (
          <div className="grid gap-3 border-t border-border/70 px-4 pb-4 pt-3 md:grid-cols-2 animate-fade-in">
            <section className="space-y-2">
              <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                {t("logViewer.input")}
              </p>
              <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded-xl border border-border/70 bg-muted/35 p-3 font-mono text-[12px] leading-6 text-foreground">
                {stringify(entry.input)}
              </pre>
            </section>

            <section className="space-y-2">
              <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                {t("logViewer.output")}
              </p>
              <div className="rounded-xl border border-border/70 bg-muted/35 p-3">
                {entry.result !== undefined ? (
                  <pre
                    className={cn(
                      "overflow-x-auto whitespace-pre-wrap break-words font-mono text-[12px] leading-6",
                      entry.isError ? "text-destructive" : "text-foreground"
                    )}
                  >
                    {entry.result}
                  </pre>
                ) : (
                  <p className="text-sm text-muted-foreground">{t("logViewer.waiting")}</p>
                )}
              </div>
            </section>
          </div>
        )}
      </article>
    </TimelineRow>
  )
}

function ResultEntry({ entry }: { entry: Extract<ParsedEntry, { kind: "result" }> }) {
  const { t } = useTranslation()

  return (
    <TimelineRow markerClassName="bg-status-idle">
      <article className="overflow-hidden rounded-2xl border border-status-idle/20 bg-status-idle/8">
        <div className="px-4 py-4">
          <div className="flex flex-wrap items-center gap-2">
            <p className="text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
              {t("logViewer.result")}
            </p>
            {entry.subtype && (
              <span className="rounded-full border border-status-idle/20 bg-background/70 px-2 py-0.5 font-mono text-[11px] text-status-idle">
                {entry.subtype}
              </span>
            )}
          </div>

          <pre className="mt-3 whitespace-pre-wrap break-words font-mono text-sm leading-6 text-foreground">
            {entry.text || "—"}
          </pre>
        </div>
      </article>
    </TimelineRow>
  )
}

function RawEntry({ entry }: { entry: Extract<ParsedEntry, { kind: "raw" }> }) {
  const { t } = useTranslation()
  const isError = entry.logType === "stderr" || entry.logType === "error"
  const lineCount = entry.content.split("\n").length

  return (
    <TimelineRow markerClassName={isError ? "bg-destructive/85" : "bg-muted-foreground/55"}>
      <article className="overflow-hidden rounded-2xl border border-border/70 bg-background/80">
        <div className="flex flex-wrap items-center justify-between gap-3 px-4 pt-4">
          <div>
            <p className="text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
              {t("logViewer.rawOutput")}
            </p>
            <p className="mt-1 text-sm font-medium text-foreground">{isError ? t("logViewer.failed") : t("logViewer.raw")}</p>
          </div>

          <span className="rounded-full border border-border/70 bg-background px-2 py-0.5 font-mono text-[11px] text-muted-foreground">
            {lineCount}
          </span>
        </div>

        <div className="px-4 pb-4 pt-3">
          <pre
            className={cn(
              "overflow-x-auto whitespace-pre-wrap break-words rounded-xl border border-border/70 bg-muted/35 p-3 font-mono text-[12px] leading-6",
              isError ? "text-destructive" : "text-foreground"
            )}
          >
            {entry.content}
          </pre>
        </div>
      </article>
    </TimelineRow>
  )
}

export function LogViewer({
  executionId,
  status,
  onComplete,
  autoScroll = true,
  variant = "standalone",
}: LogViewerProps) {
  const { t } = useTranslation()
  const [entries, setEntries] = useState<ParsedEntry[]>([])
  const [filter, setFilter] = useState<LogFilter>("all")
  const [followLive, setFollowLive] = useState(autoScroll)
  const toolMapRef = useRef<Map<string, number>>(new Map())
  const viewportRef = useRef<HTMLDivElement>(null)
  const parsedLengthRef = useRef(0)
  const pendingLineRef = useRef("")
  const prevStatusRef = useRef<ExecutionStatus>(status)

  useEffect(() => {
    setEntries([])
    toolMapRef.current = new Map()
    parsedLengthRef.current = 0
    pendingLineRef.current = ""
    setFollowLive(autoScroll)
  }, [executionId, autoScroll])

  useEffect(() => {
    let disposed = false

    const appendLines = (lines: string[]) => {
      if (lines.length === 0) return
      setEntries((previous) => {
        const next = [...previous]
        lines.forEach((line) => appendEntry(line, "stdout", next, toolMapRef.current))
        return next
      })
    }

    const consumeChunk = (chunk: string, flushTail: boolean) => {
      const combined = pendingLineRef.current + chunk
      if (!combined) return

      const segments = combined.split("\n")
      if (combined.endsWith("\n")) {
        segments.pop()
        pendingLineRef.current = ""
      } else if (!flushTail) {
        pendingLineRef.current = segments.pop() ?? ""
      } else {
        pendingLineRef.current = ""
      }

      appendLines(segments.filter(Boolean))
    }

    const rebuildEntries = (content: string, flushTail: boolean) => {
      const segments = content.split("\n")
      if (content.endsWith("\n")) {
        segments.pop()
        pendingLineRef.current = ""
      } else if (!flushTail) {
        pendingLineRef.current = segments.pop() ?? ""
      } else {
        pendingLineRef.current = ""
      }

      const nextEntries: ParsedEntry[] = []
      const nextToolMap = new Map<string, number>()
      segments.filter(Boolean).forEach((line) => appendEntry(line, "stdout", nextEntries, nextToolMap))
      toolMapRef.current = nextToolMap
      setEntries(nextEntries)
    }

    const fetchLogs = async () => {
      try {
        const content = await api.executions.logs(executionId)
        if (disposed) return

        const flushTail = !isActiveStatus(status)
        if (content.length < parsedLengthRef.current) {
          parsedLengthRef.current = content.length
          rebuildEntries(content, flushTail)
          return
        }

        if (content.length > parsedLengthRef.current) {
          const chunk = content.slice(parsedLengthRef.current)
          parsedLengthRef.current = content.length
          consumeChunk(chunk, flushTail)
          return
        }

        if (flushTail && pendingLineRef.current) {
          consumeChunk("", true)
        }
      } catch {
        // ignore transient errors while the process is still writing logs
      }
    }

    fetchLogs()

    if (isActiveStatus(status)) {
      const interval = setInterval(fetchLogs, 2000)
      return () => {
        disposed = true
        clearInterval(interval)
      }
    }

    return () => {
      disposed = true
    }
  }, [executionId, status])

  useEffect(() => {
    if (isActiveStatus(prevStatusRef.current) && !isActiveStatus(status)) {
      onComplete?.()
    }
    prevStatusRef.current = status
  }, [status, onComplete])

  useEffect(() => {
    if (!autoScroll || !followLive) return
    const viewport = viewportRef.current
    if (!viewport) return
    viewport.scrollTo({ top: viewport.scrollHeight, behavior: "auto" })
  }, [entries, autoScroll, followLive])

  const { narrativeCount, toolCount, rawCount, visibleEntries } = useMemo(() => {
    let narrativeCount = 0
    let toolCount = 0
    let rawCount = 0
    for (const entry of entries) {
      if (entry.kind === "text") narrativeCount += 1
      if (entry.kind === "tool") toolCount += 1
      if (entry.kind === "raw") rawCount += 1
    }
    const visibleEntries = entries.filter((entry) => {
      if (entry.kind === "result") return true
      if (filter === "all") return true
      return entry.kind === filter
    })
    return { narrativeCount, toolCount, rawCount, visibleEntries }
  }, [entries, filter])

  const filterOptions: Array<{ key: LogFilter; label: string; count: number }> = [
    { key: "all", label: t("logViewer.all"), count: entries.length },
    { key: "text", label: t("logViewer.narrative"), count: narrativeCount },
    { key: "tool", label: t("logViewer.tools"), count: toolCount },
    { key: "raw", label: t("logViewer.raw"), count: rawCount },
  ]

  const handleViewportScroll = () => {
    if (!autoScroll || !isActiveStatus(status)) return
    const viewport = viewportRef.current
    if (!viewport) return

    const atBottom = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight < 48
    if (atBottom !== followLive) {
      setFollowLive(atBottom)
    }
  }

  const jumpToLatest = () => {
    const viewport = viewportRef.current
    if (!viewport) return
    setFollowLive(true)
    viewport.scrollTo({ top: viewport.scrollHeight, behavior: "smooth" })
  }

  const shellClassName =
    variant === "embedded"
      ? "overflow-hidden rounded-[1.6rem] bg-background/55 ring-1 ring-border/60"
      : "overflow-hidden rounded-[1.75rem] border border-border/70 bg-card"

  return (
    <div className={shellClassName}>
      <div className="border-b border-border/70 bg-muted/20 px-4 py-4 sm:px-5">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex flex-wrap items-center gap-2">
            {isActiveStatus(status) &&
              (followLive ? (
                <span className="inline-flex items-center gap-2 rounded-full border border-status-working/20 bg-status-working/10 px-3 py-1.5 text-xs font-medium text-status-working">
                  <span className="size-1.5 rounded-full bg-current animate-pulse-amber" />
                  {t("logViewer.followLive")}
                </span>
              ) : (
                <Button variant="outline" size="sm" onClick={jumpToLatest}>
                  {t("logViewer.jumpToLatest")}
                </Button>
              ))}
          </div>
        </div>

        <div className="mt-3 flex flex-wrap items-center gap-2">
          {filterOptions.map((option) => {
            const active = filter === option.key
            return (
              <Button
                key={option.key}
                variant={active ? "outline" : "ghost"}
                size="sm"
                aria-pressed={active}
                onClick={() => setFilter(option.key)}
                className={cn(active && "border-primary/20 bg-primary/5 text-foreground")}
              >
                {option.label}
                <span className="font-mono text-[11px] text-muted-foreground">{option.count}</span>
              </Button>
            )
          })}
        </div>
      </div>

      <div
        ref={viewportRef}
        onScroll={handleViewportScroll}
        className="max-h-[min(70vh,52rem)] overflow-y-auto px-4 py-4 sm:px-5"
      >
        {entries.length === 0 ? (
          <div className="rounded-2xl border border-dashed border-border/70 bg-background/70 px-4 py-6 text-sm text-muted-foreground">
            {isActiveStatus(status) ? (
              <span className="inline-flex items-center gap-2">
                <span className="size-1.5 rounded-full bg-primary animate-pulse-amber" />
                {t("logViewer.waiting")}
              </span>
            ) : (
              t("logViewer.noLogs")
            )}
          </div>
        ) : visibleEntries.length === 0 ? (
          <div className="rounded-2xl border border-dashed border-border/70 bg-background/70 px-4 py-6 text-sm text-muted-foreground">
            {t("logViewer.noMatches")}
          </div>
        ) : (
          <div className="relative">
            <div className="pointer-events-none absolute bottom-4 left-[0.375rem] top-4 w-px bg-border/60" />

            <div className="space-y-3">
              {visibleEntries.map((entry, index) => {
                if (entry.kind === "text") return <AssistantEntry key={`text-${index}`} text={entry.text} />
                if (entry.kind === "tool") return <ToolEntry key={entry.id} entry={entry} />
                if (entry.kind === "result") return <ResultEntry key={`result-${index}`} entry={entry} />
                return <RawEntry key={`raw-${index}`} entry={entry} />
              })}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
