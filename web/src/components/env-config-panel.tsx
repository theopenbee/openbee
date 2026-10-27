import { useId, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { KeyRoundIcon, PencilIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { useEnvList } from "@/hooks/use-envs"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Tooltip } from "@/components/arc/tooltip/tooltip"
import { CopyButton } from "@/components/copy-button"
import { EmptyState } from "@/components/empty-state"
import { EnvAddDialog, EnvDeleteDialog, EnvEditDialog } from "@/components/env-variable-dialogs"
import { formatRelative } from "@/lib/format"
import { cn, getErrorMessage } from "@/lib/utils"
import type { EnvConfig, EnvScope } from "@/lib/types"

const ICON = { size: 16, strokeWidth: 1.75, "aria-hidden": true } as const

const EMPTY_DESCRIPTION: Record<EnvScope, [string, string]> = {
  global: ["envConfig.emptyGlobal", "Add a variable to pass it to the Bee and every worker."],
  bee: ["envConfig.emptyBee", "Add a variable to pass it to the Bee only."],
  department: ["envConfig.emptyDepartment", "Add a variable to pass it to every worker in this department."],
  worker: ["envConfig.emptyWorker", "Add a variable to pass it to this worker only."],
}

interface EnvConfigPanelProps {
  scope: EnvScope
  scopeId?: string
  title?: string
  description?: string
  inset?: boolean
}

export function EnvConfigPanel({ scope, scopeId, title, description, inset = false }: EnvConfigPanelProps) {
  const { t } = useTranslation()
  const headingId = useId()
  const { data: envs = [], isLoading, isError, error, refetch, isRefetching } = useEnvList(scope, scopeId)
  const [addOpen, setAddOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<EnvConfig | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<EnvConfig | null>(null)
  const existingKeys = useMemo(() => envs.map((e) => e.key), [envs])
  const isEmpty = !isLoading && !isError && envs.length === 0
  const [emptyKey, emptyFallback] = EMPTY_DESCRIPTION[scope]

  const addButton = (
    <Button variant="secondary" size="sm" onClick={() => setAddOpen(true)}>
      <PlusIcon {...ICON} />
      {t("envConfig.add", "Add variable")}
    </Button>
  )

  return (
    <section className="grid min-w-0 gap-4" aria-labelledby={title ? headingId : undefined}>
      {title || description || !isEmpty ? (
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0 flex-1">
            {title ? (
              <h2 id={headingId} className="text-base font-medium text-foreground">
                {title}
              </h2>
            ) : null}
            {description ? <p className="mt-1 text-sm text-secondary">{description}</p> : null}
          </div>
          {!isEmpty && !isError ? <div className="shrink-0">{addButton}</div> : null}
        </div>
      ) : null}

      {isError ? (
        <Alert tone="danger" title={t("envConfig.loadError", "Could not load variables")}>
          <span className="block">{getErrorMessage(error)}</span>
          <span className="mt-3 block">
            <Button variant="secondary" size="sm" loading={isRefetching} onClick={() => void refetch()}>
              {t("common.retry", "Retry")}
            </Button>
          </span>
        </Alert>
      ) : (
        <div
          className={cn(
            "@container min-w-0 overflow-hidden border border-border bg-surface",
            inset ? "rounded-inset" : "rounded-surface",
          )}
          aria-busy={isLoading || undefined}
        >
          {isLoading ? (
            <EnvRowsSkeleton />
          ) : isEmpty ? (
            <EmptyState
              icon={<KeyRoundIcon size={24} strokeWidth={1.75} aria-hidden="true" />}
              title={t("envConfig.empty", "No variables yet")}
              description={t(emptyKey, emptyFallback)}
              action={addButton}
            />
          ) : (
            <ul className="divide-y divide-border-subtle">
              {envs.map((env) => (
                <EnvRow
                  key={env.id}
                  env={env}
                  onEdit={() => setEditTarget(env)}
                  onDelete={() => setDeleteTarget(env)}
                />
              ))}
            </ul>
          )}
        </div>
      )}

      <EnvAddDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        scope={scope}
        scopeId={scopeId}
        existingKeys={existingKeys}
      />
      <EnvEditDialog target={editTarget} onClose={() => setEditTarget(null)} scope={scope} scopeId={scopeId} />
      <EnvDeleteDialog target={deleteTarget} onClose={() => setDeleteTarget(null)} scope={scope} scopeId={scopeId} />
    </section>
  )
}

function EnvRow({ env, onEdit, onDelete }: { env: EnvConfig; onEdit: () => void; onDelete: () => void }) {
  const { t } = useTranslation()
  const updated = formatRelative(env.updated_at, t)

  return (
    <li className="flex min-h-16 items-center gap-3 py-3 pr-4 pl-5 sm:pl-6">
      <div className="min-w-0 flex-1">
        <p className="truncate font-mono text-sm text-foreground" title={env.key}>
          {env.key}
        </p>
        <p className="mt-1 flex min-w-0 items-center gap-2 text-xs">
          <span className="min-w-0 truncate font-mono text-secondary">
            <span className="sr-only">{t("envConfig.masked", "Value")}: </span>
            {env.masked}
          </span>
          <span aria-hidden="true" className="text-muted @max-md:hidden">·</span>
          <span className="shrink-0 text-muted @max-md:hidden">
            {t("envConfig.updated", { time: updated, defaultValue: "Updated {{time}}" })}
          </span>
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        <CopyButton value={env.key} label={t("envConfig.copyKey", { key: env.key, defaultValue: "Copy {{key}}" })} />
        <Tooltip content={t("envConfig.editAction", "Edit value")}>
          <Button
            variant="ghost"
            size="sm"
            aria-label={t("envConfig.editLabel", { key: env.key, defaultValue: "Edit {{key}}" })}
            onClick={onEdit}
          >
            <PencilIcon {...ICON} />
          </Button>
        </Tooltip>
        <Tooltip content={t("envConfig.deleteAction", "Delete variable")}>
          <Button
            variant="ghost"
            size="sm"
            aria-label={t("envConfig.deleteLabel", { key: env.key, defaultValue: "Delete {{key}}" })}
            onClick={onDelete}
          >
            <Trash2Icon {...ICON} />
          </Button>
        </Tooltip>
      </div>
    </li>
  )
}

function EnvRowsSkeleton() {
  return (
    <ul className="divide-y divide-border-subtle" aria-hidden="true">
      {[0, 1, 2].map((i) => (
        <li key={i} className="flex min-h-16 items-center gap-3 py-3 pr-4 pl-5 sm:pl-6">
          <div className="grid min-w-0 flex-1 gap-2">
            <div className="skeleton h-4 w-40 max-w-full" />
            <div className="skeleton h-3 w-56 max-w-full" />
          </div>
          <div className="skeleton h-8 w-24 shrink-0" />
        </li>
      ))}
    </ul>
  )
}
