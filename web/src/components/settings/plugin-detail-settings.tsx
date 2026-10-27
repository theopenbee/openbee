import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ArrowLeftIcon, CheckIcon, ExternalLinkIcon, PackageXIcon, QrCodeIcon, RotateCwIcon, Trash2Icon } from "lucide-react"
import { FadeIn } from "@/components/fade-in"
import { Panel } from "@/components/panel"
import { EmptyState } from "@/components/empty-state"
import { SkeletonCard } from "@/components/skeleton-loader"
import { DetailField } from "@/components/detail-primitives"
import { PluginSetupDialog } from "@/components/plugin-setup-dialog"
import { PluginStateBadge, UninstallPluginDialog, pendingUninstalls, usePluginOperationToasts } from "@/components/plugin-ui"
import { Alert } from "@/components/arc/alert/alert"
import { Badge } from "@/components/arc/badge/badge"
import { Button } from "@/components/arc/button/button"
import { Checkbox } from "@/components/arc/checkbox/checkbox"
import { Input } from "@/components/arc/input/input"
import { PasswordField } from "@/components/arc/password-field/password-field"
import { Textarea } from "@/components/arc/textarea/textarea"
import {
  useInstallPlugin,
  usePlugin,
  usePluginAction,
  usePluginCatalog,
  useUninstallPlugin,
  useUpdatePluginSettings,
  type PluginAction,
} from "@/hooks/use-plugins"
import { useSettingsDialog } from "@/hooks/use-settings-dialog"
import { ApiError } from "@/lib/api"
import { buildSettingsPayload, initialFormValues, invalidIntFields, localize, pendingInstalls, type FormValues } from "@/lib/plugins"
import { getErrorMessage } from "@/lib/utils"
import type { CatalogPlugin, PluginConfigField, PluginDetail as Detail, PluginSettingsUpdate } from "@/lib/types"

const SAVED_FEEDBACK_MS = 2500

