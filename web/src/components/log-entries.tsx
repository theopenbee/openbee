import { useEffect, useState, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { AnimatePresence, motion, useReducedMotion } from "motion/react"
import { ChevronDown, FileText, Globe, Lightbulb, SquareTerminal, TriangleAlert, Wrench, type LucideIcon } from "lucide-react"
import { Badge } from "@/components/arc/badge/badge"
import { TextShimmer } from "@/components/arc/text-shimmer/text-shimmer"
import { motionTokens } from "@/components/arc/lib/motion-tokens"
import { ChatMarkdown } from "@/components/chat-markdown"
import { CopyButton } from "@/components/copy-button"
import { cn } from "@/lib/utils"
import type { ParsedEntry } from "./log-viewer/types"
import { getToolMeta, stringify } from "./log-viewer/claude-parser"

const TOOL_ICONS: Record<string, LucideIcon> = {
  SH: SquareTerminal,
  FS: FileText,
  WEB: Globe,
}

const PRE_CLASS = "whitespace-pre-wrap break-words font-mono text-xs leading-relaxed"

function formatCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(n % 1_000_000 === 0 ? 0 : 1)}m`
  if (n >= 1_000) return `${(n / 1_000).toFixed(n % 1_000 === 0 ? 0 : 1)}k`
  return String(n)
}

function toolMetaFor(name: string) {
  return getToolMeta(name ? name.charAt(0).toUpperCase() + name.slice(1) : name)
}

function Collapse({ open, children }: { open: boolean; children: ReactNode }) {
  const reduced = useReducedMotion()
  return (
    <AnimatePresence initial={false}>
      {open && (
        <motion.div
          key="content"
          className="overflow-hidden"
          initial={{ height: 0, opacity: 0 }}
          animate={{ height: "auto", opacity: 1 }}
          exit={{ height: 0, opacity: 0 }}
          transition={
            reduced
              ? { duration: 0 }
              : {
                  height: motionTokens.spring.smooth,
                  opacity: { duration: motionTokens.duration.fast, ease: [...motionTokens.ease.standard] },
                }
          }
        >
          {children}
        </motion.div>
      )}
    </AnimatePresence>
  )
}

function Chevron({ open }: { open: boolean }) {
  const reduced = useReducedMotion()
  return (
    <motion.span
      aria-hidden="true"
      className="inline-flex shrink-0 text-muted"
      initial={false}
      animate={{ rotate: open ? 180 : 0 }}
      transition={reduced ? { duration: 0 } : motionTokens.spring.snappy}
    >
      <ChevronDown size={16} strokeWidth={1.75} />
    </motion.span>
  )
}

function ToolSection({
  label,
  copyValue,
  children,
}: {
  label: string
  copyValue?: string
  children: ReactNode
}) {
  return (
    <section className="border-t border-border-subtle px-4 py-3 first:border-t-0">
      <div className="flex min-h-6 items-center justify-between gap-2">
        <h4 className="text-xs text-secondary">{label}</h4>
        {copyValue ? (
          <span className="-my-2 -mr-2 inline-flex">
            <CopyButton value={copyValue} />
          </span>
        ) : null}
      </div>
      <div className="mt-1 min-w-0">{children}</div>
    </section>
  )
}

function ToolBlock({
  icon: Icon,
  title,
  summary,
  status,
  open,
  onToggle,
  children,
}: {
  icon: LucideIcon
  title: string
  summary?: string
  status?: ReactNode
  open: boolean
  onToggle: () => void
  children: ReactNode
}) {
  return (
    <article className="min-w-0 overflow-hidden rounded-inset border border-border bg-surface">
      <button
        type="button"
        aria-expanded={open}
        onClick={onToggle}
        className="flex min-h-11 w-full items-center gap-3 px-4 py-2 text-left transition-colors hover:bg-surface-muted"
      >
        <Icon size={16} strokeWidth={1.75} aria-hidden="true" className="shrink-0 text-muted" />
        <span className="shrink-0 text-sm font-medium text-foreground">{title}</span>
        {summary ? (
          <span className="min-w-0 flex-1 truncate font-mono text-xs text-secondary">{summary}</span>
        ) : (
          <span className="flex-1" />
        )}
        {status}
        <Chevron open={open} />
      </button>
      <Collapse open={open}>
        <div className="border-t border-border-subtle bg-surface-muted">{children}</div>
      </Collapse>
    </article>
  )
}

export function AssistantEntry({ text }: { text: string }) {
  const { t } = useTranslation()
  return (
    <article className="min-w-0 py-1">
      <h3 className="mb-2 text-xs font-medium text-secondary">{t("logViewer.assistant")}</h3>
      <ChatMarkdown>{text}</ChatMarkdown>
    </article>
  )
}

export function ThinkingEntry({ entry }: { entry: Extract<ParsedEntry, { kind: "pi-thinking" }> }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  return (
    <article className="min-w-0">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        className="-mx-3 inline-flex min-h-9 items-center gap-2 rounded-control px-3 text-xs font-medium text-secondary transition-colors hover:bg-surface-muted hover:text-foreground"
      >
        <Lightbulb size={16} strokeWidth={1.75} aria-hidden="true" className="text-muted" />
        {t("logViewer.thinking")}
        <Chevron open={open} />
      </button>
      <Collapse open={open}>
        <p className="mt-2 whitespace-pre-wrap break-words border-l-2 border-border pl-4 text-sm leading-relaxed text-secondary">
          {entry.thinking}
        </p>
      </Collapse>
    </article>
  )
}

export function ToolEntry({ entry, live }: { entry: Extract<ParsedEntry, { kind: "tool" }>; live: boolean }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(Boolean(entry.isError))
  const meta = toolMetaFor(entry.name)
  const input = stringify(entry.input)

  const status = entry.isError ? (
    <Badge tone="danger" size="sm">{t("logViewer.failed")}</Badge>
  ) : entry.result === undefined && live ? (
    <TextShimmer className="shrink-0 text-xs text-secondary">{t("logViewer.running")}</TextShimmer>
  ) : null

  return (
    <ToolBlock
      icon={TOOL_ICONS[meta.label] ?? Wrench}
      title={entry.name}
      summary={meta.summary(entry.input)}
      status={status}
      open={open}
      onToggle={() => setOpen((current) => !current)}
    >
      <ToolSection label={t("logViewer.input")} copyValue={input}>
        <pre className={cn(PRE_CLASS, "text-foreground")}>{input}</pre>
      </ToolSection>
      <ToolSection label={t("logViewer.output")} copyValue={entry.result || undefined}>
        {entry.result !== undefined ? (
          <pre className={cn(PRE_CLASS, entry.isError ? "text-danger" : "text-foreground")}>
            {entry.result || t("logViewer.emptyOutput")}
          </pre>
        ) : live ? (
          <TextShimmer className="text-xs text-secondary">{t("logViewer.waiting")}</TextShimmer>
        ) : (
          <p className="text-xs text-secondary">{t("logViewer.noOutput")}</p>
        )}
      </ToolSection>
    </ToolBlock>
  )
}

export function CommandEntry({ entry }: { entry: Extract<ParsedEntry, { kind: "codex-command" }> }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(entry.inProgress)

  useEffect(() => {
    if (!entry.inProgress) setOpen(false)
  }, [entry.inProgress])

  return (
    <ToolBlock
      icon={SquareTerminal}
      title={t("logViewer.commandExecution")}
      summary={entry.command}
      status={
        entry.inProgress ? (
          <TextShimmer className="shrink-0 text-xs text-secondary">{t("logViewer.running")}</TextShimmer>
        ) : null
      }
      open={open}
      onToggle={() => setOpen((current) => !current)}
    >
      <ToolSection label={t("logViewer.input")} copyValue={entry.command}>
        <pre className={cn(PRE_CLASS, "text-foreground")}>{entry.command}</pre>
      </ToolSection>
      <ToolSection label={t("logViewer.output")} copyValue={entry.inProgress ? undefined : entry.output || undefined}>
        {entry.inProgress ? (
          <TextShimmer className="text-xs text-secondary">{t("logViewer.running")}</TextShimmer>
        ) : (
          <pre className={cn(PRE_CLASS, "text-foreground")}>{entry.output || t("logViewer.emptyOutput")}</pre>
        )}
      </ToolSection>
    </ToolBlock>
  )
}

export function ResultEntry({ entry }: { entry: Extract<ParsedEntry, { kind: "result" }> }) {
  const { t } = useTranslation()
  const succeeded = entry.subtype === "success"
  const failed = entry.subtype.startsWith("error")

  return (
    <article className="min-w-0 rounded-inset bg-surface-muted px-4 py-3">
      <div className="flex min-h-6 flex-wrap items-center gap-2">
        <h3 className="text-xs font-medium text-secondary">{t("logViewer.result")}</h3>
        {entry.subtype ? (
          <Badge tone={succeeded ? "success" : failed ? "danger" : "neutral"} size="sm">
            {succeeded ? t("logViewer.succeeded") : entry.subtype}
          </Badge>
        ) : null}
      </div>
      <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-relaxed text-foreground">
        {entry.text || t("logViewer.emptyOutput")}
      </p>
    </article>
  )
}

export function TurnUsageEntry({ entry }: { entry: Extract<ParsedEntry, { kind: "codex-turn" }> }) {
  const { t } = useTranslation()
  const parts = [
    { label: t("logViewer.inputTokens"), value: entry.inputTokens },
    ...(entry.cachedInputTokens > 0 ? [{ label: t("logViewer.cachedTokens"), value: entry.cachedInputTokens }] : []),
    { label: t("logViewer.outputTokens"), value: entry.outputTokens },
  ]

  return (
    <div className="flex items-center gap-3 py-1 text-xs text-secondary">
      <span aria-hidden="true" className="min-w-4 flex-1 border-t border-border-subtle" />
      <p className="flex flex-wrap items-center justify-center gap-x-2 gap-y-1 tabular">
        <span className="font-medium">{t("logViewer.turnUsage")}</span>
        {parts.map((part) => (
          <span key={part.label} title={part.value.toLocaleString()}>
            <span className="text-foreground">{formatCount(part.value)}</span> {part.label}
          </span>
        ))}
      </p>
      <span aria-hidden="true" className="min-w-4 flex-1 border-t border-border-subtle" />
    </div>
  )
}

export function RawEntry({ entry }: { entry: Extract<ParsedEntry, { kind: "raw" }> }) {
  const { t } = useTranslation()
  const isError = entry.logType === "stderr" || entry.logType === "error"

  return (
    <article className="min-w-0 overflow-hidden rounded-inset border border-border bg-surface">
      <div className="flex min-h-11 items-center gap-3 px-4 py-1">
        {isError ? (
          <TriangleAlert size={16} strokeWidth={1.75} aria-hidden="true" className="shrink-0 text-danger" />
        ) : (
          <SquareTerminal size={16} strokeWidth={1.75} aria-hidden="true" className="shrink-0 text-muted" />
        )}
        <h3 className="text-sm font-medium text-foreground">
          {isError ? t("logViewer.errorOutput") : t("logViewer.rawOutput")}
        </h3>
        <span className="text-xs text-secondary tabular">{t("logViewer.lineCount", { count: entry.lineCount })}</span>
        <span className="flex-1" />
        {isError ? <Badge tone="danger" size="sm">{t("logViewer.failed")}</Badge> : null}
        <span className="-mr-2 inline-flex">
          <CopyButton value={entry.content} />
        </span>
      </div>
      <pre
        className={cn(
          PRE_CLASS,
          "overflow-x-auto border-t border-border-subtle bg-surface-muted px-4 py-3",
          isError ? "text-danger" : "text-secondary"
        )}
      >
        {entry.content}
      </pre>
    </article>
  )
}
