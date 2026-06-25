import { useState } from "react"
import { useTranslation } from "react-i18next"
import {
  Check,
  ClipboardCheck,
  MessageSquareText,
  Pencil,
  Plus,
  ScrollText,
  Shield,
  X,
  type LucideIcon,
} from "lucide-react"
import { DetailSection } from "@/components/detail-primitives"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { CopyButton } from "@/components/copy-button"
import { useUpdateWorker } from "@/hooks/use-workers"
import type { Worker } from "@/lib/types"

// Starter templates shown on the empty state. Clicking one opens the editor
// pre-filled with the body string, so the operator never faces a blank page.
const TEMPLATES: { id: string; icon: LucideIcon }[] = [
  { id: "tone", icon: MessageSquareText },
  { id: "scope", icon: Shield },
  { id: "delivery", icon: ClipboardCheck },
]

export function WorkerConstraintsPanel({ worker }: { worker: Worker }) {
  const { t } = useTranslation()
  const updateWorker = useUpdateWorker()
  const [isEditing, setIsEditing] = useState(false)
  const [draft, setDraft] = useState("")

  const constraints = worker.constraints?.trim() ? worker.constraints : ""

  function startEditing(initial: string) {
    setDraft(initial)
    setIsEditing(true)
  }

  function cancelEditing() {
    setIsEditing(false)
    setDraft("")
  }

  async function save() {
    await updateWorker.mutateAsync({ id: worker.id, data: { constraints: draft } })
    setIsEditing(false)
  }

  // ---- Edit state ----
  if (isEditing) {
    return (
      <DetailSection className="p-5 sm:p-6">
        <div className="flex flex-col gap-4">
          <div className="flex items-center gap-2.5">
            <span className="flex size-7 items-center justify-center rounded-sm bg-muted text-muted-foreground">
              <ScrollText className="size-4" aria-hidden="true" />
            </span>
            <div className="min-w-0">
              <p className="text-sm font-medium text-foreground">{t("workerDetail.editConstraints")}</p>
            </div>
          </div>

          <Textarea
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Escape") cancelEditing()
              if ((event.metaKey || event.ctrlKey) && event.key === "Enter") void save()
            }}
            autoFocus
            rows={14}
            placeholder={t("workers.form.constraintsPlaceholder")}
            className="text-sm leading-7"
          />

          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-xs text-muted-foreground">
              {t("workerDetail.constraintsPanel.charCount", { count: draft.length })}
              <span aria-hidden="true" className="mx-2 text-border">·</span>
              {t("workerDetail.constraintsPanel.shortcuts")}
            </p>
            <div className="flex gap-2">
              <Button size="sm" onClick={() => void save()} disabled={updateWorker.isPending}>
                <Check className="size-4" />
                {t("common.save")}
              </Button>
              <Button size="sm" variant="outline" onClick={cancelEditing}>
                <X className="size-4" />
                {t("common.cancel")}
              </Button>
            </div>
          </div>
        </div>
      </DetailSection>
    )
  }

  // ---- View state (configured) ----
  if (constraints) {
    return (
      <DetailSection className="px-5 py-5 sm:px-6">
        <div className="flex items-start justify-between gap-4">
          <p className="min-w-0 max-w-[70ch] whitespace-pre-wrap break-words text-sm leading-7 text-foreground">
            {constraints}
          </p>
          <div className="flex shrink-0 items-center gap-1">
            <CopyButton value={constraints} />
            <Button size="sm" variant="ghost" onClick={() => startEditing(worker.constraints || "")}>
              <Pencil className="size-4" />
              {t("workerDetail.editConstraints")}
            </Button>
          </div>
        </div>
      </DetailSection>
    )
  }

  // ---- Empty state ----
  return (
    <DetailSection className="px-5 py-12 sm:px-6">
      <div className="mx-auto flex max-w-md flex-col items-center text-center">
        <span className="flex size-12 items-center justify-center rounded-sm bg-muted text-muted-foreground">
          <ScrollText className="size-6" aria-hidden="true" />
        </span>
        <h2 className="mt-4 text-base font-medium text-foreground">
          {t("workerDetail.constraintsPanel.emptyTitle", { name: worker.name })}
        </h2>
        <p className="mt-1.5 text-sm leading-6 text-muted-foreground">
          {t("workerDetail.constraintsPanel.emptyDescription")}
        </p>
        <Button className="mt-5" onClick={() => startEditing("")}>
          <Plus className="size-4" />
          {t("workerDetail.constraintsPanel.add")}
        </Button>
      </div>

      <div className="mx-auto mt-9 max-w-md">
        <p className="text-[0.6875rem] font-medium uppercase tracking-[0.08em] text-muted-foreground/80">
          {t("workerDetail.constraintsPanel.templatesLabel")}
        </p>
        <div className="mt-2.5 flex flex-col gap-2">
          {TEMPLATES.map(({ id, icon: Icon }) => (
            <button
              key={id}
              type="button"
              onClick={() => startEditing(t(`workerDetail.constraintsPanel.templates.${id}.body`))}
              className="group flex items-start gap-3 rounded-sm border border-border/70 bg-background/60 px-3.5 py-3 text-left transition-colors hover:border-border hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
            >
              <span className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-sm bg-muted text-muted-foreground transition-colors group-hover:bg-background">
                <Icon className="size-4" aria-hidden="true" />
              </span>
              <span className="min-w-0">
                <span className="block text-sm font-medium text-foreground">
                  {t(`workerDetail.constraintsPanel.templates.${id}.title`)}
                </span>
                <span className="block text-xs leading-5 text-muted-foreground">
                  {t(`workerDetail.constraintsPanel.templates.${id}.desc`)}
                </span>
              </span>
            </button>
          ))}
        </div>
      </div>
    </DetailSection>
  )
}
