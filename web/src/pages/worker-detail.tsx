import { useState, useMemo } from "react"
import { useParams, Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useWorker, useWorkerExecutions, useUpdateWorker } from "@/hooks/use-workers"
import { useWorkerSkills, useSkill, useSetWorkerSkillVersion, useRemoveWorkerOverride } from "@/hooks/use-skills"
import type { ScannedSkill } from "@/lib/types"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { Pencil } from "lucide-react"
import { StatusBadge } from "@/components/status-badge"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { SkeletonPage } from "@/components/skeleton-loader"
import { EmptyState } from "@/components/empty-state"
import { PaginationControls } from "@/components/pagination-controls"

const PAGE_SIZE = 20

function WorkerSkillRow({
  skill,
  workerId,
}: {
  skill: ScannedSkill
  workerId: string
}) {
  const { t } = useTranslation()
  const { data: skillEntry } = useSkill(skill.source === "managed" ? skill.name : "")
  const setVersion = useSetWorkerSkillVersion(workerId)
  const removeOverride = useRemoveWorkerOverride(workerId)

  const versions = skillEntry ? Object.keys(skillEntry.versions).sort((a, b) => {
    const na = parseInt(a.replace("v", ""), 10)
    const nb = parseInt(b.replace("v", ""), 10)
    return nb - na
  }) : []

  return (
    <div className="flex items-center justify-between py-3 border-b last:border-0">
      <div className="flex items-center gap-2">
        <span className="text-sm font-medium">{skill.name}</span>
        <Badge variant={skill.source === "managed" ? "default" : "secondary"}>
          {t(`skills.source.${skill.source}`)}
        </Badge>
        {skill.is_override && (
          <Badge variant="outline">{t("skills.override")}</Badge>
        )}
        <span className="text-xs font-mono text-muted-foreground">{skill.active_version}</span>
      </div>
      <div className="flex items-center gap-2">
        {skill.source === "managed" && versions.length > 0 && (
          <Select
            value={skill.active_version}
            onValueChange={(v) => v && setVersion.mutate({ name: skill.name, version: v })}
          >
            <SelectTrigger className="h-7 w-24 text-xs" size="sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {versions.map((v) => (
                <SelectItem key={v} value={v}>{v}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        {skill.is_override && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => removeOverride.mutate(skill.name)}
            disabled={removeOverride.isPending}
          >
            {t("skills.resetToGlobal")}
          </Button>
        )}
      </div>
    </div>
  )
}

