import { useMemo, useState } from "react"
import { Navigate, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { CheckIcon, DownloadIcon } from "lucide-react"
import { AuthLayout } from "@/components/auth-layout"
import { OperationProgress, PluginAvatar, useNow, usePluginOperationToasts } from "@/components/plugin-ui"
import { SkeletonCard } from "@/components/skeleton-loader"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { useFinishOnboarding, useOnboarding, useSkipOnboarding } from "@/hooks/use-onboarding"
import { useInstallPlugin, usePluginCatalog } from "@/hooks/use-plugins"
import { useOpenSettings } from "@/hooks/use-settings"
import { CHAT_PATH } from "@/lib/routes"
import { localize, marketAction, pendingInstalls } from "@/lib/plugins"
import { cn, getErrorMessage } from "@/lib/utils"
import type { CatalogPlugin, PluginOperation } from "@/lib/types"
import styles from "./welcome.module.css"

type Category = "engine" | "channel"

export function Welcome() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const show = useOpenSettings()
  const onboarding = useOnboarding()
  const finish = useFinishOnboarding()
  const skip = useSkipOnboarding()
  const catalog = usePluginCatalog()
  const [step, setStep] = useState<Category>("engine")

  if (onboarding.isLoading) return null
  if (step === "engine" && !onboarding.data?.pending) return <Navigate to={CHAT_PATH} replace />

  const hasEngine = Boolean(catalog.data?.plugins.some((p) => p.category === "engine" && p.installed))

  const leave = () => {
    finish()
    navigate(CHAT_PATH, { replace: true })
  }

  if (step === "engine") {
    return (
      <AuthLayout width="md" title={t("welcome.engine.title")} description={t("welcome.engine.description")}>
        <PluginChoices category="engine" />
        <div className="mt-6 flex items-center gap-3">
          <Button
            variant="ghost"
            loading={skip.isPending}
            onClick={() => skip.mutate(undefined, { onError: (err) => toast.error(getErrorMessage(err)) })}
          >
            {t("welcome.skip")}
          </Button>
          <span className="min-w-0 flex-1 text-xs text-muted">{t("welcome.engine.skipHint")}</span>
          <Button disabled={!hasEngine} onClick={() => setStep("channel")}>
            {t("welcome.next")}
          </Button>
        </div>
      </AuthLayout>
    )
  }

  return (
    <AuthLayout width="lg" title={t("welcome.channel.title")} description={t("welcome.channel.description")}>
      <PluginChoices
        category="channel"
        onConfigure={(id) => {
          finish()
          show({ section: "plugins", plugin: id }, { replace: true })
        }}
      />
      <div className="mt-6 flex items-center justify-between gap-3">
        <Button variant="ghost" onClick={leave}>
          {t("welcome.skip")}
        </Button>
        <Button onClick={leave}>{t("welcome.done")}</Button>
      </div>
    </AuthLayout>
  )
}

function PluginChoices({ category, onConfigure }: { category: Category; onConfigure?: (id: string) => void }) {
  const { t, i18n } = useTranslation()
  const catalog = usePluginCatalog()
  const operations = usePluginOperationToasts()
  const install = useInstallPlugin()
  const [requested, setRequested] = useState<Set<string>>(() => new Set())
  const pending = useMemo(() => pendingInstalls(operations.data ?? []), [operations.data])
  const now = useNow(Object.keys(pending).length > 0 || requested.size > 0)
  const listClass = cn("grid gap-3", category === "channel" && "sm:grid-cols-2")

  if (catalog.isLoading) {
    return (
      <div className={listClass} aria-busy="true">
        {Array.from({ length: 4 }, (_, i) => <SkeletonCard key={i} className={cn(styles.choice, "p-4")} />)}
      </div>
    )
  }
  if (catalog.error || !catalog.data) {
    return (
      <Alert tone="danger" title={t("plugins.catalogError")}>
        <span className="mt-1 flex flex-wrap items-center gap-3">
          <span className="wrap-anywhere">{getErrorMessage(catalog.error)}</span>
          <Button variant="secondary" size="sm" onClick={() => catalog.refetch()}>
            {t("common.retry")}
          </Button>
        </span>
      </Alert>
    )
  }

  const startInstall = (p: CatalogPlugin) => {
    setRequested((prev) => new Set(prev).add(p.package))
    install.mutate(`${p.package}@${p.version}`, {
      onError: (err) => toast.error(getErrorMessage(err)),
      onSettled: () =>
        setRequested((prev) => {
          const next = new Set(prev)
          next.delete(p.package)
          return next
        }),
    })
  }

  return (
    <ul className={listClass}>
      {catalog.data.plugins
        .filter((p) => p.category === category)
        .map((p) => {
          const name = localize(p.name, i18n.language) || p.id
          const description = localize(p.description, i18n.language)
          const op = pending[p.package]
          return (
            <li key={p.id} className={cn(styles.choice, "flex min-w-0 items-center gap-3 border border-border bg-surface px-4 py-3")}>
              <PluginAvatar name={name} />
              <div className="min-w-0 flex-1">
                <h3 className="truncate text-sm font-medium text-foreground">{name}</h3>
                {description && <p className="mt-0.5 truncate text-xs text-secondary">{description}</p>}
              </div>
              <div className="shrink-0">
                <ChoiceAction
                  action={marketAction(p, Boolean(op) || requested.has(p.package))}
                  op={op}
                  now={now}
                  onInstall={() => startInstall(p)}
                  onConfigure={onConfigure && (() => onConfigure(p.id))}
                />
              </div>
            </li>
          )
        })}
    </ul>
  )
}

function ChoiceAction({
  action,
  op,
  now,
  onInstall,
  onConfigure,
}: {
  action: ReturnType<typeof marketAction>
  op?: PluginOperation
  now: number
  onInstall: () => void
  onConfigure?: () => void
}) {
  const { t } = useTranslation()
  switch (action) {
    case "installing":
      return <OperationProgress label={t("plugins.action.installingShort")} startedAt={op?.created_at} now={now} />
    case "incompatible":
      return <span className="text-xs text-secondary">{t("plugins.action.incompatible")}</span>
    case "install":
      return (
        <Button size="sm" variant="secondary" onClick={onInstall}>
          <DownloadIcon size={16} strokeWidth={1.75} aria-hidden="true" />
          {t("plugins.action.install")}
        </Button>
      )
    default:
      return onConfigure ? (
        <Button size="sm" variant="secondary" onClick={onConfigure}>
          {t("plugins.action.configure")}
        </Button>
      ) : (
        <span className="inline-flex min-h-9 items-center gap-1.5 text-sm text-secondary">
          <CheckIcon size={16} strokeWidth={1.75} className="shrink-0 text-success" aria-hidden="true" />
          {t("welcome.installed")}
        </span>
      )
  }
}
