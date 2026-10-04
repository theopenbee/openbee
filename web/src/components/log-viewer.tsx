import { useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { ChevronDown, ChevronUp } from "lucide-react"
import { useTranslation } from "react-i18next"
import type { TFunction } from "i18next"
import { Streamdown } from "streamdown"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { SegmentedControl } from "@/components/segmented-control"
import { api } from "@/lib/api"
import type { ExecutionStatus } from "@/lib/types"
import { isActiveStatus } from "@/lib/format"
import { cn } from "@/lib/utils"
import { FIELD_LABEL, STREAMDOWN_BLOCKS, SURFACE } from "@/lib/styles"
import type { ParsedEntry, StreamParser } from "./log-viewer/types"
import { detectEngine } from "./log-viewer/detect-engine"
import { ClaudeParser, getToolMeta, stringify } from "./log-viewer/claude-parser"
import { CodexParser } from "./log-viewer/codex-parser"
import { PiParser } from "./log-viewer/pi-parser"

type LogFilter = "all" | "text" | "tool" | "raw"
type LogViewerVariant = "standalone" | "embedded"

const FILTER_ALIAS: Partial<Record<string, LogFilter>> = { "codex-command": "tool", "pi-thinking": "text" }

const PARSER_FACTORY: Record<string, () => StreamParser> = {
  codex: () => new CodexParser(),
  pi: () => new PiParser(),
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

// Log output (tool I/O, raw streams, results) renders as mono text in a
// recessed well inside the white panel body.
const LOG_WELL =
  "overflow-x-auto rounded-sm bg-recessed px-3 py-2.5 font-mono text-xs leading-5 break-words whitespace-pre-wrap"

// Small mono tag naming the tool family (SH, FS, WEB, TOOL).
function ToolTag({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex h-5 shrink-0 items-center rounded-sm bg-recessed px-1.5 font-mono text-xs font-medium text-muted-foreground">
      {children}
    </span>
  )
}

function Chevron({ open }: { open: boolean }) {
  return (
    <span className="shrink-0 text-muted-foreground" aria-hidden="true">
      {open ? <ChevronUp className="size-4" /> : <ChevronDown className="size-4" />}
    </span>
  )
}

// Accessible name for an entry's expand/collapse toggle. The visible state
// badge (Failed / Running) sits inside the button, but aria-label overrides the
// button's content, so the state is folded into the label itself.
function toggleLabel(t: TFunction, open: boolean, name: string, state?: string | null): string {
  const label = t(open ? "logViewer.collapse" : "logViewer.expand", { name })
  return state ? t("logViewer.toggleWithState", { label, state }) : label
}

// Header row of a collapsible entry (tool call, command, thinking).
const TOGGLE_ROW =
  "flex w-full items-center gap-2.5 px-4 py-2.5 text-left transition-colors outline-none hover:bg-elevated focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"

function MetricChip({ label, value }: { label: string; value: number }) {
  return (
    <span className="inline-flex h-6 items-center gap-1.5 rounded-sm bg-recessed px-2 text-xs" title={value.toLocaleString()}>
      <span className="font-medium text-foreground tabular-nums">{formatCount(value)}</span>
      <span className="text-muted-foreground">{label}</span>
    </span>
  )
}

function AssistantEntry({ text }: { text: string }) {
  const { t } = useTranslation()

  return (
    <div className="px-4 py-3">
      <p className={FIELD_LABEL}>{t("logViewer.assistant")}</p>
      <div className={cn("mt-1 min-w-0", STREAMDOWN_BLOCKS)}>
        <Streamdown mode="static">{text}</Streamdown>
      </div>
    </div>
  )
}

function ToolEntry({
  entry,
  live,
}: {
  entry: Extract<ParsedEntry, { kind: "tool" }>
  live: boolean
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(Boolean(entry.isError))
  const meta = getToolMeta(entry.name)
  const summary = meta.summary(entry.input)
  // A call with no result yet is only "running" while the execution is live;
  // once it ends, a missing result means the call never reported back.
  const pending = live && entry.result === undefined && !entry.isError
  const state = entry.isError ? t("logViewer.failed") : pending ? t("logViewer.running") : null

  return (
    <div>
      <button
        type="button"
        aria-expanded={open}
        aria-label={toggleLabel(t, open, entry.name, state)}
        onClick={() => setOpen((current) => !current)}
        className={TOGGLE_ROW}
      >
        <ToolTag>{meta.label}</ToolTag>
        <span className="shrink-0 text-sm font-medium text-strong">{entry.name}</span>
        <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">{summary}</span>
        {entry.isError && <Badge variant="destructive">{t("logViewer.failed")}</Badge>}
        {pending && <Badge variant="info">{t("logViewer.running")}</Badge>}
        <Chevron open={open} />
      </button>

      {open && (
        <ExpandedDetails
          input={<pre className={cn(LOG_WELL, "text-foreground")}>{stringify(entry.input)}</pre>}
          output={
            entry.result !== undefined ? (
              <pre className={cn(LOG_WELL, entry.isError ? "text-destructive-foreground" : "text-foreground")}>
                {entry.result}
              </pre>
            ) : (
              <p className="rounded-sm bg-recessed px-3 py-2.5 text-body-sm text-muted-foreground">{t("logViewer.waiting")}</p>
            )
          }
        />
      )}
    </div>
  )
}

function ExpandedDetails({ input, output }: { input: ReactNode; output: ReactNode }) {
  const { t } = useTranslation()
  return (
    <div className="grid animate-fade-in gap-3 px-4 pt-1 pb-3 md:grid-cols-2">
      <section className="min-w-0 space-y-1.5">
        <p className={FIELD_LABEL}>
          {t("logViewer.input")}
        </p>
        {input}
      </section>
      <section className="min-w-0 space-y-1.5">
        <p className={FIELD_LABEL}>
          {t("logViewer.output")}
        </p>
        {output}
      </section>
    </div>
  )
}

function resultBadgeVariant(entry: Extract<ParsedEntry, { kind: "result" }>) {
  if (entry.isError || entry.subtype.startsWith("error")) return "destructive"
  return entry.subtype === "success" ? "success" : "secondary"
}

function ResultEntry({ entry }: { entry: Extract<ParsedEntry, { kind: "result" }> }) {
  const { t } = useTranslation()

  return (
    <div className="px-4 py-3">
      <div className="flex flex-wrap items-center gap-2">
        <p className={FIELD_LABEL}>
          {t("logViewer.result")}
        </p>
        {entry.subtype && (
          <Badge variant={resultBadgeVariant(entry)} className="font-mono">
            {entry.subtype}
          </Badge>
        )}
      </div>

      <pre className={cn(LOG_WELL, "mt-1.5 text-body-sm leading-6 text-foreground")}>
        {entry.text || "—"}
      </pre>
    </div>
  )
}

function CodexCommandEntry({
  entry,
  live,
}: {
  entry: Extract<ParsedEntry, { kind: "codex-command" }>
  live: boolean
}) {
  const { t } = useTranslation()
  // Same rule as ToolEntry: an unfinished command only counts as running while
  // the execution is live; after it ends, the command never reported back.
  const running = live && entry.inProgress
  const [open, setOpen] = useState(running)

  useEffect(() => {
    if (!running) setOpen(false)
  }, [running])

  return (
    <div>
      <button
        type="button"
        aria-expanded={open}
        aria-label={toggleLabel(
          t,
          open,
          t("logViewer.commandExecution"),
          running ? t("logViewer.running") : null,
        )}
        onClick={() => setOpen((current) => !current)}
        className={TOGGLE_ROW}
      >
        <ToolTag>SH</ToolTag>
        <span className="shrink-0 text-sm font-medium text-strong">{t("logViewer.commandExecution")}</span>
        <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">{entry.command}</span>
        {running && <Badge variant="info">{t("logViewer.running")}</Badge>}
        <Chevron open={open} />
      </button>

      {open && (
        <ExpandedDetails
          input={<pre className={cn(LOG_WELL, "text-foreground")}>{entry.command}</pre>}
          output={
            running ? (
              <p className="rounded-sm bg-recessed px-3 py-2.5 text-body-sm text-muted-foreground">{t("logViewer.running")}</p>
            ) : (
              <pre className={cn(LOG_WELL, "text-foreground")}>{entry.output || "—"}</pre>
            )
          }
        />
      )}
    </div>
  )
}

function CodexTurnEntry({
  entry,
}: {
  entry: Extract<ParsedEntry, { kind: "codex-turn" }>
}) {
  const { t } = useTranslation()

  return (
    <div className="flex flex-wrap items-center gap-2 px-4 py-2.5">
      <span className={FIELD_LABEL}>
        {t("logViewer.turnUsage")}
      </span>
      <MetricChip label={t("logViewer.inputTokens")} value={entry.inputTokens} />
      {entry.cachedInputTokens > 0 && (
        <MetricChip label={t("logViewer.cachedTokens")} value={entry.cachedInputTokens} />
      )}
      <MetricChip label={t("logViewer.outputTokens")} value={entry.outputTokens} />
    </div>
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
    <div>
      <button
        type="button"
        aria-expanded={open}
        aria-label={toggleLabel(t, open, t("logViewer.thinking"))}
        onClick={() => setOpen((current) => !current)}
        className={TOGGLE_ROW}
      >
        <span className="min-w-0 flex-1 text-body-sm font-medium text-muted-foreground">
          {t("logViewer.thinking")}
        </span>
        <Chevron open={open} />
      </button>

      {open && (
        <div className="animate-fade-in px-4 pt-1 pb-3">
          <pre className={cn(LOG_WELL, "text-muted-foreground")}>
            {entry.thinking}
          </pre>
        </div>
      )}
    </div>
  )
}

function RawEntry({ entry }: { entry: Extract<ParsedEntry, { kind: "raw" }> }) {
  const { t } = useTranslation()
  const isError = entry.logType === "stderr" || entry.logType === "error"

  return (
    <div className="px-4 py-3">
      <div className="flex flex-wrap items-center gap-2">
        <p className={FIELD_LABEL}>
          {t("logViewer.rawOutput")}
        </p>
        {isError && <Badge variant="destructive">{t("logViewer.failed")}</Badge>}
        <span className="ml-auto text-xs text-muted-foreground tabular-nums">{entry.lineCount}</span>
      </div>

      <pre className={cn(LOG_WELL, "mt-1.5", isError ? "text-destructive-foreground" : "text-foreground")}>
        {entry.content}
      </pre>
    </div>
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

  const live = isActiveStatus(status)
  const shellClassName = variant === "embedded" ? "overflow-hidden" : SURFACE

  return (
    <div className={shellClassName}>
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2 border-b border-border px-4 py-2.5">
        <SegmentedControl
          ariaLabel={t("logViewer.filterLabel")}
          value={filter}
          onChange={setFilter}
          options={filterOptions.map((option) => ({
            value: option.key,
            label: (
              <>
                {option.label}
                <span className="text-xs font-normal text-muted-foreground">{option.count}</span>
              </>
            ),
          }))}
        />

        {isActiveStatus(status) &&
          (followLive ? (
            <Badge variant="info">
              <span className="size-1.5 animate-presence-pulse rounded-full bg-current" aria-hidden="true" />
              {t("logViewer.followLive")}
            </Badge>
          ) : (
            <Button variant="outline" size="sm" onClick={jumpToLatest}>
              {t("logViewer.jumpToLatest")}
            </Button>
          ))}
      </div>

      <div
        ref={viewportRef}
        onScroll={handleViewportScroll}
        className="max-h-[min(70vh,52rem)] overflow-y-auto"
      >
        {entries.length === 0 ? (
          <div className="px-4 py-10 text-center text-sm text-muted-foreground">
            {isActiveStatus(status) ? (
              <span className="inline-flex items-center gap-2">
                <span className="size-1.5 animate-presence-pulse rounded-full bg-status-working" aria-hidden="true" />
                {t("logViewer.waiting")}
              </span>
            ) : (
              t("logViewer.noLogs")
            )}
          </div>
        ) : visibleItems.length === 0 ? (
          <div className="px-4 py-10 text-center text-sm text-muted-foreground">
            {t("logViewer.noMatches")}
          </div>
        ) : (
          <div className="divide-y divide-hairline">
            {visibleItems.map(({ entry, index: k }) => {
              if (entry.kind === "pi-thinking") return <PiThinkingEntry key={entry.id} entry={entry} />
              if (entry.kind === "text") return <AssistantEntry key={`text-${k}`} text={entry.text} />
              if (entry.kind === "tool") return <ToolEntry key={entry.id} entry={entry} live={live} />
              if (entry.kind === "result") return <ResultEntry key={`result-${k}`} entry={entry} />
              if (entry.kind === "codex-command") return <CodexCommandEntry key={entry.id} entry={entry} live={live} />
              if (entry.kind === "codex-turn") return <CodexTurnEntry key={`codex-turn-${k}`} entry={entry} />
              return <RawEntry key={`raw-${k}`} entry={entry} />
            })}
          </div>
        )}
      </div>
    </div>
  )
}
