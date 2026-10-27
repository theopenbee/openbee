import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { BotIcon, CheckIcon, DownloadIcon, EllipsisIcon, PackagePlusIcon, PuzzleIcon, RefreshCwIcon, RotateCwIcon, SearchXIcon, Trash2Icon } from "lucide-react"
import { FadeIn } from "@/components/fade-in"
import { Panel } from "@/components/panel"
import { EmptyState } from "@/components/empty-state"
import { SkeletonCard, SkeletonTable } from "@/components/skeleton-loader"
import {
  OperationProgress,
  PluginAvatar,
  PluginStateBadge,
  UninstallPluginDialog,
  pendingUninstalls,
  useNow,
  usePluginOperationToasts,
} from "@/components/plugin-ui"
import { PluginInstallDialog } from "@/components/plugin-install-dialog"
import { Alert } from "@/components/arc/alert/alert"
import { Badge } from "@/components/arc/badge/badge"
import { Button } from "@/components/arc/button/button"
import { DropdownMenu } from "@/components/arc/dropdown-menu/dropdown-menu"
import { SearchField } from "@/components/arc/search-field/search-field"
import SegmentedControl from "@/components/arc/segmented-control/segmented-control"
import { Switch } from "@/components/arc/switch/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/arc/tabs/tabs"
import { Tooltip } from "@/components/arc/tooltip/tooltip"
import {
  useInstallPlugin,
  usePluginAction,
  usePluginCatalog,
  usePlugins,
  useRefreshPluginCatalog,
  useUninstallPlugin,
  type PluginAction,
} from "@/hooks/use-plugins"
import { useSettingsDialog, type PluginsTab } from "@/hooks/use-settings-dialog"
import { filterCatalog, localize, marketAction, pendingInstalls } from "@/lib/plugins"
import { getErrorMessage } from "@/lib/utils"
import type { CatalogPlugin, PluginOperation, PluginStatus } from "@/lib/types"

const ALL_TAGS = "__all__"
const INSTALLED_GRID = "@3xl:grid-cols-[minmax(0,1fr)_5.5rem_minmax(0,12rem)_minmax(0,10rem)_auto]"

export function PluginsSettings({ initialTab = "market" }: { initialTab?: PluginsTab }) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<PluginsTab>(initialTab)
  const plugins = usePlugins()
  const operations = usePluginOperationToasts()
  const [installOpen, setInstallOpen] = useState(false)
  const ops = operations.data ?? []

  return (
    <FadeIn>
      <Tabs value={tab} onValueChange={(value) => setTab(value === "installed" ? "installed" : "market")}>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <TabsList aria-label={t("plugins.title")}>
            <TabsTrigger value="market">{t("plugins.tabs.market")}</TabsTrigger>
            <TabsTrigger value="installed">
              <span className="inline-flex items-center gap-1.5">
                {t("plugins.tabs.installed")}
                {plugins.data && <span className="text-xs text-secondary tabular">{plugins.data.length}</span>}
              </span>
            </TabsTrigger>
          </TabsList>
          <Button variant="secondary" size="sm" onClick={() => setInstallOpen(true)}>
            <PackagePlusIcon size={16} strokeWidth={1.75} aria-hidden="true" />
            {t("plugins.installDialog.open")}
          </Button>
        </div>
        <TabsContent value="market">
          <div className="pt-2">
            <Market operations={ops} />
          </div>
        </TabsContent>
        <TabsContent value="installed">
          <div className="pt-2">
            <Installed operations={ops} onBrowse={() => setTab("market")} />
          </div>
        </TabsContent>
      </Tabs>
      <PluginInstallDialog open={installOpen} onOpenChange={setInstallOpen} operations={ops} />
    </FadeIn>
  )
}

