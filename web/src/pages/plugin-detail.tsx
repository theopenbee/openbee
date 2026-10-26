import { useState, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { Link, useNavigate, useParams } from "react-router-dom"
import { toast } from "sonner"
import { ArrowLeftIcon, ExternalLinkIcon, QrCodeIcon } from "lucide-react"
import { FadeIn } from "@/components/fade-in"
import { Panel } from "@/components/panel"
import { EmptyState } from "@/components/empty-state"
import { SkeletonPage } from "@/components/skeleton-loader"
import { DetailField, DetailHero } from "@/components/detail-primitives"
import { PluginSetupDialog } from "@/components/plugin-setup-dialog"
import { PluginStateLabel, PluginTile, UninstallPluginDialog, usePluginOperationToasts } from "@/components/plugin-ui"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import {
  useInstallPlugin,
  usePlugin,
  usePluginAction,
  usePluginCatalog,
  useUninstallPlugin,
  useUpdatePluginSettings,
  type PluginAction,
} from "@/hooks/use-plugins"
import { ApiError } from "@/lib/api"
import { buildSettingsPayload, initialFormValues, invalidIntFields, localize, pendingInstalls, type FormValues } from "@/lib/plugins"
import { ALERT_DESTRUCTIVE } from "@/lib/styles"
import { cn, getErrorMessage } from "@/lib/utils"
import type { CatalogPlugin, PluginConfigField, PluginDetail as Detail, PluginSettingsUpdate } from "@/lib/types"

export function PluginDetail() {
  const { id = "" } = useParams()
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const detail = usePlugin(id)
  const catalog = usePluginCatalog()
  const action = usePluginAction()
  const uninstall = useUninstallPlugin()
  const install = useInstallPlugin()
  const [confirmUninstall, setConfirmUninstall] = useState(false)
  const [setupOpen, setSetupOpen] = useState(false)
  const [resetKey, setResetKey] = useState(0)
  const operations = usePluginOperationToasts()

  if (detail.isLoading) return <SkeletonPage />
  if (detail.error || !detail.data) {
    const missing = detail.error instanceof ApiError && detail.error.status === 404
    return (
      <div className="mx-auto w-full max-w-6xl">
        <BackLink />
        {missing ? (
          <EmptyState title={t("plugins.detail.notFound")} />
        ) : (
          <div className={ALERT_DESTRUCTIVE}>{getErrorMessage(detail.error)}</div>
        )}
      </div>
    )
  }

  const d = detail.data
  const lang = i18n.language
  const name = localize(d.name, lang) || d.id
  const entry = catalog.data?.plugins.find((p) => p.id === d.id)
  const setupLabel = localize(d.setup_label, lang) || t("plugins.setup.defaultLabel")

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
      <div className="mx-auto w-full max-w-6xl">
        <BackLink />
        <DetailHero className="mb-5 p-5">
          <div className="flex flex-wrap items-center gap-4">
            <PluginTile name={name} className="size-11" />
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="text-xl font-bold tracking-tight">{name}</h1>
                {entry?.official && <Badge variant="outline">{t("plugins.official")}</Badge>}
                <PluginStateLabel state={d.state} />
              </div>
              <div className="mt-1 truncate font-mono text-xs text-muted-foreground">
                {[d.package, d.id].filter(Boolean).join(" · ")}
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              {d.enabled ? (
                <Button variant="outline" size="sm" disabled={action.isPending} onClick={() => run("disable")}>
                  {t("plugins.action.disable")}
                </Button>
              ) : (
                <Button size="sm" disabled={action.isPending || d.state === "not_configured"} onClick={() => run("enable")}>
                  {t("plugins.action.enable")}
                </Button>
              )}
              <Button variant="outline" size="sm" disabled={action.isPending || !d.enabled} onClick={() => run("restart")}>
                {t("plugins.action.restart")}
              </Button>
              <Button variant="destructive" size="sm" onClick={() => setConfirmUninstall(true)}>
                {t("plugins.action.uninstall")}
              </Button>
            </div>
          </div>
        </DetailHero>

        {d.error && (d.state === "failed" || d.state === "retrying") && (
          <div className={cn(ALERT_DESTRUCTIVE, "mb-5")}>{d.error}</div>
        )}

        <div className="grid grid-cols-1 gap-5 lg:grid-cols-[minmax(0,1fr)_320px]">
          <Panel
            title={t("plugins.detail.settings")}
            action={<span className="hidden text-xs text-muted-foreground sm:inline">{t("plugins.detail.settingsHint", { name })}</span>}
          >
            {d.has_setup && (
              <div className="mb-5 flex flex-wrap items-center justify-between gap-3 rounded-sm border border-border/70 bg-muted/40 px-4 py-3">
                <div>
                  <div className="text-sm font-medium">{setupLabel}</div>
                  <div className="text-xs text-muted-foreground">{t("plugins.setup.cardHint")}</div>
                </div>
                <Button size="sm" onClick={() => setSetupOpen(true)}>
                  <QrCodeIcon />
                  {t("plugins.setup.start")}
                </Button>
              </div>
            )}
            <SettingsForm
              key={`${resetKey}:${JSON.stringify(d.config)}:${d.bot_name}`}
              detail={d}
              name={name}
              onReset={() => setResetKey((k) => k + 1)}
            />
          </Panel>
          <InfoPanel
            detail={d}
            entry={entry}
            installing={install.isPending || Boolean(entry && pendingInstalls(operations.data ?? [])[entry.package])}
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
          onOpenChange={setConfirmUninstall}
          onConfirm={() =>
            uninstall.mutate(d.id, {
              onSuccess: () => navigate("/plugins?tab=installed"),
              onError: (err) => toast.error(getErrorMessage(err)),
            })
          }
        />
      </div>
    </FadeIn>
  )
}

