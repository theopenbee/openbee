import { useState } from "react"
import { Link } from "react-router-dom"
import { useTranslation, Trans } from "react-i18next"
import { useSkills, useCreateSkill, useDeleteSkill, useAdoptSkill } from "@/hooks/use-skills"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  DialogFooter,
  DialogDescription,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { EmptyState } from "@/components/empty-state"

export function Skills() {
  const { t } = useTranslation()
  const { data: skills = [], isLoading } = useSkills()
  const createSkill = useCreateSkill()
  const deleteSkill = useDeleteSkill()
  const adoptSkill = useAdoptSkill()

  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [content, setContent] = useState("")

  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)

  const error =
    createSkill.error?.message ||
    deleteSkill.error?.message ||
    adoptSkill.error?.message ||
    ""

  const handleCreate = async () => {
    await createSkill.mutateAsync({ name, description, content })
    setCreateOpen(false)
    setName("")
    setDescription("")
    setContent("")
  }

  const handleDeleteConfirm = async () => {
    if (!deleteTarget) return
    await deleteSkill.mutateAsync(deleteTarget)
    setDeleteTarget(null)
  }

  return (
    <FadeIn>
      <PageHeader
        title={t("skills.title")}
        actions={
          <Dialog open={createOpen} onOpenChange={setCreateOpen}>
            <DialogTrigger render={<Button size="sm" />}>
              {t("skills.createSkill")}
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>{t("skills.createSkill")}</DialogTitle>
              </DialogHeader>
              <div className="space-y-4 py-2">
                <div className="space-y-1">
                  <Label>{t("workers.form.name")}</Label>
                  <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. code-review" />
                </div>
                <div className="space-y-1">
                  <Label>{t("workers.form.description")}</Label>
                  <Input value={description} onChange={(e) => setDescription(e.target.value)} placeholder={t("workers.form.descriptionPlaceholder")} />
                </div>
                <div className="space-y-1">
                  <Label>SKILL.md</Label>
                  <Textarea
                    value={content}
                    onChange={(e) => setContent(e.target.value)}
                    placeholder={t("skills.editor.placeholder")}
                    rows={8}
                    className="font-mono text-sm"
                  />
                </div>
              </div>
              <DialogFooter>
                <Button variant="outline" onClick={() => setCreateOpen(false)}>{t("common.cancel")}</Button>
                <Button onClick={handleCreate} disabled={!name || !content || createSkill.isPending}>
                  {t("common.create")}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        }
      />

      {error && <p className="text-destructive mb-4 text-sm">{error}</p>}

      {!isLoading && skills.length === 0 && (
        <EmptyState title={t("skills.noSkills")} />
      )}

      <div className="space-y-3">
        {skills.map((skill) => (
          <Card key={skill.name}>
            <CardContent className="flex items-center justify-between py-4">
              <div className="space-y-1">
                <div className="flex items-center gap-2">
                  <Link to={`/skills/${skill.name}`} className="font-medium text-primary hover:underline">
                    {skill.name}
                  </Link>
                  <Badge variant={skill.source === "managed" ? "default" : "secondary"}>
                    {t(`skills.source.${skill.source}`)}
                  </Badge>
                </div>
                {skill.active_version && (
                  <p className="text-xs text-muted-foreground font-mono">{skill.active_version}</p>
                )}
              </div>
              <div className="flex items-center gap-2">
                {skill.source === "external" && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => adoptSkill.mutate(skill.name)}
                    disabled={adoptSkill.isPending}
                  >
                    {t("skills.adopt")}
                  </Button>
                )}
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-destructive hover:text-destructive"
                  onClick={() => setDeleteTarget(skill.name)}
                >
                  {t("common.delete")}
                </Button>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Delete confirm dialog */}
      <Dialog open={!!deleteTarget} onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("skills.deleteDialog.title")}</DialogTitle>
            <DialogDescription>
              <Trans
                i18nKey="skills.deleteDialog.confirm"
                values={{ name: deleteTarget }}
                components={{ strong: <strong /> }}
              />
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>{t("common.cancel")}</Button>
            <Button variant="destructive" onClick={handleDeleteConfirm} disabled={deleteSkill.isPending}>
              {t("common.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </FadeIn>
  )
}