export function PluginDetailSettings({ id }: { id: string }) {
  const { t, i18n } = useTranslation()
  const show = useSettingsDialog()
  const detail = usePlugin(id)
  const catalog = usePluginCatalog()
  const action = usePluginAction()
  const uninstall = useUninstallPlugin()
  const install = useInstallPlugin()
  const [confirmUninstall, setConfirmUninstall] = useState(false)
  const [uninstallError, setUninstallError] = useState("")
  const [setupOpen, setSetupOpen] = useState(false)
  const [resetKey, setResetKey] = useState(0)
  const [savedAt, setSavedAt] = useState(0)
  const operations = usePluginOperationToasts()

  useEffect(() => {
    if (!savedAt) return
    const timer = setTimeout(() => setSavedAt(0), SAVED_FEEDBACK_MS)
    return () => clearTimeout(timer)
  }, [savedAt])

  if (detail.isLoading) {
    return (
      <div className="grid gap-6" aria-busy="true">
        <SkeletonCard className="rounded-inset" />
        <SkeletonCard className="rounded-inset" />
      </div>
    )
  }
  if (detail.error || !detail.data) {
    const missing = detail.error instanceof ApiError && detail.error.status === 404
    return (
      <FadeIn>
        <BackLink />
        {missing ? (
          <div className="rounded-inset border border-border bg-surface">
            <EmptyState
              icon={<PackageXIcon size={24} strokeWidth={1.5} />}
              title={t("plugins.detail.notFound")}
              description={t("plugins.detail.notFoundHint")}
              action={<Button onClick={() => show({ section: "plugins" })}>{t("plugins.emptyInstalled.action")}</Button>}
            />
          </div>
        ) : (
          <Alert tone="danger" title={t("plugins.detail.loadError")}>
            <span className="mt-1 flex flex-wrap items-center gap-3">
              <span className="wrap-anywhere">{getErrorMessage(detail.error)}</span>
              <Button variant="secondary" size="sm" onClick={() => detail.refetch()}>
                {t("common.retry")}
              </Button>
            </span>
          </Alert>
        )}
      </FadeIn>
    )
  }

  const d = detail.data
  const lang = i18n.language
  const name = localize(d.name, lang) || d.id
  const entry = catalog.data?.plugins.find((p) => p.id === d.id)
  const setupLabel = localize(d.setup_label, lang) || t("plugins.setup.defaultLabel")
  const ops = operations.data ?? []
  const removing = Boolean(pendingUninstalls(ops)[d.id])
  const pendingKind = action.isPending ? action.variables?.action : undefined
  const showError = d.error && (d.state === "failed" || d.state === "retrying")

  const run = (kind: PluginAction) =>
    action.mutate(
      { id: d.id, action: kind },
      {
        onSuccess: () => toast.success(t(`plugins.toast.${kind === "enable" ? "enabled" : kind === "disable" ? "disabled" : "restarted"}`, { name })),
        onError: (err) => toast.error(getErrorMessage(err)),
      },
    )

  return (
    <FadeIn>
      <BackLink />
      <header className="mb-6 flex flex-wrap items-start justify-between gap-x-6 gap-y-4">
        <div className="min-w-0">
          <h3 className="text-xl font-medium text-foreground">{name}</h3>
          <p className="mt-2 flex flex-wrap items-center gap-2 text-sm text-secondary" aria-live="polite">
            <PluginStateBadge state={d.state} uninstalling={removing} />
            {entry?.official && <Badge size="sm">{t("plugins.official")}</Badge>}
            <span className="min-w-0 font-mono text-xs wrap-anywhere">{[d.package, d.id].filter(Boolean).join(" · ")}</span>
          </p>
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          {d.enabled ? (
            <Button
              variant="secondary"
              size="sm"
              loading={pendingKind === "disable"}
              disabled={removing || (action.isPending && pendingKind !== "disable")}
              onClick={() => run("disable")}
            >
              {t("plugins.action.disable")}
            </Button>
          ) : (
            <Button
              variant="secondary"
              size="sm"
              loading={pendingKind === "enable"}
              disabled={removing || d.state === "not_configured" || (action.isPending && pendingKind !== "enable")}
              onClick={() => run("enable")}
            >
              {t("plugins.action.enable")}
            </Button>
          )}
          <Button
            variant="secondary"
            size="sm"
            loading={pendingKind === "restart"}
            disabled={removing || !d.enabled || (action.isPending && pendingKind !== "restart")}
            onClick={() => run("restart")}
          >
            <RotateCwIcon size={16} strokeWidth={1.75} aria-hidden="true" />
            {t("plugins.action.restart")}
          </Button>
          <Button
            variant="danger"
            size="sm"
            disabled={removing}
            onClick={() => {
              setUninstallError("")
              setConfirmUninstall(true)
            }}
          >
            <Trash2Icon size={16} strokeWidth={1.75} aria-hidden="true" />
            {t("plugins.action.uninstall")}
          </Button>
        </div>
      </header>

      {showError && (
        <div className="mb-6">
          <Alert
            tone={d.state === "failed" ? "danger" : "warning"}
            title={d.state === "failed" ? t("plugins.detail.failedTitle") : t("plugins.detail.retryingTitle")}
          >
            <span className="mt-1 block font-mono text-xs wrap-anywhere">{d.error}</span>
            {d.state === "failed" && d.enabled && (
              <span className="mt-3 flex">
                <Button variant="secondary" size="sm" loading={pendingKind === "restart"} disabled={action.isPending} onClick={() => run("restart")}>
                  {t("plugins.detail.restartNow")}
                </Button>
              </span>
            )}
          </Alert>
        </div>
      )}

      <div className="grid gap-6 @3xl:grid-cols-[minmax(0,1fr)_20rem] @3xl:items-start">
        <div className="grid min-w-0 gap-6">
          {d.has_setup && (
            <section className="flex flex-wrap items-center justify-between gap-4 rounded-inset border border-border bg-surface px-6 py-5">
              <div className="min-w-0">
                <h2 className="text-base font-medium text-foreground">{setupLabel}</h2>
                <p className="mt-1 text-sm text-secondary">{t("plugins.setup.cardHint")}</p>
              </div>
              <Button variant="secondary" onClick={() => setSetupOpen(true)}>
                <QrCodeIcon size={16} strokeWidth={1.75} aria-hidden="true" />
                {t("plugins.setup.start")}
              </Button>
            </section>
          )}
          <Panel
            title={t("plugins.detail.settings")}
            description={t("plugins.detail.settingsHint", { name })}
            className="rounded-inset"
          >
            <SettingsForm
              key={`${resetKey}:${JSON.stringify(d.config)}:${d.bot_name}`}
              detail={d}
              name={name}
              saved={savedAt > 0}
              onSaved={() => setSavedAt(Date.now())}
              onReset={() => setResetKey((k) => k + 1)}
            />
          </Panel>
        </div>
        <InfoPanel
          detail={d}
          entry={entry}
          installing={install.isPending || Boolean(entry && pendingInstalls(ops)[entry.package])}
          onUpdate={(spec) => install.mutate(spec, { onError: (err) => toast.error(getErrorMessage(err)) })}
        />
      </div>

      {d.has_setup && (
        <PluginSetupDialog pluginId={d.id} title={setupLabel} open={setupOpen} onOpenChange={setSetupOpen} />
      )}
      <UninstallPluginDialog
        name={name}
        open={confirmUninstall}
        pending={uninstall.isPending}
        error={uninstallError}
        onOpenChange={setConfirmUninstall}
        onConfirm={() => {
          setUninstallError("")
          uninstall.mutate(d.id, {
            onSuccess: () => show({ section: "plugins", tab: "installed" }),
            onError: (err) => setUninstallError(getErrorMessage(err)),
          })
        }}
      />
    </FadeIn>
  )
}

