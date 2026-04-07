import { useState, type ReactNode } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { ArrowRight, Clock3, MessageSquare, Plus, Search, Trash2, X } from "lucide-react"
import { useLocalSessions, useCreateSession, useDeleteSession } from "@/hooks/use-local-chat"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { SkeletonLine } from "@/components/skeleton-loader"
import type { LocalChatSession } from "@/lib/types"

import { isSameDay } from "@/lib/format"

type SessionGroupKey = "today" | "thisWeek" | "earlier"

function getSessionGroup(updatedAt: number, now: number): SessionGroupKey {
  if (isSameDay(updatedAt, now)) return "today"

  const dayMs = 24 * 60 * 60 * 1000
  if (now - updatedAt < 7 * dayMs) return "thisWeek"
  return "earlier"
}

function formatSessionTime(timestamp: number, language: string) {
  return new Intl.DateTimeFormat(language, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(new Date(timestamp))
}

function formatSessionStatTime(timestamp: number, language: string) {
  return new Intl.DateTimeFormat(language, isSameDay(timestamp, Date.now())
    ? {
        hour: "numeric",
        minute: "2-digit",
      }
    : {
        month: "short",
        day: "numeric",
        hour: "numeric",
        minute: "2-digit",
      }).format(new Date(timestamp))
}

function SessionListSkeleton() {
  return (
    <div className="divide-y divide-border/70">
      {Array.from({ length: 4 }).map((_, index) => (
        <div key={index} className="space-y-3 px-5 py-4">
          <SkeletonLine className="h-5 w-56" />
          <div className="flex flex-wrap gap-2">
            <SkeletonLine className="h-4 w-40" />
            <SkeletonLine className="h-4 w-32" />
          </div>
        </div>
      ))}
    </div>
  )
}

function EmptyPanel({
  title,
  description,
  action,
}: {
  title: string
  description: string
  action?: ReactNode
}) {
  return (
    <div className="flex flex-col items-start gap-4 px-5 py-14 sm:px-6">
      <div className="inline-flex size-11 items-center justify-center rounded-2xl border border-border/70 bg-muted/40">
        <MessageSquare className="size-5 text-muted-foreground" />
      </div>
      <div className="space-y-2">
        <h2 className="text-lg font-semibold tracking-tight text-foreground">{title}</h2>
        <p className="max-w-xl text-sm leading-6 text-muted-foreground">{description}</p>
      </div>
      {action}
    </div>
  )
}

export function LocalChat() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const { data: sessions = [], isLoading, error } = useLocalSessions()
  const createSession = useCreateSession()
  const deleteSession = useDeleteSession()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [newName, setNewName] = useState("")
  const [searchQuery, setSearchQuery] = useState("")

  const trimmedNewName = newName.trim()
  const normalizedQuery = searchQuery.trim().toLowerCase()
  const sortedSessions = [...sessions].sort((left, right) => right.updated_at - left.updated_at)
  const filteredSessions = normalizedQuery
    ? sortedSessions.filter((session) => session.name.toLowerCase().includes(normalizedQuery))
    : sortedSessions

  const now = Date.now()
  const updatedTodayCount = sessions.filter((session) => isSameDay(session.updated_at, now)).length
  const latestSession = sortedSessions[0]

  const sessionGroups: Array<{ key: SessionGroupKey; label: string; items: LocalChatSession[] }> = [
    { key: "today", label: t("localChat.groupToday"), items: [] },
    { key: "thisWeek", label: t("localChat.groupThisWeek"), items: [] },
    { key: "earlier", label: t("localChat.groupEarlier"), items: [] },
  ]

  filteredSessions.forEach((session) => {
    const group = sessionGroups.find((entry) => entry.key === getSessionGroup(session.updated_at, now))
    group?.items.push(session)
  })

  const visibleGroups = sessionGroups.filter((group) => group.items.length > 0)

  const handleCreate = async () => {
    if (!trimmedNewName) return
    const session = await createSession.mutateAsync(trimmedNewName)
    setNewName("")
    setDialogOpen(false)
    navigate(`/local-chat/${session.id}`)
  }

  const handleDialogChange = (open: boolean) => {
    setDialogOpen(open)
    if (!open) setNewName("")
  }

  const errorMessage = error instanceof Error ? error.message : ""
  const headerSubtitle = normalizedQuery
    ? t("localChat.summaryFiltered", { shown: filteredSessions.length, total: sessions.length })
    : sessions.length > 0
      ? t("localChat.summary", { count: sessions.length })
      : t("localChat.subtitle")

  return (
    <FadeIn>
      <div className="space-y-6">
        <PageHeader
          title={t("localChat.title")}
          subtitle={headerSubtitle}
          actions={
            <Button onClick={() => setDialogOpen(true)}>
              {t("localChat.newChat")}
            </Button>
          }
        />

        {errorMessage && (
          <div className="rounded-2xl border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
            {errorMessage}
          </div>
        )}

        <div className="grid gap-6 xl:grid-cols-[minmax(0,1.45fr)_19rem]">
          <section
            className="relative overflow-hidden rounded-[2rem] border border-border/70 bg-card/70"
            style={{
              backgroundImage: [
                "radial-gradient(circle at top right, color-mix(in oklch, var(--foreground) 10%, transparent), transparent 28%)",
                "linear-gradient(180deg, color-mix(in oklch, var(--card) 92%, var(--muted) 8%), color-mix(in oklch, var(--card) 84%, var(--background) 16%))",
                "linear-gradient(color-mix(in oklch, var(--border) 45%, transparent) 1px, transparent 1px)",
                "linear-gradient(90deg, color-mix(in oklch, var(--border) 45%, transparent) 1px, transparent 1px)",
              ].join(", "),
              backgroundSize: "100% 100%, 100% 100%, 24px 24px, 24px 24px",
            }}
          >
            <div className="absolute inset-x-6 top-0 h-px bg-gradient-to-r from-transparent via-foreground/25 to-transparent" />
            <div className="relative p-5 sm:p-6 lg:p-7">
              <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_17rem]">
                <div className="space-y-5">
                  <div className="space-y-3">
                    <span className="inline-flex items-center rounded-full border border-border/70 bg-background/70 px-3 py-1 text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                      {t("localChat.controlLabel")}
                    </span>
                    <div className="space-y-3">
                      <h2 className="max-w-3xl text-[clamp(1.8rem,3vw,2.7rem)] font-semibold tracking-[-0.03em] text-foreground">
                        {t("localChat.controlTitle")}
                      </h2>
                      <p className="max-w-2xl text-sm leading-6 text-muted-foreground">
                        {t("localChat.controlDescription")}
                      </p>
                    </div>
                  </div>

                  <div className="relative w-full max-w-xl">
                    <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                    <Input
                      value={searchQuery}
                      onChange={(event) => setSearchQuery(event.target.value)}
                      placeholder={t("localChat.searchPlaceholder")}
                      className="h-10 rounded-2xl border-border/70 bg-background/80 pl-9 pr-10"
                      aria-label={t("localChat.searchPlaceholder")}
                    />
                    {searchQuery && (
                      <button
                        type="button"
                        className="absolute right-2 top-1/2 inline-flex size-6 -translate-y-1/2 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                        onClick={() => setSearchQuery("")}
                        aria-label={t("localChat.clearSearch")}
                      >
                        <X className="size-4" />
                      </button>
                    )}
                  </div>
                </div>

                <div className="grid gap-3 sm:grid-cols-3 xl:grid-cols-1">
                  <div className="rounded-[1.5rem] border border-border/70 bg-background/72 p-4">
                    <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                      {t("localChat.totalSessions")}
                    </p>
                    <p className="mt-3 text-3xl font-semibold tracking-[-0.04em] text-foreground">
                      {sessions.length}
                    </p>
                  </div>
                  <div className="rounded-[1.5rem] border border-border/70 bg-background/72 p-4">
                    <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                      {t("localChat.touchedToday")}
                    </p>
                    <p className="mt-3 text-3xl font-semibold tracking-[-0.04em] text-foreground">
                      {updatedTodayCount}
                    </p>
                  </div>
                  <div className="rounded-[1.5rem] border border-border/70 bg-background/72 p-4">
                    <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                      {t("localChat.latestActivity")}
                    </p>
                    <p className="mt-3 text-sm font-medium text-foreground">
                      {latestSession
                        ? formatSessionStatTime(latestSession.updated_at, i18n.language)
                        : t("localChat.latestActivityEmpty")}
                    </p>
                    {latestSession && (
                      <p className="mt-1 truncate text-xs text-muted-foreground">
                        {latestSession.name}
                      </p>
                    )}
                  </div>
                </div>
              </div>

              <div className="mt-6 overflow-hidden rounded-[1.75rem] border border-border/70 bg-background/82">
                <div className="flex flex-col gap-3 border-b border-border/70 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
                  <div>
                    <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                      {t("localChat.recentSessions")}
                    </p>
                    <p className="mt-1 text-sm text-foreground">
                      {t("localChat.sessionListHint")}
                    </p>
                  </div>
                  <div className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-muted/35 px-3 py-1 text-xs text-muted-foreground">
                    <Clock3 className="size-3.5" />
                    {headerSubtitle}
                  </div>
                </div>

                {isLoading ? (
                  <SessionListSkeleton />
                ) : sessions.length === 0 ? (
                  <EmptyPanel
                    title={t("emptyState.noSessions")}
                    description={t("emptyState.noSessionsDesc")}
                    action={
                      <Button onClick={() => setDialogOpen(true)}>
                        {t("localChat.newChat")}
                      </Button>
                    }
                  />
                ) : filteredSessions.length === 0 ? (
                  <EmptyPanel
                    title={t("localChat.noMatches")}
                    description={t("localChat.noMatchesDesc")}
                    action={
                      <Button variant="outline" onClick={() => setSearchQuery("")}>
                        {t("localChat.clearSearch")}
                      </Button>
                    }
                  />
                ) : (
                  <div>
                    {visibleGroups.map((group) => (
                      <div key={group.key}>
                        <div className="flex items-center gap-3 px-5 py-3 text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                          <span>{group.label}</span>
                          <span className="h-px flex-1 bg-border/70" />
                          <span>{group.items.length}</span>
                        </div>
                        <div className="divide-y divide-border/70">
                          {group.items.map((session) => (
                            <div
                              key={session.id}
                              className="grid gap-4 px-5 py-4 transition-colors hover:bg-muted/25 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center"
                            >
                              <div className="min-w-0">
                                <div className="flex flex-wrap items-center gap-2">
                                  <Link
                                    to={`/local-chat/${session.id}`}
                                    className="truncate text-sm font-semibold text-foreground transition-colors hover:text-primary"
                                  >
                                    {session.name}
                                  </Link>
                                  {isSameDay(session.updated_at, now) && (
                                    <span className="inline-flex items-center rounded-full border border-border/70 bg-muted/35 px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
                                      {t("localChat.updatedToday")}
                                    </span>
                                  )}
                                </div>
                                <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
                                  <span className="inline-flex items-center gap-1.5">
                                    <Clock3 className="size-3.5" />
                                    {t("localChat.updatedAt", {
                                      time: formatSessionTime(session.updated_at, i18n.language),
                                    })}
                                  </span>
                                  <span>
                                    {t("localChat.createdAt", {
                                      time: formatSessionTime(session.created_at, i18n.language),
                                    })}
                                  </span>
                                </div>
                              </div>

                              <div className="flex items-center gap-2">
                                <Link
                                  to={`/local-chat/${session.id}`}
                                  className="inline-flex h-8 items-center gap-1.5 rounded-lg border border-border/70 bg-background px-3 text-sm font-medium text-foreground transition-colors hover:bg-muted"
                                >
                                  {t("localChat.openSession")}
                                  <ArrowRight className="size-4" />
                                </Link>
                                <Button
                                  variant="ghost"
                                  size="icon-sm"
                                  className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                                  onClick={() => deleteSession.mutate(session.id)}
                                  disabled={deleteSession.isPending}
                                  aria-label={t("localChat.deleteSession")}
                                >
                                  <Trash2 className="size-4" />
                                </Button>
                              </div>
                            </div>
                          ))}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </section>

          <aside className="space-y-4">
            <section className="rounded-[1.75rem] border border-border/70 bg-card/70 p-5">
              <div className="inline-flex size-11 items-center justify-center rounded-2xl border border-border/70 bg-background/80">
                <Plus className="size-5 text-foreground" />
              </div>
              <h2 className="mt-4 text-lg font-semibold tracking-tight text-foreground">
                {t("localChat.createPanelTitle")}
              </h2>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                {t("localChat.createPanelDescription")}
              </p>
              <Button className="mt-6 w-full justify-between" onClick={() => setDialogOpen(true)}>
                {t("localChat.newChat")}
                <ArrowRight className="size-4" />
              </Button>
              <p className="mt-3 text-xs leading-5 text-muted-foreground">
                {t("localChat.createPanelHint")}
              </p>
            </section>

            <section className="rounded-[1.75rem] border border-border/70 bg-card/55 p-5">
              <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                {t("localChat.latestPanelLabel")}
              </p>
              {latestSession ? (
                <div className="mt-4 space-y-4">
                  <div className="space-y-2">
                    <h2 className="text-lg font-semibold tracking-tight text-foreground">
                      {latestSession.name}
                    </h2>
                    <p className="text-sm leading-6 text-muted-foreground">
                      {t("localChat.updatedAt", {
                        time: formatSessionTime(latestSession.updated_at, i18n.language),
                      })}
                    </p>
                  </div>
                  <div className="grid grid-cols-2 gap-3 border-t border-border/70 pt-4">
                    <div>
                      <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                        {t("localChat.totalSessions")}
                      </p>
                      <p className="mt-2 text-xl font-semibold tracking-[-0.04em] text-foreground">
                        {sessions.length}
                      </p>
                    </div>
                    <div>
                      <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                        {t("localChat.touchedToday")}
                      </p>
                      <p className="mt-2 text-xl font-semibold tracking-[-0.04em] text-foreground">
                        {updatedTodayCount}
                      </p>
                    </div>
                  </div>
                  <Link
                    to={`/local-chat/${latestSession.id}`}
                    className="inline-flex w-full items-center justify-between rounded-xl border border-border/70 bg-background/80 px-3 py-2.5 text-sm font-medium text-foreground transition-colors hover:bg-muted"
                  >
                    {t("localChat.openSession")}
                    <ArrowRight className="size-4" />
                  </Link>
                </div>
              ) : (
                <p className="mt-4 text-sm leading-6 text-muted-foreground">
                  {t("localChat.latestPanelEmpty")}
                </p>
              )}
            </section>
          </aside>
        </div>
      </div>

      <Dialog open={dialogOpen} onOpenChange={handleDialogChange}>
        <DialogContent className="max-w-lg gap-0 overflow-hidden p-0">
          <div className="border-b border-border/70 bg-muted/30 px-6 py-5">
            <DialogHeader className="gap-2">
              <DialogTitle>{t("localChat.newSessionTitle")}</DialogTitle>
              <DialogDescription>
                {t("localChat.newSessionDescription")}
              </DialogDescription>
            </DialogHeader>
          </div>

          <form
            className="space-y-5 px-6 py-5"
            onSubmit={(event) => {
              event.preventDefault()
              void handleCreate()
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="local-chat-session-name">{t("localChat.sessionNameLabel")}</Label>
              <Input
                id="local-chat-session-name"
                placeholder={t("localChat.sessionNamePlaceholder")}
                value={newName}
                onChange={(event) => setNewName(event.target.value)}
                autoFocus
                className="h-10 rounded-xl"
              />
              <p className="text-xs leading-5 text-muted-foreground">
                {t("localChat.sessionNameHint")}
              </p>
            </div>

            <DialogFooter className="-mx-6 -mb-5 mt-6 px-6">
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={!trimmedNewName || createSession.isPending}>
                {t("localChat.newChat")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </FadeIn>
  )
}