function Market({ operations }: { operations: PluginOperation[] }) {
  const { t, i18n } = useTranslation()
  const show = useSettingsDialog()
  const catalog = usePluginCatalog()
  const refresh = useRefreshPluginCatalog()
  const install = useInstallPlugin()
  const [query, setQuery] = useState("")
  const [tag, setTag] = useState<string | null>(null)
  const [requested, setRequested] = useState<Set<string>>(() => new Set())
  const pending = useMemo(() => pendingInstalls(operations), [operations])
  const now = useNow(Object.keys(pending).length > 0 || requested.size > 0)
  const lang = i18n.language

  const toolbar = (
    <div className="mb-6 flex flex-wrap items-end gap-4">
      <div className="w-full min-w-0 @xl:w-72">
        <SearchField
          label={t("plugins.search")}
          value={query}
          onValueChange={setQuery}
          placeholder={t("plugins.searchPlaceholder")}
          disabled={!catalog.data}
        />
      </div>
      {catalog.data && (
        <div className="min-w-0 max-w-full">
          <SegmentedControl
            label={t("plugins.tagFilter")}
            value={tag ?? ALL_TAGS}
            onValueChange={(value) => setTag(value === ALL_TAGS ? null : value)}
            options={[
              { value: ALL_TAGS, label: t("plugins.allTags") },
              ...catalog.data.tags
                .filter((v) => catalog.data.plugins.some((p) => p.tags.includes(v.id)))
                .map((v) => ({ value: v.id, label: localize(v.name, lang) || v.id })),
            ]}
          />
        </div>
      )}
      <div className="ml-auto flex h-11 items-center">
        <Button
          variant="ghost"
          size="sm"
          loading={refresh.isPending}
          onClick={() => refresh.mutate(undefined, { onError: (err) => toast.error(getErrorMessage(err)) })}
        >
          <RefreshCwIcon size={16} strokeWidth={1.75} aria-hidden="true" />
          {t("plugins.refreshCatalog")}
        </Button>
      </div>
    </div>
  )

  if (catalog.isLoading) {
    return (
      <>
        {toolbar}
        <div className="grid gap-4 @xl:grid-cols-2 @4xl:grid-cols-3" aria-busy="true">
          {Array.from({ length: 6 }, (_, i) => <SkeletonCard key={i} className="rounded-inset" />)}
        </div>
      </>
    )
  }
  if (catalog.error || !catalog.data) {
    return (
      <>
        {toolbar}
        <Alert tone="danger" title={t("plugins.catalogError")}>
          <span className="mt-1 flex flex-wrap items-center gap-3">
            <span className="wrap-anywhere">{getErrorMessage(catalog.error)}</span>
            <Button variant="secondary" size="sm" onClick={() => catalog.refetch()}>
              {t("common.retry")}
            </Button>
          </span>
        </Alert>
      </>
    )
  }

  const data = catalog.data
  const list = filterCatalog(data.plugins, query, tag)
  const tagName = (id: string) => localize(data.tags.find((v) => v.id === id)?.name, lang) || id

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
    <>
      {toolbar}
      {list.length === 0 ? (
        <div className="rounded-inset border border-border bg-surface">
          <EmptyState
            icon={<SearchXIcon size={24} strokeWidth={1.5} />}
            title={t("plugins.noResults")}
            description={t("plugins.noResultsHint")}
            action={
              <Button
                variant="secondary"
                onClick={() => {
                  setQuery("")
                  setTag(null)
                }}
              >
                {t("plugins.clearFilters")}
              </Button>
            }
          />
        </div>
      ) : (
        <ul className="grid gap-4 @xl:grid-cols-2 @4xl:grid-cols-3">
          {list.map((p) => {
            const op = pending[p.package]
            const action = marketAction(p, Boolean(op) || requested.has(p.package))
            const name = localize(p.name, lang) || p.id
            const deprecated = localize(p.deprecated, lang)
            const description = localize(p.description, lang)
            return (
              <li key={p.id} className="flex min-w-0 flex-col rounded-inset border border-border bg-surface p-6">
                <div className="flex-1">
                  <div className="flex items-start gap-3">
                    <PluginAvatar name={name} />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <h3 className="min-w-0 truncate text-base font-medium text-foreground">{name}</h3>
                        {p.official && <Badge size="sm">{t("plugins.official")}</Badge>}
                        {deprecated && (
                          <Tooltip content={deprecated}>
                            <span tabIndex={0} className="inline-flex rounded-full">
                              <Badge tone="warning" size="sm">{t("plugins.deprecated")}</Badge>
                            </span>
                          </Tooltip>
                        )}
                      </div>
                      <p className="mt-0.5 truncate font-mono text-xs text-secondary">
                        {p.id} · v{p.version}
                      </p>
                    </div>
                  </div>
                  <p className="mt-4 line-clamp-2 min-h-10 text-sm text-secondary">{description}</p>
                  {p.tags.length > 0 && (
                    <p className="mt-3 truncate text-xs text-muted">{p.tags.map(tagName).join(" · ")}</p>
                  )}
                </div>
                <div className="mt-5 flex min-h-9 flex-wrap items-center justify-between gap-x-3 gap-y-2 border-t border-border-subtle pt-4">
                  <div className="min-w-0 text-xs text-secondary">
                    {p.installed && (
                      <span className="inline-flex items-center gap-1.5">
                        <CheckIcon size={14} strokeWidth={1.75} className="shrink-0 text-success" aria-hidden="true" />
                        {p.update_available
                          ? t("plugins.updateAvailable", { version: p.installed_version })
                          : t("plugins.installedVersion", { version: p.installed_version })}
                      </span>
                    )}
                  </div>
                  <MarketAction
                    action={action}
                    plugin={p}
                    op={op}
                    now={now}
                    onInstall={() => startInstall(p)}
                    onConfigure={() => show({ section: "plugins", plugin: p.id })}
                  />
                </div>
              </li>
            )
          })}
        </ul>
      )}
    </>
  )
}