function BackLink() {
  const { t } = useTranslation()
  return (
    <Link to="/plugins?tab=installed" className="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
      <ArrowLeftIcon className="size-4" />
      {t("plugins.detail.back")}
    </Link>
  )
}

function Field({ label, htmlFor, required, help, children }: { label: string; htmlFor: string; required?: boolean; help?: string; children: ReactNode }) {
  const { t } = useTranslation()
  return (
    <div className="space-y-1.5">
      <Label htmlFor={htmlFor} className="text-sm">
        {label}
        {required && <span className="ml-1 text-xs font-normal text-muted-foreground">{t("plugins.detail.required")}</span>}
      </Label>
      {children}
      {help && <p className="text-xs text-muted-foreground">{help}</p>}
    </div>
  )
}

function SettingsForm({ detail, name, onReset }: { detail: Detail; name: string; onReset: () => void }) {
  const { t, i18n } = useTranslation()
  const lang = i18n.language
  const save = useUpdatePluginSettings(detail.id)
  const [values, setValues] = useState<FormValues>(() => initialFormValues(detail.fields, detail.config))
  const [botName, setBotName] = useState(detail.bot_name)
  const [invalid, setInvalid] = useState<string[]>([])

  const labelOf = (f: PluginConfigField) => localize(f.label, lang) || f.key
  const set = (key: string, value: string | boolean) => setValues((v) => ({ ...v, [key]: value }))

  const submit = () => {
    const bad = invalidIntFields(detail.fields, values)
    setInvalid(bad)
    if (bad.length > 0) {
      const field = detail.fields.find((f) => f.key === bad[0])
      toast.error(t("plugins.detail.intInvalid", { label: field ? labelOf(field) : bad[0] }))
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
      onSuccess: () =>
        toast.success(detail.enabled ? t("plugins.toast.savedRestarting", { name }) : t("plugins.toast.savedEnableHint")),
      onError: (err) => toast.error(getErrorMessage(err)),
    })
  }

  const input = (f: PluginConfigField) => {
    const id = `plugin-field-${f.key}`
    const value = values[f.key]
    const defaultHint = f.default !== undefined && f.default !== null && f.default !== "" ? t("plugins.detail.defaultValue", { value: String(f.default) }) : undefined
    switch (f.type) {
      case "bool":
        return (
          <Switch id={id} checked={Boolean(value)} onCheckedChange={(checked) => set(f.key, checked)} />
        )
      case "list":
        return (
          <Textarea
            id={id}
            rows={3}
            className="rounded-sm font-mono"
            value={String(value ?? "")}
            placeholder={t("plugins.detail.listHelp")}
            onChange={(e) => set(f.key, e.target.value)}
          />
        )
      case "secret": {
        const mask = typeof detail.config[f.key] === "string" ? String(detail.config[f.key]) : ""
        return (
          <Input
            id={id}
            type="password"
            autoComplete="new-password"
            className="rounded-sm"
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
            inputMode={f.type === "int" ? "numeric" : undefined}
            aria-invalid={invalid.includes(f.key) || undefined}
            className={cn("rounded-sm", f.type === "int" && "font-mono")}
            value={String(value ?? "")}
            placeholder={defaultHint}
            onChange={(e) => set(f.key, e.target.value)}
          />
        )
    }
  }

  return (
    <form
      className="space-y-5"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      {detail.fields.map((f) => (
        <Field key={f.key} htmlFor={`plugin-field-${f.key}`} label={labelOf(f)} required={f.required} help={localize(f.help, lang) || undefined}>
          {input(f)}
        </Field>
      ))}
      <Field htmlFor="plugin-bot-name" label={t("plugins.detail.botName")} help={t("plugins.detail.botNameHelp")}>
        <Input
          id="plugin-bot-name"
          className="rounded-sm"
          value={botName}
          onChange={(e) => setBotName(e.target.value)}
        />
      </Field>
      <div className="flex items-center justify-end gap-2 border-t border-border/70 pt-4">
        <Button type="button" variant="ghost" size="sm" onClick={onReset}>
          {t("plugins.detail.reset")}
        </Button>
        <Button type="submit" size="sm" disabled={save.isPending}>
          {t("common.save")}
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
  const sourceLabel = detail.source === "npm" ? t("plugins.detail.sourceNpm") : detail.source === "path" ? t("plugins.detail.sourcePath") : detail.source
  const repo = entry?.repository
  return (
    <Panel title={t("plugins.detail.info")}>
      <div className="space-y-4">
        {detail.package && <DetailField label={t("plugins.detail.package")} value={detail.package} mono />}
        <DetailField
          label={t("plugins.detail.version")}
          value={
            <div className="space-y-2">
              <div>
                <span className="font-mono">{detail.version || "—"}</span>
                {entry && detail.source === "npm" && (
                  <span className="ml-2 text-xs text-muted-foreground">
                    {entry.update_available ? t("plugins.detail.newer", { version: entry.version }) : t("plugins.detail.latest")}
                  </span>
                )}
              </div>
              {entry?.update_available && entry.installable && (
                <Button size="sm" variant="outline" disabled={installing} onClick={() => onUpdate(`${entry.package}@${entry.version}`)}>
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
              <a href={repo} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 break-all text-primary-strong hover:underline">
                {repo.replace(/^https?:\/\//, "")}
                <ExternalLinkIcon className="size-3.5 shrink-0" />
              </a>
            }
          />
        )}
        {entry?.license && <DetailField label={t("plugins.detail.license")} value={entry.license} />}
      </div>
    </Panel>
  )
}
