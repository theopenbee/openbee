import { Fragment, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { Link, useNavigate, useSearchParams } from "react-router-dom"
import { toast } from "sonner"
import { CheckIcon, Loader2Icon, MoreHorizontalIcon, RefreshCwIcon, RotateCwIcon, SearchIcon, Trash2Icon } from "lucide-react"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { Panel } from "@/components/panel"
import { EmptyState } from "@/components/empty-state"
import { SkeletonCard, SkeletonTable } from "@/components/skeleton-loader"
import { PluginStateLabel, PluginTile, UninstallPluginDialog, useNow, usePluginOperationToasts } from "@/components/plugin-ui"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  useInstallPlugin,
  usePluginAction,
  usePluginCatalog,
  usePlugins,
  useRefreshPluginCatalog,
  useUninstallPlugin,
  type PluginAction,
} from "@/hooks/use-plugins"
import { filterCatalog, formatElapsed, localize, marketAction, pendingInstalls } from "@/lib/plugins"
import { ALERT_DESTRUCTIVE, EYEBROW_LABEL } from "@/lib/styles"
import { cn, getErrorMessage } from "@/lib/utils"
import type { CatalogPlugin, PluginOperation, PluginStatus } from "@/lib/types"

type Tab = "market" | "installed"

const ROW_GRID = "md:grid md:grid-cols-[minmax(0,2fr)_90px_minmax(0,2fr)_120px_200px] md:items-center md:gap-4"