function BackLink() {
  const { t } = useTranslation()
  const show = useSettingsDialog()
  return (
    <button
      type="button"
      onClick={() => show({ section: "plugins", tab: "installed" })}
      className="mb-4 inline-flex min-h-9 items-center gap-1.5 text-sm text-secondary transition-colors hover:text-foreground"
    >
      <ArrowLeftIcon size={16} strokeWidth={1.75} aria-hidden="true" />
      {t("plugins.detail.back")}
    </button>
  )
}

function SettingsForm({
  detail,
  name,
  saved,
  onSaved,
  onReset,
}: {
  detail: Detail
  name: string
  saved: boolean
  onSaved: () => void
  onReset: () => void
}) {
  const { t, i18n } = useTranslation()
  const lang = i18n.language
  const save = useUpdatePluginSettings(detail.id)
  const [values, setValues] = useState<FormValues>(() => initialFormValues(detail.fields, detail.config))
  const [botName, setBotName] = useState(detail.bot_name)
  const [invalid, setInvalid] = useState<string[]>([])

  const labelOf = (f: PluginConfigField) => localize(f.label, lang) || f.key
  const fieldId = (f: PluginConfigField) => `plugin-field-${f.key}`
  const set = (key: string, value: string | boolean) => setValues((v) => ({ ...v, [key]: value }))

  const submit = () => {
    const bad = invalidIntFields(detail.fields, values)
    setInvalid(bad)
    if (bad.length > 0) {
      document.getElementById(`plugin-field-${bad[0]}`)?.focus()
      return
    }
    const config = buildSettingsPayload(detail.fields, detail.config, values)
    const body: PluginSettingsUpdate = { config }
    if (botName.trim() !== detail.bot_name) body.bot_name = botName.trim()
    if (Object.keys(config).length === 0 && body.bot_name === undefined) {
      toast.info(t("plugins.detail.noChanges"))
      return
    }
    save.mutate(body, {
      onSuccess: () => {
        onSaved()
        toast.success(detail.enabled ? t("plugins.toast.savedRestarting", { name }) : t("plugins.toast.savedEnableHint"))
      },
      onError: (err) => toast.error(getErrorMessage(err)),
    })
  }

  const field = (f: PluginConfigField) => {
    const id = fieldId(f)
    const value = values[f.key]
    const label = f.required ? t("plugins.detail.requiredLabel", { label: labelOf(f) }) : labelOf(f)
    const help = localize(f.help, lang) || undefined
    const defaultHint =
      f.default !== undefined && f.default !== null && f.default !== ""
        ? t("plugins.detail.defaultValue", { value: String(f.default) })
        : undefined
    switch (f.type) {
      case "bool":
        return (
          <Checkbox
            id={id}
            label={label}
            description={help}
            checked={Boolean(value)}
            onCheckedChange={(checked) => set(f.key, checked === true)}
          />
        )
      case "list":
        return (
          <Textarea
            id={id}
            label={label}
            description={help ?? t("plugins.detail.listHelp")}
            rows={3}
            spellCheck={false}
            value={String(value ?? "")}
            placeholder={t("plugins.detail.listHelp")}
            onChange={(e) => set(f.key, e.target.value)}
          />
        )
      case "secret": {
        const mask = typeof detail.config[f.key] === "string" ? String(detail.config[f.key]) : ""
        return (
          <PasswordField
            id={id}
            label={label}
            description={help}
            autoComplete="new-password"
            value={String(value ?? "")}
            placeholder={mask ? t("plugins.detail.secretSaved", { mask }) : defaultHint}
            onChange={(e) => set(f.key, e.target.value)}
          />
        )
      }
      default:
        return (
          <Input
            id={id}
            label={label}
            description={help}
            inputMode={f.type === "int" ? "numeric" : undefined}
            spellCheck={false}
            error={invalid.includes(f.key) ? t("plugins.detail.intInvalid", { label: labelOf(f) }) : undefined}
            value={String(value ?? "")}
            placeholder={defaultHint}
            onChange={(e) => {
              set(f.key, e.target.value)
              if (invalid.includes(f.key)) setInvalid((prev) => prev.filter((k) => k !== f.key))
            }}
          />
        )
    }
  }

  return (
    <form
      className="grid gap-6"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      {detail.fields.map((f) => (
        <div key={f.key} className={f.type === "bool" ? "-ml-3 min-w-0" : "min-w-0"}>
          {field(f)}
        </div>
      ))}
      <Input
        id="plugin-bot-name"
        label={t("plugins.detail.botName")}
        description={t("plugins.detail.botNameHelp")}
        value={botName}
        onChange={(e) => setBotName(e.target.value)}
      />
      <div className="flex flex-wrap items-center justify-end gap-2 border-t border-border-subtle pt-5">
        <Button type="button" variant="ghost" onClick={onReset} disabled={save.isPending}>
          {t("plugins.detail.reset")}
        </Button>
        <Button type="submit" loading={save.isPending}>
          {saved ? (
            <>
              <CheckIcon size={16} strokeWidth={1.75} aria-hidden="true" />
              {t("plugins.detail.saved")}
            </>
          ) : (
            t("plugins.detail.save")
          )}
        </Button>
      </div>
    </form>
  )
}

