import { useState } from "react"
import { useTranslation } from "react-i18next"
import {
  Check,
  ChevronRight,
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
import { Panel } from "@/components/panel"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { CopyButton } from "@/components/copy-button"
import { useUpdateWorker } from "@/hooks/use-workers"
import { useCan } from "@/hooks/use-can"
import { Perm } from "@/lib/permissions"
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
  const canWrite = useCan(Perm.ContactsWrite)
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
      <Panel title={t("workerDetail.editConstraints")}>
        <div className="flex flex-col gap-3">
          <Textarea
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Escape") cancelEditing()
              if ((event.metaKey || event.ctrlKey) && event.key === "Enter") void save()
            }}
            autoFocus
            rows={14}
            aria-label={t("workerDetail.editConstraints")}
            placeholder={t("workers.form.constraintsPlaceholder")}
            className="min-h-72 text-base leading-6 md:text-sm"
          />

          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-xs text-muted-foreground tabular-nums">
              {t("workerDetail.constraintsPanel.charCount", { count: draft.length })}
              <span aria-hidden="true" className="mx-2">·</span>
              {t("workerDetail.constraintsPanel.shortcuts")}
            </p>
            <div className="flex gap-2">
              <Button variant="outline" onClick={cancelEditing}>
                <X className="size-4" />
                {t("common.cancel")}
              </Button>
              <Button onClick={() => void save()} disabled={updateWorker.isPending}>
                <Check className="size-4" />
                {t("common.save")}
              </Button>
            </div>
          </div>
        </div>
      </Panel>
    )
  }

  // ---- View state (configured) ----
  // The section header already names the content, so the text sits on a plain
  // surface with its actions top-right instead of under a repeated title strip.
  if (constraints) {
    return (
      <DetailSection className="p-4">
        <div className="flex items-start justify-between gap-4">
          <p className="min-w-0 max-w-[72ch] text-sm leading-6 break-words whitespace-pre-wrap text-foreground">
            {constraints}
          </p>
          <div className="flex shrink-0 items-center gap-1">
            <CopyButton value={constraints} className="flex size-7 items-center justify-center" />
            {canWrite && (
              <Button size="sm" variant="outline" onClick={() => startEditing(constraints)}>
                <Pencil className="size-3.5" />
                {t("workerDetail.editConstraints")}
              </Button>
            )}
          </div>
        </div>
      </DetailSection>
    )
  }

  // ---- Empty state (read-only) ----
  // Without contacts:write the operator cannot add constraints, so drop the
  // editing affordances (add button + templates) and show a plain notice.
  if (!canWrite) {
    return (
      <DetailSection className="px-6 py-12">
        <div className="mx-auto flex max-w-md flex-col items-center text-center">
          <span className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <ScrollText className="size-5" aria-hidden="true" />
          </span>
          <p className="mt-3 text-sm leading-6 text-muted-foreground">
            {t("workerDetail.constraintsPanel.emptyReadonly")}
          </p>
        </div>
      </DetailSection>
    )
  }

  // ---- Empty state ----
  // The call to action sits on its own surface; the starter templates follow
  // as a layered panel whose rows are plain dividers, not nested cards.
  return (
    <div className="space-y-6">
      <DetailSection className="px-6 py-10">
        <div className="mx-auto flex max-w-md flex-col items-center text-center">
          <span className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <ScrollText className="size-5" aria-hidden="true" />
          </span>
          <h2 className="mt-3 text-base font-semibold text-strong">
            {t("workerDetail.constraintsPanel.emptyTitle", { name: worker.name })}
          </h2>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">
            {t("workerDetail.constraintsPanel.emptyDescription")}
          </p>
          <Button className="mt-4" onClick={() => startEditing("")}>
            <Plus className="size-4" />
            {t("workerDetail.constraintsPanel.add")}
          </Button>
        </div>
      </DetailSection>

      <Panel title={t("workerDetail.constraintsPanel.templatesLabel")} flush>
        <ul className="divide-y divide-hairline">
          {TEMPLATES.map(({ id, icon: Icon }) => (
            <li key={id}>
              <button
                type="button"
                onClick={() => startEditing(t(`workerDetail.constraintsPanel.templates.${id}.body`))}
                className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-elevated focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none focus-visible:ring-inset"
              >
                <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
                  <Icon className="size-4" aria-hidden="true" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block text-sm font-medium text-strong">
                    {t(`workerDetail.constraintsPanel.templates.${id}.title`)}
                  </span>
                  <span className="block text-body-sm leading-5 text-muted-foreground">
                    {t(`workerDetail.constraintsPanel.templates.${id}.desc`)}
                  </span>
                </span>
                <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
              </button>
            </li>
          ))}
        </ul>
      </Panel>
    </div>
  )
}
