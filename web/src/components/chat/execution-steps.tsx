import { useState, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import {
  BrainIcon,
  CheckIcon,
  ChevronDownIcon,
  CircleAlertIcon,
  CircleSlashIcon,
  FilePenLineIcon,
  FileTextIcon,
  GlobeIcon,
  LoaderCircleIcon,
  SearchIcon,
  SendIcon,
  TerminalIcon,
  WrenchIcon,
  XIcon,
  type LucideIcon,
} from "lucide-react"
import { ExpandableText } from "@/components/arc/expandable-text/expandable-text"
import { TextShimmer } from "@/components/arc/text-shimmer/text-shimmer"
import { ChatMarkdown } from "@/components/chat-markdown"
import { isToolKind, toolDuration } from "@/lib/execution"
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

interface ExecutionStepsProps {
  steps: ExecutionStep[]
  running: boolean
  failure: string | null
}

export function ExecutionSteps({ steps, running, failure }: ExecutionStepsProps) {
  return (
    <ol className="grid gap-5">
      {steps.map((step, index) => (
        <StepItem key={index} step={step} running={running} last={index === steps.length - 1 && !failure} />
      ))}
      {failure && <FailureItem message={failure} />}
    </ol>
  )
}

function StepIcon({ children, tone }: { children: ReactNode; tone?: "danger" | "active" }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "relative z-10 grid size-7 shrink-0 place-items-center rounded-full border border-border bg-surface text-secondary",
        tone === "danger" && "border-danger/30 bg-danger/10 text-danger",
        tone === "active" && "border-border-strong text-foreground",
      )}
    >
      {children}
    </span>
  )
}

function StepItem({ step, running, last }: { step: ExecutionStep; running: boolean; last: boolean }) {
  return (
    <li className="relative flex min-w-0 gap-3">
      {!last && <span aria-hidden="true" className="absolute top-8 bottom-[-1.25rem] left-3.5 w-px bg-border" />}
      {step.type === "thinking" && <ThinkingStep text={step.text} />}
      {step.type === "text" && <TextStep text={step.text} />}
      {step.type === "error" && <ErrorStep text={step.text} />}
      {step.type === "tool" && <ToolStepItem step={step} running={running} />}
    </li>
  )
}

function ThinkingStep({ text }: { text: string }) {
  const { t } = useTranslation()
  return (
    <>
      <StepIcon>
        <BrainIcon size={14} strokeWidth={1.75} />
      </StepIcon>
      <div className="grid min-w-0 flex-1 gap-1 pt-1">
        <span className="text-xs text-muted">{t("execution.thinking")}</span>
        <ExpandableText lines={3}>
          <p className="whitespace-pre-line">{text}</p>
        </ExpandableText>
      </div>
    </>
  )
}

function TextStep({ text }: { text: string }) {
  return (
    <>
      <StepIcon tone="active">
        <span className="size-1.5 rounded-full bg-current" />
      </StepIcon>
      <div className="min-w-0 flex-1 pt-0.5 text-sm text-foreground">
        <ChatMarkdown breaks>{text}</ChatMarkdown>
      </div>
    </>
  )
}

function ToolMark({ step, active }: { step: ToolStep; active: boolean }) {
  const { t } = useTranslation()
  if (step.status === "error") {
    return (
      <span className="inline-flex shrink-0 items-center text-danger">
        <XIcon size={14} strokeWidth={2} aria-hidden="true" />
        <span className="sr-only">{t("execution.status.failed")}</span>
      </span>
    )
  }
  if (step.status === "done") return <CheckIcon size={14} strokeWidth={2} className="shrink-0 text-success" aria-hidden="true" />
  if (active) return null
  const stopped = t("execution.toolStopped")
  return (
    <span className="inline-flex shrink-0 items-center text-muted" title={stopped}>
      <CircleSlashIcon size={14} strokeWidth={1.75} aria-hidden="true" />
      <span className="sr-only">{stopped}</span>
    </span>
  )
}

function ToolStepItem({ step, running }: { step: ToolStep; running: boolean }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const kind = isToolKind(step.kind) ? step.kind : "other"
  const Icon = TOOL_ICONS[kind]
  const active = running && step.status === "running"
  const failed = step.status === "error"
  const label = kind === "other" ? t("execution.tool.other", { name: step.name }) : t(`execution.tool.${kind}`)
  const duration = toolDuration(step.duration_ms, t)
  const header = (
    <>
      {active ? (
        <TextShimmer className="truncate text-sm font-medium text-foreground">{label}</TextShimmer>
      ) : (
        <span className="truncate font-medium text-foreground">{label}</span>
      )}
      <ToolMark step={step} active={active} />
      <span className="ml-auto flex shrink-0 items-center gap-1 text-xs text-muted tabular">
        {duration}
        {step.output && (
          <ChevronDownIcon
            size={14}
            strokeWidth={1.75}
            aria-hidden="true"
            className={cn("transition-transform duration-150", open && "rotate-180")}
          />
        )}
      </span>
    </>
  )

  return (
    <>
      <StepIcon tone={failed ? "danger" : active ? "active" : undefined}>
        {active ? (
          <LoaderCircleIcon size={14} strokeWidth={1.75} className="animate-spin" />
        ) : (
          <Icon size={14} strokeWidth={1.75} />
        )}
      </StepIcon>
      <div className="grid min-w-0 flex-1 gap-1.5 pt-1">
        {step.output ? (
          <button
            type="button"
            onClick={() => setOpen((value) => !value)}
            aria-expanded={open}
            className="flex min-w-0 cursor-pointer items-center gap-2 rounded-control text-left text-sm"
          >
            {header}
          </button>
        ) : (
          <div className="flex min-w-0 items-center gap-2 text-sm">{header}</div>
        )}
        {step.summary &&
          (kind === "message" ? (
            <p className="line-clamp-2 border-l-2 border-border pl-3 text-sm whitespace-pre-line text-secondary">
              {step.summary}
            </p>
          ) : (
            <code
              title={step.summary}
              className="block truncate rounded-control bg-surface-muted px-3 py-1.5 font-mono text-xs text-secondary"
            >
              {step.summary}
            </code>
          ))}
        {open && step.output && (
          <pre
            className={cn(
              "max-h-56 overflow-auto rounded-control border border-border px-3 py-2.5 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-words text-secondary",
              failed && "border-danger/25 text-danger",
            )}
          >
            {step.output}
          </pre>
        )}
      </div>
    </>
  )
}

function ErrorStep({ text }: { text: string }) {
  return (
    <>
      <StepIcon tone="danger">
        <CircleAlertIcon size={14} strokeWidth={1.75} />
      </StepIcon>
      <p className="min-w-0 flex-1 pt-1 text-sm whitespace-pre-line break-words text-danger">{text}</p>
    </>
  )
}

function FailureItem({ message }: { message: string }) {
  const { t } = useTranslation()
  return (
    <li className="relative flex min-w-0 gap-3">
      <StepIcon tone="danger">
        <CircleAlertIcon size={14} strokeWidth={1.75} />
      </StepIcon>
      <div className="grid min-w-0 flex-1 gap-1.5 pt-1">
        <span className="text-sm font-medium text-danger">{t("execution.failedTitle")}</span>
        <p className="rounded-control border border-danger/25 bg-danger/10 px-3 py-2 text-xs whitespace-pre-line break-words text-danger">
          {message}
        </p>
      </div>
    </li>
  )
}