function MarketAction({
  action,
  plugin,
  op,
  now,
  onInstall,
  onConfigure,
}: {
  action: ReturnType<typeof marketAction>
  plugin: CatalogPlugin
  op?: PluginOperation
  now: number
  onInstall: () => void
  onConfigure: () => void
}) {
  const { t } = useTranslation()
  switch (action) {
    case "installing":
      return <OperationProgress label={t("plugins.action.installingShort")} startedAt={op?.created_at} now={now} />
    case "configure":
      return (
        <Button size="sm" variant="secondary" onClick={onConfigure}>
          {t("plugins.action.configure")}
        </Button>
      )
    case "update":
      return (
        <Button size="sm" variant="secondary" onClick={onInstall}>
          {t("plugins.action.update", { version: plugin.version })}
        </Button>
      )
    case "incompatible":
      return <span className="text-xs text-secondary">{t("plugins.action.incompatible")}</span>
    default:
      return (
        <Button size="sm" variant="secondary" onClick={onInstall}>
          <DownloadIcon size={16} strokeWidth={1.75} aria-hidden="true" />
          {t("plugins.action.install")}
        </Button>
      )
  }
}

function Installed({ operations, onBrowse }: { operations: PluginOperation[]; onBrowse: () => void }) {
  const { t, i18n } = useTranslation()
  const show = useSettingsDialog()
  const plugins = usePlugins()
  const action = usePluginAction()
  const uninstall = useUninstallPlugin()
  const [target, setTarget] = useState<PluginStatus | null>(null)
  const [uninstallError, setUninstallError] = useState("")
  const [toggling, setToggling] = useState<Record<string, boolean>>({})
  const removing = useMemo(() => pendingUninstalls(operations), [operations])
  const lang = i18n.language

  if (plugins.isLoading) return <SkeletonTable rows={4} columns={5} className="rounded-inset" />
  if (plugins.error) {
    return (
      <Alert tone="danger" title={t("plugins.installedError")}>
        <span className="mt-1 flex flex-wrap items-center gap-3">
          <span className="wrap-anywhere">{getErrorMessage(plugins.error)}</span>
          <Button variant="secondary" size="sm" onClick={() => plugins.refetch()}>
            {t("common.retry")}
          </Button>
        </span>
      </Alert>
    )
  }

  const list = plugins.data ?? []
  if (list.length === 0) {
    return (
      <div className="rounded-inset border border-border bg-surface">
        <EmptyState
          icon={<PuzzleIcon size={24} strokeWidth={1.5} />}
          title={t("plugins.emptyInstalled.title")}
          description={t("plugins.emptyInstalled.description")}
          action={<Button onClick={onBrowse}>{t("plugins.emptyInstalled.action")}</Button>}
        />
      </div>
    )
  }

  const nameOf = (p: PluginStatus) => localize(p.name, lang) || p.id
  const run = (p: PluginStatus, kind: PluginAction) => {
    if (kind !== "restart") setToggling((prev) => ({ ...prev, [p.id]: kind === "enable" }))
    action.mutate(
      { id: p.id, action: kind },
      {
        onSuccess: () => toast.success(t(`plugins.toast.${kind === "enable" ? "enabled" : kind === "disable" ? "disabled" : "restarted"}`, { name: nameOf(p) })),
        onError: (err) => toast.error(getErrorMessage(err)),
        onSettled: () =>
          setToggling((prev) => {
            const next = { ...prev }
            delete next[p.id]
            return next
          }),
      },
    )
  }
  const running = list.filter((p) => p.state === "running").length

  return (
    <>
      <Panel
        title={t("plugins.installedTitle")}
        className="rounded-inset"
        flush
        action={<span className="text-xs text-secondary tabular">{t("plugins.installedSummary", { count: list.length, running })}</span>}
      >
        <div className={`hidden gap-4 px-6 pb-2 text-xs text-secondary @3xl:grid ${INSTALLED_GRID}`} aria-hidden="true">
          <span>{t("plugins.columns.plugin")}</span>
          <span>{t("plugins.columns.version")}</span>
          <span>{t("plugins.columns.status")}</span>
          <span>{t("plugins.columns.botName")}</span>
          <span className="w-44" />
        </div>
        <ul>
          {list.map((p) => {
            const name = nameOf(p)
            const busy = Boolean(removing[p.id])
            const enabled = toggling[p.id] ?? p.enabled
            const showError = p.error && (p.state === "failed" || p.state === "retrying")
            return (
              <li
                key={p.id}
                className={`grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-3 border-t border-border-subtle px-6 py-4 ${INSTALLED_GRID}`}
              >
                <button
                  type="button"
                  onClick={() => show({ section: "plugins", plugin: p.id })}
                  className="group flex min-w-0 items-center gap-3 text-left"
                >
                  <PluginAvatar name={name} />
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-medium text-foreground group-hover:text-accent-strong">{name}</span>
                    <span className="block truncate font-mono text-xs text-secondary">{p.id}</span>
                  </span>
                </button>
                <div className="col-span-2 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5 @3xl:contents">
                  <span className="font-mono text-sm text-secondary tabular">{p.version ? `v${p.version}` : "–"}</span>
                  <div className={showError ? "order-first w-full min-w-0 @3xl:order-none @3xl:w-auto" : "order-first min-w-0 @3xl:order-none"}>
                    <PluginStateBadge state={p.state} uninstalling={busy} />
                    {showError && (
                      <p className="mt-1 truncate text-xs text-danger" title={p.error}>
                        {p.error}
                      </p>
                    )}
                  </div>
                  <span className="flex min-w-0 items-center gap-1.5 text-sm text-foreground">
                    <BotIcon size={14} strokeWidth={1.75} className="shrink-0 text-muted @3xl:hidden" aria-hidden="true" />
                    <span className="truncate">{p.bot_name || <span className="text-muted">–</span>}</span>
                  </span>
                </div>
                <div className="row-start-1 col-start-2 flex items-center justify-end gap-2 @3xl:col-start-auto @3xl:row-start-auto @3xl:w-44">
                  <Switch
                    aria-label={t("plugins.action.toggle", { name })}
                    checked={enabled}
                    disabled={busy || action.isPending || (!p.enabled && p.state === "not_configured")}
                    onCheckedChange={(checked) => run(p, checked ? "enable" : "disable")}
                  />
                  <span className="hidden @lg:inline-flex">
                    <Button size="sm" variant="secondary" disabled={busy} onClick={() => show({ section: "plugins", plugin: p.id })}>
                      {t("plugins.action.configure")}
                    </Button>
                  </span>
                  <DropdownMenu
                    iconOnly
                    icon={<EllipsisIcon size={16} strokeWidth={1.75} />}
                    label={t("plugins.action.moreFor", { name })}
                    disabled={busy}
                    items={[
                      {
                        label: t("plugins.action.restart"),
                        icon: <RotateCwIcon size={16} strokeWidth={1.75} />,
                        disabled: !p.enabled || action.isPending,
                        onSelect: () => run(p, "restart"),
                      },
                      {
                        label: t("plugins.action.uninstall"),
                        icon: <Trash2Icon size={16} strokeWidth={1.75} />,
                        destructive: true,
                        separatorBefore: true,
                        onSelect: () => {
                          setUninstallError("")
                          setTarget(p)
                        },
                      },
                    ]}
                  />
                </div>
              </li>
            )
          })}
        </ul>
      </Panel>
      <UninstallPluginDialog
        name={target ? nameOf(target) : ""}
        open={target !== null}
        pending={uninstall.isPending}
        error={uninstallError}
        onOpenChange={(open) => !open && setTarget(null)}
        onConfirm={() => {
          if (!target) return
          setUninstallError("")
          uninstall.mutate(target.id, {
            onSuccess: () => setTarget(null),
            onError: (err) => setUninstallError(getErrorMessage(err)),
          })
        }}
      />
    </>
  )
}
