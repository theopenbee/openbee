import { useState } from "react"
import { useTranslation } from "react-i18next"
import {
  ChevronRight,
  ClipboardCheck,
  MessageSquareText,
  Pencil,
  Plus,
  ScrollText,
  Shield,
  type LucideIcon,
} from "lucide-react"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Textarea } from "@/components/arc/textarea/textarea"
import { DetailSection } from "@/components/detail-primitives"
import { CopyButton } from "@/components/copy-button"
import { EmptyState } from "@/components/empty-state"
import { useUpdateWorker } from "@/hooks/use-workers"
import { getErrorMessage } from "@/lib/utils"
import type { Worker } from "@/lib/types"

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
  const [saveError, setSaveError] = useState("")

  const constraints = worker.constraints?.trim() ? worker.constraints : ""

  function startEditing(initial: string) {
    setDraft(initial)
    setSaveError("")
    setIsEditing(true)
  }

  function cancelEditing() {
    setIsEditing(false)
    setDraft("")
    setSaveError("")
  }

  async function save() {
    if (updateWorker.isPending) return
    setSaveError("")
    try {
      await updateWorker.mutateAsync({ id: worker.id, data: { constraints: draft } })
      setIsEditing(false)
    } catch (err) {
      setSaveError(getErrorMessage(err))
    }
  }

  if (isEditing) {
    return (
      <DetailSection className="p-6">
        <form
          className="grid gap-4"
          onSubmit={(event) => {
            event.preventDefault()
            void save()
          }}
        >
          <Textarea
            label={t("workerDetail.editConstraints")}
            description={`${t("workerDetail.constraintsPanel.charCount", { count: draft.length })} · ${t("workerDetail.constraintsPanel.shortcuts")}`}
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Escape") cancelEditing()
              if ((event.metaKey || event.ctrlKey) && event.key === "Enter") void save()
            }}
            autoFocus
            rows={14}
            placeholder={t("workers.form.constraintsPlaceholder")}
            disabled={updateWorker.isPending}
          />

          {saveError && (
            <Alert tone="danger" title={t("workerDetail.constraintsPanel.saveError")}>
              {saveError}
            </Alert>
          )}

          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={cancelEditing} disabled={updateWorker.isPending}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" loading={updateWorker.isPending}>
              {t("workerDetail.constraintsPanel.save")}
            </Button>
          </div>
        </form>
      </DetailSection>
    )
  }

  if (constraints) {
    return (
      <DetailSection>
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border-subtle px-6 py-4">
          <p className="text-sm text-secondary tabular">
            {t("workerDetail.constraintsPanel.charCount", { count: constraints.length })}
          </p>
          <div className="ml-auto flex items-center gap-2">
            <CopyButton value={constraints} />
            <Button size="sm" variant="secondary" onClick={() => startEditing(constraints)}>
              <Pencil size={16} strokeWidth={1.75} aria-hidden="true" />
              {t("workerDetail.editConstraints")}
            </Button>
          </div>
        </div>
        <p className="max-w-prose whitespace-pre-wrap break-words px-6 py-5 text-sm leading-7 text-foreground">
          {constraints}
        </p>
      </DetailSection>
    )
  }

  return (
    <div className="grid gap-6">
      <DetailSection>
        <EmptyState
          icon={<ScrollText size={20} strokeWidth={1.75} aria-hidden="true" />}
          title={t("workerDetail.constraintsPanel.emptyTitle", { name: worker.name })}
          description={t("workerDetail.constraintsPanel.emptyDescription")}
          action={
            <Button onClick={() => startEditing("")}>
              <Plus size={16} strokeWidth={1.75} aria-hidden="true" />
              {t("workerDetail.constraintsPanel.add")}
            </Button>
          }
        />
      </DetailSection>

      <section className="grid gap-3" aria-labelledby="constraint-templates">
        <h2 id="constraint-templates" className="text-base font-medium text-foreground">
          {t("workerDetail.constraintsPanel.templatesLabel")}
        </h2>
        <ul className="overflow-hidden rounded-surface border border-border bg-surface">
          {TEMPLATES.map(({ id, icon: Icon }) => (
            <li key={id} className="border-t border-border-subtle first:border-t-0">
              <button
                type="button"
                onClick={() => startEditing(t(`workerDetail.constraintsPanel.templates.${id}.body`))}
                className="flex min-h-16 w-full items-center gap-4 px-6 py-4 text-left transition-colors hover:bg-surface-muted"
              >
                <Icon size={20} strokeWidth={1.75} aria-hidden="true" className="shrink-0 text-secondary" />
                <span className="grid min-w-0 flex-1 gap-0.5">
                  <span className="text-sm font-medium text-foreground">
                    {t(`workerDetail.constraintsPanel.templates.${id}.title`)}
                  </span>
                  <span className="text-sm text-secondary">
                    {t(`workerDetail.constraintsPanel.templates.${id}.desc`)}
                  </span>
                </span>
                <ChevronRight size={16} strokeWidth={1.75} aria-hidden="true" className="shrink-0 text-muted" />
              </button>
            </li>
          ))}
        </ul>
      </section>
    </div>
  )
}