function InfoPanel({
  detail,
  entry,
  installing,
  onUpdate,
}: {
  detail: Detail
  entry?: CatalogPlugin
  installing: boolean
  onUpdate: (spec: string) => void
}) {
  const { t } = useTranslation()
  const sourceLabel =
    detail.source === "npm" ? t("plugins.detail.sourceNpm") : detail.source === "path" ? t("plugins.detail.sourcePath") : detail.source
  const repo = entry?.repository
  return (
    <Panel title={t("plugins.detail.info")} className="rounded-inset">
      <div className="grid gap-5">
        {detail.package && <DetailField label={t("plugins.detail.package")} value={detail.package} mono />}
        <DetailField
          label={t("plugins.detail.version")}
          value={
            <div className="grid justify-items-start gap-3">
              <div>
                <span className="font-mono tabular">{detail.version || "–"}</span>
                {entry && detail.source === "npm" && (
                  <span className="ml-2 text-xs text-secondary">
                    {entry.update_available ? t("plugins.detail.newer", { version: entry.version }) : t("plugins.detail.latest")}
                  </span>
                )}
              </div>
              {entry?.update_available && entry.installable && (
                <Button size="sm" variant="secondary" loading={installing} onClick={() => onUpdate(`${entry.package}@${entry.version}`)}>
                  {t("plugins.action.update", { version: entry.version })}
                </Button>
              )}
            </div>
          }
        />
        <DetailField label={t("plugins.detail.source")} value={sourceLabel} />
        {detail.source === "path" && <DetailField label={t("plugins.detail.path")} value={detail.path} mono />}
        {repo && (
          <DetailField
            label={t("plugins.detail.repository")}
            value={
              <a href={repo} target="_blank" rel="noreferrer" className="text-accent-strong wrap-anywhere hover:underline">
                {repo.replace(/^https?:\/\//, "")}
                <ExternalLinkIcon size={14} strokeWidth={1.75} className="ml-1 inline align-text-top" aria-hidden="true" />
              </a>
            }
          />
        )}
        {entry?.license && <DetailField label={t("plugins.detail.license")} value={entry.license} />}
      </div>
    </Panel>
  )
}