export function WorkerDetail() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { data: worker, error: workerError } = useWorker(id!)
  const [page, setPage] = useState(1)
  const { data } = useWorkerExecutions(id!, page, PAGE_SIZE)

  const executions = data?.items ?? []
  const totalPages = Math.max(1, Math.ceil((data?.total ?? 0) / PAGE_SIZE))

  const sessionGroups = useMemo(() => {
    const map = new Map<string, typeof executions>()
    for (const e of executions) {
      const group = map.get(e.session_id) ?? []
      group.push(e)
      map.set(e.session_id, group)
    }
    return Array.from(map.values()).sort((a, b) => {
      return (b[0].started_at ?? 0) - (a[0].started_at ?? 0)
    })
  }, [executions])

  const [isEditingDesc, setIsEditingDesc] = useState(false)
  const [editDesc, setEditDesc] = useState("")
  const [isEditingMemory, setIsEditingMemory] = useState(false)
  const [editMemory, setEditMemory] = useState("")
  const updateWorker = useUpdateWorker()
  const { data: workerSkills = [] } = useWorkerSkills(id!)

  if (!worker) return <SkeletonPage />

  return (
    <FadeIn>
      <PageHeader
        title={worker.name}
        subtitle={
          isEditingDesc ? undefined : (worker.description || t("common.noDescription"))
        }
        actions={<StatusBadge status={worker.status} />}
      />

      {/* Inline description editing */}
      {isEditingDesc ? (
        <div className="-mt-6 mb-8 space-y-2">
          <Textarea
            value={editDesc}
            onChange={(e) => setEditDesc(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") { setIsEditingDesc(false); setEditDesc(worker.description) }
            }}
            autoFocus
            rows={2}
            className="text-sm"
          />
          <div className="flex gap-2">
            <Button size="sm" onClick={async () => {
              await updateWorker.mutateAsync({ id: id!, data: { description: editDesc } })
              setIsEditingDesc(false)
            }}>{t("common.save")}</Button>
            <Button size="sm" variant="outline" onClick={() => {
              setIsEditingDesc(false)
              setEditDesc(worker.description)
            }}>{t("common.cancel")}</Button>
          </div>
        </div>
      ) : (
        <div className="-mt-6 mb-8">
          <button
            className="inline-flex items-center gap-1 text-muted-foreground hover:text-foreground transition-colors"
            onClick={() => { setEditDesc(worker.description); setIsEditingDesc(true) }}
          >
            <Pencil className="h-3 w-3" />
          </button>
        </div>
      )}

      {workerError && (
        <p className="text-destructive mb-4">{workerError?.message}</p>
      )}

      <Tabs defaultValue="sessions">
        <TabsList variant="line">
          <TabsTrigger value="sessions">{t("workerDetail.sessions")}</TabsTrigger>
          <TabsTrigger value="info">{t("executionDetail.info")}</TabsTrigger>
          <TabsTrigger value="skills">{t("workerDetail.skills")}</TabsTrigger>
        </TabsList>

        <TabsContent value="sessions" className="mt-6">
          {sessionGroups.length === 0 && (
            <EmptyState
              title={t("executions.noExecutions")}
            />
          )}
          <div className="space-y-3">
            {sessionGroups.map((group) => {
              const latest = group[0]
              const oldest = group[group.length - 1]
              const isRunning = latest.status === "running"
              return (
                <Card
                  key={latest.session_id}
                  className={isRunning ? "border-l-2 border-l-primary" : ""}
                >
                  <CardContent className="flex items-center justify-between py-4">
                    <div>
                      <Link
                        to={`/sessions/${latest.session_id}`}
                        className="font-mono text-sm text-primary hover:underline"
                      >
                        {latest.session_id.slice(0, 8)}...
                      </Link>
                      <p className="text-xs text-muted-foreground mt-1">
                        {oldest.started_at ? new Date(oldest.started_at).toLocaleString() : "-"}
                        {oldest.trigger_input && ` · ${oldest.trigger_input.slice(0, 50)}${oldest.trigger_input.length > 50 ? "..." : ""}`}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {t("executions.turnCount", { count: group.length })}
                      </p>
                    </div>
                    <StatusBadge status={latest.status} />
                  </CardContent>
                </Card>
              )
            })}
          </div>

          <PaginationControls page={page} totalPages={totalPages} onPageChange={setPage} />
        </TabsContent>

        <TabsContent value="info" className="mt-6">
          <Card>
            <CardHeader>
              <CardTitle>{t("workerDetail.workerInfo")}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-3 text-sm">
                <span className="text-muted-foreground">{t("workerDetail.id")}</span>
                <span className="font-mono">{worker.id}</span>

                <span className="text-muted-foreground">{t("workerDetail.workDir")}</span>
                <span className="font-mono">{worker.work_dir}</span>

                <span className="text-muted-foreground">{t("workerDetail.created")}</span>
                <span className="font-mono">{new Date(worker.created_at).toLocaleString()}</span>
              </div>

              <div className="mt-6">
                <div className="flex items-center gap-2 mb-2">
                  <span className="text-sm font-medium">{t("workerDetail.memory")}</span>
                  {!isEditingMemory && (
                    <button
                      className="text-muted-foreground hover:text-foreground transition-colors"
                      onClick={() => { setEditMemory(worker.memory); setIsEditingMemory(true) }}
                    >
                      <Pencil className="h-3 w-3" />
                    </button>
                  )}
                </div>
                {isEditingMemory ? (
                  <div className="space-y-2">
                    <Textarea
                      value={editMemory}
                      onChange={(e) => setEditMemory(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Escape") { setIsEditingMemory(false); setEditMemory(worker.memory) }
                      }}
                      autoFocus
                      rows={8}
                      className="text-sm font-mono"
                    />
                    <div className="flex gap-2">
                      <Button size="sm" onClick={async () => {
                        await updateWorker.mutateAsync({ id: id!, data: { memory: editMemory } })
                        setIsEditingMemory(false)
                      }}>{t("common.save")}</Button>
                      <Button size="sm" variant="outline" onClick={() => {
                        setIsEditingMemory(false)
                        setEditMemory(worker.memory)
                      }}>{t("common.cancel")}</Button>
                    </div>
                  </div>
                ) : (
                  worker.memory ? (
                    <pre className="whitespace-pre-wrap text-sm bg-secondary rounded-lg p-4 font-mono">
                      {worker.memory}
                    </pre>
                  ) : (
                    <p className="text-muted-foreground text-sm">{t("workerDetail.noMemory")}</p>
                  )
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="skills" className="mt-6">
          <Card>
            <CardContent className="py-4 px-6">
              {workerSkills.length === 0 ? (
                <p className="text-muted-foreground text-sm">{t("skills.noSkills")}</p>
              ) : (
                workerSkills.map((skill) => (
                  <WorkerSkillRow key={skill.name} skill={skill} workerId={id!} />
                ))
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </FadeIn>
  )
}
