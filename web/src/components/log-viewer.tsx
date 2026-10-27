import { useEffect, useMemo, useRef, useState } from "react"
import { ArrowDown } from "lucide-react"
import { useTranslation } from "react-i18next"
import { Button } from "@/components/arc/button/button"
import SegmentedControl from "@/components/arc/segmented-control/segmented-control"
import { TextShimmer } from "@/components/arc/text-shimmer/text-shimmer"
import { api } from "@/lib/api"
import type { ExecutionStatus } from "@/lib/types"
import { isActiveStatus } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { ParsedEntry, StreamParser } from "./log-viewer/types"
import { detectEngine } from "./log-viewer/detect-engine"
import { ClaudeParser } from "./log-viewer/claude-parser"
import { CodexParser } from "./log-viewer/codex-parser"
import { PiParser } from "./log-viewer/pi-parser"
import {
  AssistantEntry,
  CommandEntry,
  RawEntry,
  ResultEntry,
  ThinkingEntry,
  ToolEntry,
  TurnUsageEntry,
} from "./log-entries"

type LogFilter = "all" | "text" | "tool" | "raw"
type LogViewerVariant = "standalone" | "embedded"

const FILTER_ALIAS: Partial<Record<string, LogFilter>> = { "codex-command": "tool", "pi-thinking": "text" }

const PARSER_FACTORY: Record<string, () => StreamParser> = {
  codex: () => new CodexParser(),
  pi: () => new PiParser(),
}

const FOLLOW_THRESHOLD_PX = 48

interface LogViewerProps {
  executionId: string
  status: ExecutionStatus
  onComplete?: () => void
  autoScroll?: boolean
  variant?: LogViewerVariant
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
  const live = isActiveStatus(status)

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
        return
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
    if (!autoScroll || !live) return
    const viewport = viewportRef.current
    if (!viewport) return

    const atBottom = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight < FOLLOW_THRESHOLD_PX
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

  const segments = filterOptions.map((option) => ({
    value: option.key,
    label: option.label,
    accessory: <span className="ml-1.5 text-muted tabular">{option.count}</span>,
  }))

  return (
    <div
      className={cn(
        "min-w-0",
        variant === "standalone" && "overflow-hidden rounded-surface border border-border bg-surface pt-5"
      )}
    >
      <div className="flex flex-wrap items-center justify-between gap-3 px-6 pb-4">
        <div className="min-w-0 max-w-full">
          <SegmentedControl
            label={t("logViewer.filterLabel")}
            options={segments}
            value={filter}
            onValueChange={(value) => setFilter(value as LogFilter)}
          />
        </div>

        {live && (
          <div className="flex min-h-9 items-center" aria-live="polite">
            {followLive ? (
              <span className="inline-flex items-center gap-2 text-xs text-secondary">
                <span aria-hidden="true" className="size-1.5 rounded-full bg-accent animate-pulse-soft" />
                <TextShimmer>{t("logViewer.followLive")}</TextShimmer>
              </span>
            ) : (
              <Button variant="secondary" size="sm" onClick={jumpToLatest}>
                <ArrowDown size={16} strokeWidth={1.75} aria-hidden="true" />
                {t("logViewer.jumpToLatest")}
              </Button>
            )}
          </div>
        )}
      </div>

      <div
        ref={viewportRef}
        onScroll={handleViewportScroll}
        className="max-h-[min(70vh,52rem)] overflow-y-auto overscroll-contain border-t border-border-subtle px-6 py-5"
      >
        {entries.length === 0 ? (
          <div className="py-10 text-center text-sm text-secondary">
            {live ? <TextShimmer>{t("logViewer.waiting")}</TextShimmer> : t("logViewer.noLogs")}
          </div>
        ) : visibleItems.length === 0 ? (
          <div className="flex flex-col items-center gap-3 py-10 text-center">
            <p className="text-sm text-secondary">{t("logViewer.noMatches")}</p>
            <Button variant="ghost" size="sm" onClick={() => setFilter("all")}>
              {t("logViewer.showAll")}
            </Button>
          </div>
        ) : (
          <div className="flex flex-col gap-3">
            {visibleItems.map(({ entry, index: k }) => {
              if (entry.kind === "pi-thinking") return <ThinkingEntry key={entry.id} entry={entry} />
              if (entry.kind === "text") return <AssistantEntry key={`text-${k}`} text={entry.text} />
              if (entry.kind === "tool") return <ToolEntry key={entry.id} entry={entry} live={live} />
              if (entry.kind === "result") return <ResultEntry key={`result-${k}`} entry={entry} />
              if (entry.kind === "codex-command") return <CommandEntry key={entry.id} entry={entry} />
              if (entry.kind === "codex-turn") return <TurnUsageEntry key={`codex-turn-${k}`} entry={entry} />
              return <RawEntry key={`raw-${k}`} entry={entry} />
            })}
          </div>
        )}
      </div>
    </div>
  )
}
