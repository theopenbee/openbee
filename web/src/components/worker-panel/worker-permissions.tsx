import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { Alert } from "@/components/arc/alert/alert"
import { DetailSection } from "@/components/detail-primitives"
import { ScopeToggleCard } from "@/components/scope-toggle-card"
import { useUpdateWorker } from "@/hooks/use-workers"
import { KNOWN_SCOPES, parseScopes, serializeScopes, toggleScope } from "@/lib/scopes"
import type { Worker } from "@/lib/types"
import { getErrorMessage } from "@/lib/utils"

export function WorkerPermissions({ worker }: { worker: Worker }) {
  const { t } = useTranslation()
  const updateWorker = useUpdateWorker()
  const [localScopes, setLocalScopes] = useState<string[]>(() => parseScopes(worker.permission_scopes ?? ""))
  const [scopeError, setScopeError] = useState("")

  useEffect(() => {
    setLocalScopes(parseScopes(worker.permission_scopes ?? ""))
  }, [worker.permission_scopes])

  const toggleScopeValue = (scopeId: string, val: boolean) => {
    const prevScopes = localScopes
    const newScopes = toggleScope(localScopes, scopeId, val)
    setLocalScopes(newScopes)
    setScopeError("")
    updateWorker.mutate(
      { id: worker.id, data: { permission_scopes: serializeScopes(newScopes) } },
      {
        onError: (err) => {
          setLocalScopes(prevScopes)
          setScopeError(getErrorMessage(err))
        },
      },
    )
  }

  return (
    <section className="grid gap-4" aria-label={t("workerDetail.permissions")}>
      <p className="text-sm text-secondary">{t("workers.form.permissionsHelper")}</p>
      {scopeError && (
        <Alert tone="danger" title={t("workerDetail.permissionsSaveError")}>
          {scopeError}
        </Alert>
      )}
      <DetailSection className="divide-y divide-border-subtle">
        {KNOWN_SCOPES.map((scope) => (
          <ScopeToggleCard
            key={scope.id}
            scope={scope}
            checked={localScopes.includes(scope.id)}
            onToggle={toggleScopeValue}
            disabled={updateWorker.isPending}
          />
        ))}
      </DetailSection>
    </section>
  )
}