export function Plugins() {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const tab: Tab = params.get("tab") === "installed" ? "installed" : "market"
  const plugins = usePlugins()
  const refresh = useRefreshPluginCatalog()
  const operations = usePluginOperationToasts()

  const setTab = (next: Tab) => setParams(next === "market" ? {} : { tab: next }, { replace: true })

  return (
    <FadeIn>
      <div className="mx-auto w-full max-w-6xl">
        <PageHeader
          title={t("plugins.title")}
          subtitle={t("plugins.subtitle")}
          actions={
            tab === "market" ? (
              <Button
                variant="outline"
                size="sm"
                disabled={refresh.isPending}
                onClick={() => refresh.mutate(undefined, { onError: (err) => toast.error(getErrorMessage(err)) })}
              >
                <RefreshCwIcon className={cn(refresh.isPending && "animate-spin")} />
                {t("plugins.refreshCatalog")}
              </Button>
            ) : undefined
          }
        />
        <div role="tablist" className="mb-5 flex gap-6 border-b border-border/70">
          {(["market", "installed"] as const).map((key) => (
            <button
              key={key}
              role="tab"
              aria-selected={tab === key}
              onClick={() => setTab(key)}
              className={cn(
                "relative -mb-px inline-flex h-9 items-center gap-1.5 border-b-2 px-1 text-sm",
                tab === key ? "border-primary font-medium text-foreground" : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              {t(`plugins.tabs.${key}`)}
              {key === "installed" && plugins.data && (
                <span className="rounded-sm bg-muted px-1.5 text-xs text-muted-foreground">{plugins.data.length}</span>
              )}
            </button>
          ))}
        </div>
        {tab === "market" ? (
          <Market operations={operations.data ?? []} />
        ) : (
          <Installed onBrowse={() => setTab("market")} />
        )}
      </div>
    </FadeIn>
  )
}

function Market({ operations }: { operations: PluginOperation[] }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const catalog = usePluginCatalog()
  const install = useInstallPlugin()
  const [query, setQuery] = useState("")
  const [tag, setTag] = useState<string | null>(null)
  const [requested, setRequested] = useState<Set<string>>(() => new Set())
  const pending = useMemo(() => pendingInstalls(operations), [operations])
  const now = useNow(Object.keys(pending).length > 0 || requested.size > 0)
  const lang = i18n.language

  if (catalog.isLoading) {
    return (
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
        {Array.from({ length: 6 }, (_, i) => <SkeletonCard key={i} />)}
      </div>
    )
  }
  if (catalog.error || !catalog.data) {
    return (
      <div className={cn(ALERT_DESTRUCTIVE, "flex items-center justify-between gap-3")}>
        <span>{getErrorMessage(catalog.error)}</span>
        <Button size="sm" variant="outline" onClick={() => catalog.refetch()}>
          {t("common.retry")}
        </Button>
      </div>
    )
  }

  const usedTags = new Set(catalog.data.plugins.flatMap((p) => p.tags))
  const tags = catalog.data.tags.filter((v) => usedTags.has(v.id))
  const list = filterCatalog(catalog.data.plugins, query, tag)

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
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <div className="relative w-full sm:w-72">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="rounded-sm pl-8"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("plugins.searchPlaceholder")}
            aria-label={t("plugins.searchPlaceholder")}
          />
        </div>
        <div className="flex flex-wrap gap-1.5">
          {[{ id: null, label: t("plugins.allTags") }, ...tags.map((v) => ({ id: v.id, label: localize(v.name, lang) || v.id }))].map((chip) => (
            <button
              key={chip.id ?? "all"}
              onClick={() => setTag(chip.id)}
              aria-pressed={tag === chip.id}
              className={cn(
                "h-7 rounded-sm border px-2.5 text-xs",
                tag === chip.id
                  ? "border-foreground/80 bg-foreground text-background"
                  : "border-border/70 text-muted-foreground hover:text-foreground",
              )}
            >
              {chip.label}
            </button>
          ))}
        </div>
      </div>
      {list.length === 0 ? (
        <EmptyState title={t("plugins.noResults")} />
      ) : (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
          {list.map((p) => {
            const op = pending[p.package]
            const action = marketAction(p, Boolean(op) || requested.has(p.package))
            const name = localize(p.name, lang) || p.id
            const deprecated = localize(p.deprecated, lang)
            return (
              <div key={p.id} className="flex flex-col rounded-sm border border-border/70 bg-card p-4">
                <div className="flex items-start gap-3">
                  <PluginTile name={name} />
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium">{name}</span>
                      {p.official && <Badge variant="outline">{t("plugins.official")}</Badge>}
                      {deprecated && (
                        <Badge variant="outline" className="border-destructive/30 text-destructive" title={deprecated}>
                          {t("plugins.deprecated")}
                        </Badge>
                      )}
                    </div>
                    <div className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
                      {p.id} · v{p.version}
                    </div>
                  </div>
                </div>
                <p className="mt-3 line-clamp-2 min-h-10 text-sm text-muted-foreground">{localize(p.description, lang)}</p>
                <div className="mt-3 flex items-center justify-between gap-2">
                  <div className="flex min-w-0 flex-wrap gap-1">
                    {p.tags.map((id) => (
                      <span key={id} className="rounded-sm bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground">
                        {localize(catalog.data.tags.find((v) => v.id === id)?.name, lang) || id}
                      </span>
                    ))}
                  </div>
                  <MarketButton
                    action={action}
                    plugin={p}
                    op={op}
                    now={now}
                    onInstall={() => startInstall(p)}
                    onConfigure={() => navigate(`/plugins/${p.id}`)}
                  />
                </div>
                {p.installed && (
                  <div className="mt-2 flex items-center gap-1 text-xs text-muted-foreground">
                    <CheckIcon className="size-3.5 text-status-idle" />
                    {p.update_available
                      ? t("plugins.updateAvailable", { version: p.installed_version })
                      : t("plugins.installedVersion", { version: p.installed_version })}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </>
  )
}

function MarketButton({
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
      return (
        <Button size="sm" variant="outline" disabled>
          <Loader2Icon className="animate-spin" />
          {t("plugins.action.installing", { time: formatElapsed(now - (Date.parse(op?.created_at ?? "") || now)) })}
        </Button>
      )
    case "configure":
      return (
        <Button size="sm" variant="outline" onClick={onConfigure}>
          {t("plugins.action.configure")}
        </Button>
      )
    case "update":
      return (
        <Button size="sm" variant="outline" onClick={onInstall}>
          {t("plugins.action.update", { version: plugin.version })}
        </Button>
      )
    case "incompatible":
      return (
        <Button size="sm" variant="outline" disabled>
          {t("plugins.action.incompatible")}
        </Button>
      )
    default:
      return (
        <Button size="sm" onClick={onInstall}>
          {t("plugins.action.install")}
        </Button>
      )
  }
}

function Installed({ onBrowse }: { onBrowse: () => void }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const plugins = usePlugins()
  const action = usePluginAction()
  const uninstall = useUninstallPlugin()
  const [target, setTarget] = useState<PluginStatus | null>(null)
  const lang = i18n.language

  if (plugins.isLoading) return <SkeletonTable rows={4} columns={5} />
  if (plugins.error) return <div className={ALERT_DESTRUCTIVE}>{getErrorMessage(plugins.error)}</div>

  const list = plugins.data ?? []
  if (list.length === 0) {
    return (
      <EmptyState
        title={t("plugins.emptyInstalled.title")}
        description={t("plugins.emptyInstalled.description")}
        action={<Button onClick={onBrowse}>{t("plugins.emptyInstalled.action")}</Button>}
      />
    )
  }

  const nameOf = (p: PluginStatus) => localize(p.name, lang) || p.id
  const run = (p: PluginStatus, kind: PluginAction) =>
    action.mutate(
      { id: p.id, action: kind },
      {
        onSuccess: () => toast.success(t(`plugins.toast.${kind === "enable" ? "enabled" : kind === "disable" ? "disabled" : "restarted"}`, { name: nameOf(p) })),
        onError: (err) => toast.error(getErrorMessage(err)),
      },
    )
  const running = list.filter((p) => p.state === "running").length

  return (
    <>
      <Panel
        title={t("plugins.installedTitle")}
        flush
        action={<span className="text-xs text-muted-foreground">{t("plugins.installedSummary", { count: list.length, running })}</span>}
      >
        <div className={cn("hidden border-b border-border/70 px-5 py-2", ROW_GRID)}>
          {[t("plugins.columns.plugin"), t("plugins.columns.version"), t("plugins.columns.status"), t("plugins.columns.botName"), ""].map((h, i) => (
            <div key={i} className={EYEBROW_LABEL}>{h}</div>
          ))}
        </div>
        {list.map((p) => (
          <div key={p.id} className={cn("flex flex-col gap-3 border-b border-border/50 px-5 py-3 last:border-b-0", ROW_GRID)}>
            <Link to={`/plugins/${p.id}`} className="flex min-w-0 items-center gap-3">
              <PluginTile name={nameOf(p)} className="size-8" />
              <div className="min-w-0">
                <div className="truncate font-medium hover:text-primary">{nameOf(p)}</div>
                <div className="truncate font-mono text-xs text-muted-foreground">{p.id}</div>
              </div>
            </Link>
            <div className="font-mono text-sm">{p.version || "—"}</div>
            <div className="min-w-0">
              <PluginStateLabel state={p.state} />
              {p.error && (p.state === "failed" || p.state === "retrying") && (
                <div className="mt-0.5 truncate text-xs text-destructive" title={p.error}>{p.error}</div>
              )}
            </div>
            <div className="truncate text-sm">{p.bot_name || <span className="text-muted-foreground">—</span>}</div>
            <div className="flex justify-end gap-2">
              <Button size="sm" variant="outline" onClick={() => navigate(`/plugins/${p.id}`)}>
                {t("plugins.action.configure")}
              </Button>
              {p.enabled ? (
                <Button size="sm" variant="outline" disabled={action.isPending} onClick={() => run(p, "disable")}>
                  {t("plugins.action.disable")}
                </Button>
              ) : (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={action.isPending || p.state === "not_configured"}
                  onClick={() => run(p, "enable")}
                >
                  {t("plugins.action.enable")}
                </Button>
              )}
              <DropdownMenu>
                <DropdownMenuTrigger render={<Button variant="ghost" size="icon-sm" aria-label={t("plugins.action.more")} />}>
                  <MoreHorizontalIcon className="size-4" />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="min-w-36">
                  {p.enabled && (
                    <Fragment>
                      <DropdownMenuItem onClick={() => run(p, "restart")}>
                        <RotateCwIcon className="size-4" />
                        {t("plugins.action.restart")}
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                    </Fragment>
                  )}
                  <DropdownMenuItem variant="destructive" onClick={() => setTarget(p)}>
                    <Trash2Icon className="size-4" />
                    {t("plugins.action.uninstall")}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>
        ))}
      </Panel>
      <UninstallPluginDialog
        name={target ? nameOf(target) : ""}
        open={target !== null}
        pending={uninstall.isPending}
        onOpenChange={(open) => !open && setTarget(null)}
        onConfirm={() => {
          if (!target) return
          uninstall.mutate(target.id, {
            onSuccess: () => setTarget(null),
            onError: (err) => toast.error(getErrorMessage(err)),
          })
        }}
      />
    </>
  )
}
