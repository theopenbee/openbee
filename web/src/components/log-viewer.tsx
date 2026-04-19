import { useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { ChevronDown, ChevronUp } from "lucide-react"
import { useTranslation } from "react-i18next"
import { Streamdown } from "streamdown"
import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"
import type { ExecutionStatus } from "@/lib/types"
import { isActiveStatus } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { ParsedEntry, StreamParser } from "./log-viewer/types"
import { detectEngine } from "./log-viewer/detect-engine"
import { ClaudeParser, getToolMeta, stringify } from "./log-viewer/claude-parser"
import { CodexParser } from "./log-viewer/codex-parser"
import { PiParser } from "./log-viewer/pi-parser"
import { KimiParser } from "./log-viewer/kimi-parser"

type LogFilter = "all" | "text" | "tool" | "raw"
type LogViewerVariant = "standalone" | "embedded"

const FILTER_ALIAS: Partial<Record<string, LogFilter>> = { "codex-command": "tool", "pi-thinking": "text" }

const PARSER_FACTORY: Record<string, () => StreamParser> = {
  codex: () => new CodexParser(),
  pi: () => new PiParser(),
  kimi: () => new KimiParser(),
}

interface LogViewerProps {
  executionId: string
  status: ExecutionStatus
  onComplete?: () => void
  autoScroll?: boolean
  variant?: LogViewerVariant
}

function formatCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(n % 1_000_000 === 0 ? 0 : 1)}m`
  if (n >= 1_000) return `${(n / 1_000).toFixed(n % 1_000 === 0 ? 0 : 1)}k`
  return String(n)
}

function MetricChip({ label, value }: { label: string; value: number }) {
  return (
    <span className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-background/80 px-3 py-1.5 text-xs" title={value.toLocaleString()}>
      <span className="font-mono text-foreground">{formatCount(value)}</span>
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
            <p className="text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
              {t("logViewer.toolCall")}
            </p>

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
          <ExpandedDetails
            input={
              <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded-xl border border-border/70 bg-muted/35 p-3 font-mono text-[12px] leading-6 text-foreground">
                {stringify(entry.input)}
              </pre>
            }
            output={
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
            }
          />
        )}
      </article>
    </TimelineRow>
  )
}

function ExpandedDetails({ input, output }: { input: ReactNode; output: ReactNode }) {
  const { t } = useTranslation()
  return (
    <div className="grid gap-3 border-t border-border/70 px-4 pb-4 pt-3 md:grid-cols-2 animate-fade-in">
      <section className="space-y-2">
        <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
          {t("logViewer.input")}
        </p>
        {input}
      </section>
      <section className="space-y-2">
        <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
          {t("logViewer.output")}
        </p>
        {output}
      </section>
    </div>
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

function CodexCommandEntry({
  entry,
}: {
  entry: Extract<ParsedEntry, { kind: "codex-command" }>
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(entry.inProgress)

  useEffect(() => {
    if (!entry.inProgress) setOpen(false)
  }, [entry.inProgress])

  const markerClass = entry.inProgress ? "bg-primary/55" : "bg-status-idle"

  return (
    <TimelineRow markerClassName={markerClass}>
      <article className="overflow-hidden rounded-2xl border border-border/70 bg-background/80">
        <button
          type="button"
          aria-expanded={open}
          aria-label={
            open
              ? t("logViewer.collapse", { name: t("logViewer.commandExecution") })
              : t("logViewer.expand", { name: t("logViewer.commandExecution") })
          }
          onClick={() => setOpen((current) => !current)}
          className="flex w-full items-start gap-3 px-4 py-4 text-left transition-colors hover:bg-muted/25"
        >
          <span className="inline-flex h-7 shrink-0 items-center rounded-full border border-border/70 bg-background px-2.5 font-mono text-[11px] tracking-[0.18em] text-muted-foreground">
            SH
          </span>

          <div className="min-w-0 flex-1">
            <p className="text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
              {t("logViewer.commandExecution")}
            </p>
            <div className="mt-1 flex flex-wrap items-baseline gap-2">
              <p className="min-w-0 flex-1 truncate font-mono text-sm text-foreground">
                {entry.command}
              </p>
              {entry.inProgress && (
                <span className="text-[11px] text-status-working">{t("logViewer.running")}</span>
              )}
            </div>
          </div>

          <span className="mt-0.5 shrink-0 text-muted-foreground" aria-hidden="true">
            {open ? <ChevronUp className="size-4" /> : <ChevronDown className="size-4" />}
          </span>
        </button>

        {open && (
          <ExpandedDetails
            input={
              <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded-xl border border-border/70 bg-muted/35 p-3 font-mono text-[12px] leading-6 text-foreground">
                {entry.command}
              </pre>
            }
            output={
              <div className="rounded-xl border border-border/70 bg-muted/35 p-3">
                {entry.inProgress ? (
                  <p className="text-sm text-muted-foreground">{t("logViewer.running")}</p>
                ) : (
                  <pre className="overflow-x-auto whitespace-pre-wrap break-words font-mono text-[12px] leading-6 text-foreground">
                    {entry.output || "—"}
                  </pre>
                )}
              </div>
            }
          />
        )}
      </article>
    </TimelineRow>
  )
}

function CodexTurnEntry({
  entry,
}: {
  entry: Extract<ParsedEntry, { kind: "codex-turn" }>
}) {
  const { t } = useTranslation()

  return (
    <TimelineRow markerClassName="bg-muted-foreground/40">
      <div className="flex flex-wrap items-center gap-2 py-2">
        <span className="text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
          {t("logViewer.turnUsage")}
        </span>
        <MetricChip label={t("logViewer.inputTokens")} value={entry.inputTokens} />
        {entry.cachedInputTokens > 0 && (
          <MetricChip label={t("logViewer.cachedTokens")} value={entry.cachedInputTokens} />
        )}
        <MetricChip label={t("logViewer.outputTokens")} value={entry.outputTokens} />
      </div>
    </TimelineRow>
  )
}

function PiThinkingEntry({
  entry,
}: {
  entry: Extract<ParsedEntry, { kind: "pi-thinking" }>
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  return (
    <TimelineRow markerClassName="bg-muted-foreground/35">
      <article className="overflow-hidden rounded-2xl border border-border/50 bg-muted/15">
        <button
          type="button"
          aria-expanded={open}
          aria-label={t(open ? "logViewer.collapse" : "logViewer.expand", { name: t("logViewer.thinking") })}
          onClick={() => setOpen((current) => !current)}
          className="flex w-full items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-muted/25"
        >
          <p className="min-w-0 flex-1 text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground/70">
            {t("logViewer.thinking")}
          </p>
          <span className="mt-0.5 shrink-0 text-muted-foreground/60" aria-hidden="true">
            {open ? <ChevronUp className="size-4" /> : <ChevronDown className="size-4" />}
          </span>
        </button>

        {open && (
          <div className="border-t border-border/50 px-4 pb-4 pt-3">
            <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded-xl bg-muted/25 p-3 font-mono text-[12px] leading-6 text-muted-foreground">
              {entry.thinking}
            </pre>
          </div>
        )}
      </article>
    </TimelineRow>
  )
}

function RawEntry({ entry }: { entry: Extract<ParsedEntry, { kind: "raw" }> }) {
  const { t } = useTranslation()
  const isError = entry.logType === "stderr" || entry.logType === "error"

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
            {entry.lineCount}
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
  const parserRef = useRef<StreamParser | null>(null)
  const viewportRef = useRef<HTMLDivElement>(null)
  const parsedLengthRef = useRef(0)
  const pendingLineRef = useRef("")
  const prevStatusRef = useRef<ExecutionStatus>(status)

  useEffect(() => {
    setEntries([])
    toolMapRef.current = new Map()
    parserRef.current = null
    parsedLengthRef.current = 0
    pendingLineRef.current = ""
    setFollowLive(autoScroll)
  }, [executionId, autoScroll])

  useEffect(() => {
    let disposed = false

    const ensureParser = (lines: string[]): StreamParser => {
      if (!parserRef.current) {
        const engine = detectEngine(lines)
        parserRef.current = (PARSER_FACTORY[engine] ?? (() => new ClaudeParser()))()
      }
      return parserRef.current
    }

    const appendLines = (lines: string[]) => {
      if (lines.length === 0) return
      const parser = ensureParser(lines)
      setEntries((previous) => {
        const next = [...previous]
        lines.forEach((line) => parser.parseLine(line, "stdout", next, toolMapRef.current))
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

      const lines = segments.filter(Boolean)
      const nextEntries: ParsedEntry[] = []
      const nextToolMap = new Map<string, number>()
      parserRef.current = null
      if (lines.length > 0) {
        const parser = ensureParser(lines)
        lines.forEach((line) => parser.parseLine(line, "stdout", nextEntries, nextToolMap))
      }
      toolMapRef.current = nextToolMap
      setEntries(nextEntries)
    }

    const fetchLogs = async () => {
      try {
        const { content, size, truncated } = await api.executions.logs(executionId, parsedLengthRef.current)
        if (disposed) return

        const flushTail = !isActiveStatus(status)
        if (truncated) {
          rebuildEntries(content, flushTail)
          parsedLengthRef.current = size
          return
        }

        if (content.length > 0) {
          parsedLengthRef.current = size
          consumeChunk(content, flushTail)
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
      const interval = setInterval(fetchLogs, 500)
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

  const { filterOptions, visibleItems } = useMemo(() => {
    let narrativeCount = 0
    let toolCount = 0
    let rawCount = 0
    const visibleItems: Array<{ entry: ParsedEntry; index: number }> = []
    entries.forEach((entry, i) => {
      if (entry.kind === "text" || entry.kind === "pi-thinking") narrativeCount += 1
      else if (entry.kind === "tool" || entry.kind === "codex-command") toolCount += 1
      else if (entry.kind === "raw") rawCount += 1
      const visible =
        entry.kind === "result" || entry.kind === "codex-turn"
          ? true
          : filter === "all" || (FILTER_ALIAS[entry.kind] ?? entry.kind) === filter
      if (visible) visibleItems.push({ entry, index: i })
    })
    const filterOptions: Array<{ key: LogFilter; label: string; count: number }> = [
      { key: "all", label: t("logViewer.all"), count: entries.length },
      { key: "text", label: t("logViewer.narrative"), count: narrativeCount },
      { key: "tool", label: t("logViewer.tools"), count: toolCount },
      { key: "raw", label: t("logViewer.raw"), count: rawCount },
    ]
    return { filterOptions, visibleItems }
  }, [entries, filter, t])

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
        ) : visibleItems.length === 0 ? (
          <div className="rounded-2xl border border-dashed border-border/70 bg-background/70 px-4 py-6 text-sm text-muted-foreground">
            {t("logViewer.noMatches")}
          </div>
        ) : (
          <div className="relative">
            <div className="pointer-events-none absolute bottom-4 left-[0.375rem] top-4 w-px bg-border/60" />

            <div className="space-y-3">
              {visibleItems.map(({ entry, index: k }) => {
                if (entry.kind === "pi-thinking") return <PiThinkingEntry key={entry.id} entry={entry} />
                if (entry.kind === "text") return <AssistantEntry key={`text-${k}`} text={entry.text} />
                if (entry.kind === "tool") return <ToolEntry key={entry.id} entry={entry} />
                if (entry.kind === "result") return <ResultEntry key={`result-${k}`} entry={entry} />
                if (entry.kind === "codex-command") return <CodexCommandEntry key={entry.id} entry={entry} />
                if (entry.kind === "codex-turn") return <CodexTurnEntry key={`codex-turn-${k}`} entry={entry} />
                return <RawEntry key={`raw-${k}`} entry={entry} />
              })}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
