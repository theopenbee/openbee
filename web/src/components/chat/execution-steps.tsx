import { useId, useLayoutEffect, useState, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import {
  CheckIcon,
  ChevronDownIcon,
  CircleAlertIcon,
  CircleSlashIcon,
  FilePenLineIcon,
  FileTextIcon,
  GlobeIcon,
  LoaderCircleIcon,
  MessageSquareTextIcon,
  SearchIcon,
  SendIcon,
  SparklesIcon,
  TerminalIcon,
  WrenchIcon,
  XIcon,
  type LucideIcon,
} from "lucide-react"
import { TextShimmer } from "@/components/arc/text-shimmer/text-shimmer"
import { ChatMarkdown } from "@/components/chat-markdown"
import { diffLines, diffStats, splitThinking, toolDuration, toolKind, toolOutput, toolTitle, type DiffLine } from "@/lib/execution"
import { observeResize } from "@/lib/resize-observer"
import type { ExecutionStep, ToolKind, ToolStep } from "@/lib/types"
import { cn } from "@/lib/utils"

const TOOL_ICONS: Record<ToolKind, LucideIcon> = {
  command: TerminalIcon,
  read: FileTextIcon,
  edit: FilePenLineIcon,
  search: SearchIcon,
  web: GlobeIcon,
  message: SendIcon,
  other: WrenchIcon,
}

const ICON = { size: 16, strokeWidth: 1.75, "aria-hidden": true } as const

interface ExecutionStepsProps {
  steps: ExecutionStep[]
  running: boolean
  failure: string | null
}

export function ExecutionSteps({ steps, running, failure }: ExecutionStepsProps) {
  return (
    <ol className="-mx-3 grid gap-0.5">
      {steps.map((step, index) => (
        <StepItem key={index} step={step} running={running} />
      ))}
      {failure && <FailureItem message={failure} />}
    </ol>
  )
}

function StepItem({ step, running }: { step: ExecutionStep; running: boolean }) {
  if (step.type === "tool") return <ToolStepItem step={step} running={running} />
  if (step.type === "thinking") return <ThinkingStep text={step.text} />
  if (step.type === "error") return <ErrorStep text={step.text} />
  return <TextStep text={step.text} />
}

function overflowing(el: HTMLElement) {
  return [el, ...el.querySelectorAll("span")].some((node) => node.scrollWidth > node.clientWidth + 1)
}

function useTruncated(text: string, shimmer = false) {
  const [node, setNode] = useState<HTMLElement | null>(null)
  const [truncated, setTruncated] = useState(false)
  useLayoutEffect(() => {
    if (!node) return
    const measure = () => setTruncated(overflowing(node))
    measure()
    return observeResize(node, measure)
  }, [node, text, shimmer])
  return [setNode, truncated] as const
}

interface StepRowProps {
  icon: ReactNode
  title: ReactNode
  meta?: ReactNode
  label?: string
  expandable: boolean
  open: boolean
  bodyId: string
  onToggle: () => void
}

function StepRow({ icon, title, meta, label, expandable, open, bodyId, onToggle }: StepRowProps) {
  const content = (
    <>
      {icon}
      {label && <span className="sr-only">{label}</span>}
      {title}
      <span className="flex shrink-0 items-center gap-2 text-muted">
        {meta}
        {expandable ? (
          <ChevronDownIcon {...ICON} className={cn("transition-transform duration-150", open && "rotate-180")} />
        ) : (
          <span aria-hidden="true" className="size-4" />
        )}
      </span>
    </>
  )
  const row = "flex h-9 w-full min-w-0 items-center gap-3 rounded-control px-3 text-left text-sm"
  if (!expandable) return <div className={row}>{content}</div>
  return (
    <button
      type="button"
      aria-expanded={open}
      aria-controls={open ? bodyId : undefined}
      onClick={onToggle}
      className={cn(row, "cursor-pointer transition-colors hover:bg-surface-muted")}
    >
      {content}
    </button>
  )
}

function ThinkingStep({ text }: { text: string }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const bodyId = useId()
  const { title, rest } = splitThinking(text)
  const [titleRef, truncated] = useTruncated(title)
  const expandable = rest !== "" || truncated

  return (
    <li className="min-w-0">
      <StepRow
        icon={<SparklesIcon {...ICON} className="shrink-0 text-secondary" />}
        label={t("execution.thinking")}
        title={
          <span ref={titleRef} className="min-w-0 flex-1 truncate text-foreground">
            {title}
          </span>
        }
        expandable={expandable}
        open={open}
        bodyId={bodyId}
        onToggle={() => setOpen((value) => !value)}
      />
      {expandable && open && (
        <p id={bodyId} className="pt-1 pr-3 pb-2 pl-10 text-sm whitespace-pre-wrap break-words text-secondary">
          {truncated ? text.trim() : rest}
        </p>
      )}
    </li>
  )
}

function TextStep({ text }: { text: string }) {
  return (
    <li className="flex min-w-0 gap-3 px-3 py-2">
      <MessageSquareTextIcon {...ICON} className="mt-0.5 shrink-0 text-secondary" />
      <div className="min-w-0 flex-1 text-sm text-foreground">
        <ChatMarkdown breaks>{text}</ChatMarkdown>
      </div>
    </li>
  )
}

function ErrorStep({ text }: { text: string }) {
  return (
    <li className="flex min-w-0 gap-3 px-3 py-2 text-danger">
      <CircleAlertIcon {...ICON} className="mt-0.5 shrink-0" />
      <p className="min-w-0 flex-1 text-sm whitespace-pre-wrap break-words">{text}</p>
    </li>
  )
}

function ToolMark({ step, active }: { step: ToolStep; active: boolean }) {
  const { t } = useTranslation()
  if (step.status === "error") {
    return (
      <span className="inline-flex shrink-0 items-center text-danger">
        <XIcon {...ICON} />
        <span className="sr-only">{t("execution.status.failed")}</span>
      </span>
    )
  }
  if (step.status === "done") return <CheckIcon {...ICON} className="shrink-0 text-success" />
  if (active) {
    return (
      <span className="inline-flex shrink-0 items-center text-secondary">
        <LoaderCircleIcon {...ICON} className="animate-spin" />
        <span className="sr-only">{t("execution.status.running")}</span>
      </span>
    )
  }
  const stopped = t("execution.toolStopped")
  return (
    <span className="inline-flex shrink-0 items-center text-muted" title={stopped}>
      <CircleSlashIcon {...ICON} />
      <span className="sr-only">{stopped}</span>
    </span>
  )
}

function ToolStepItem({ step, running }: { step: ToolStep; running: boolean }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const bodyId = useId()
  const kind = toolKind(step)
  const Icon = TOOL_ICONS[kind]
  const active = running && step.status === "running"
  const title = toolTitle(step, t)
  const [titleRef, truncated] = useTruncated(title, active)
  const summary = step.summary?.trim() ?? ""
  const duration = toolDuration(step.duration_ms, t)
  const hasBody = Boolean(toolOutput(step) || step.diff)
  const expandable = hasBody || (summary !== "" && (kind === "message" || kind === "other" || truncated))

  return (
    <li className="min-w-0">
      <StepRow
        icon={<Icon {...ICON} className={cn("shrink-0", step.status === "error" ? "text-danger" : "text-secondary")} />}
        title={
          <span ref={titleRef} title={summary ? title : undefined} className="min-w-0 flex-1 truncate text-foreground">
            {active ? <TextShimmer className="[&_span]:truncate [&_span]:align-top">{title}</TextShimmer> : title}
          </span>
        }
        meta={
          <>
            {duration && <span className="text-xs tabular">{duration}</span>}
            <ToolMark step={step} active={active} />
          </>
        }
        expandable={expandable}
        open={open}
        bodyId={bodyId}
        onToggle={() => setOpen((value) => !value)}
      />
      {expandable && open && <ToolDetail id={bodyId} step={step} />}
    </li>
  )
}

export function ToolDetail({ id, step }: { id?: string; step: ToolStep }) {
  const summary = step.summary?.trim() ?? ""
  const diff = step.diff ? diffLines(step.diff) : null
  const output = toolOutput(step)
  const hasBody = Boolean(output || diff)

  return (
    <DetailBox id={id}>
      {summary && (
        <div className="flex items-start gap-2 px-3 py-2">
          <p
            className={cn(
              "min-w-0 flex-1 break-words whitespace-pre-wrap text-foreground",
              toolKind(step) === "message" ? "text-sm" : "font-mono text-xs leading-relaxed",
            )}
          >
            {summary}
          </p>
          {diff && <DiffStats lines={diff} />}
        </div>
      )}
      {hasBody && (
        <div className={cn(summary && "border-t border-border")}>
          {diff && <DiffView lines={diff} />}
          {output && (
            <pre
              className={cn(
                "max-h-64 overflow-auto px-3 py-2 font-mono text-xs leading-relaxed whitespace-pre-wrap break-words",
                step.status === "error" ? "text-danger" : "text-secondary",
                diff && "border-t border-border",
              )}
            >
              {output}
            </pre>
          )}
        </div>
      )}
    </DetailBox>
  )
}

function DetailBox({ id, tone, children }: { id?: string; tone?: "danger"; children: ReactNode }) {
  return (
    <div
      id={id}
      className={cn(
        "mt-1 mr-3 mb-2 ml-10 overflow-hidden rounded-inset border border-border",
        tone === "danger" && "border-danger/25 bg-danger/10",
      )}
    >
      {children}
    </div>
  )
}

function DiffStats({ lines }: { lines: DiffLine[] }) {
  const { added, removed } = diffStats(lines)
  return (
    <span className="flex shrink-0 gap-1.5 font-mono text-xs leading-relaxed tabular">
      {added > 0 && <span className="text-success">+{added}</span>}
      {removed > 0 && <span className="text-danger">-{removed}</span>}
    </span>
  )
}

function DiffView({ lines }: { lines: DiffLine[] }) {
  return (
    <div className="max-h-64 overflow-auto py-1.5 font-mono text-xs leading-relaxed">
      {lines.map((line, index) => (
        <div
          key={index}
          className={cn(
            "flex gap-2 px-3 whitespace-pre-wrap break-words",
            line.type === "add" && "bg-success/10 text-foreground",
            line.type === "remove" && "bg-danger/10 text-foreground",
            line.type === "context" && "text-secondary",
            line.type === "note" && "text-muted",
          )}
        >
          <span
            className={cn(
              "w-2 shrink-0 select-none",
              line.type === "add" && "text-success",
              line.type === "remove" && "text-danger",
            )}
          >
            {line.type === "add" ? "+" : line.type === "remove" ? "-" : " "}
          </span>
          <span className="min-w-0 flex-1">{line.text || " "}</span>
        </div>
      ))}
    </div>
  )
}

function FailureItem({ message }: { message: string }) {
  const { t } = useTranslation()
  return (
    <li className="min-w-0">
      <div className="flex h-9 min-w-0 items-center gap-3 px-3 text-sm">
        <CircleAlertIcon {...ICON} className="shrink-0 text-danger" />
        <span className="min-w-0 flex-1 truncate text-foreground">{t("execution.failedTitle")}</span>
      </div>
      <DetailBox tone="danger">
        <p className="px-3 py-2 text-sm whitespace-pre-wrap break-words text-danger">{message}</p>
      </DetailBox>
    </li>
  )
}
