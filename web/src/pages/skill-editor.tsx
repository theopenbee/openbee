import { useState, useEffect } from "react"
import { useParams, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useSkill, useSkillVersionContent, useCreateSkillVersion } from "@/hooks/use-skills"
import { Button } from "@/components/ui/button"

export function SkillEditor() {
  const { t } = useTranslation()
  const { name } = useParams<{ name: string }>()
  const navigate = useNavigate()

  const { data: skill } = useSkill(name!)
  const { data: contentData } = useSkillVersionContent(name!, skill?.global_version ?? "")
  const createVersion = useCreateSkillVersion(name!)

  const [content, setContent] = useState("")

  useEffect(() => {
    if (contentData?.content !== undefined) {
      setContent(contentData.content)
    }
  }, [contentData?.content])

  const handleSave = async () => {
    await createVersion.mutateAsync(content)
    navigate(`/skills/${name}`)
  }

  return (
    <div
      className="-mx-6 -mt-8 flex flex-col border-t border-border"
      style={{ height: "calc(100vh - 64px)" }}
    >
      {/* Header bar */}
      <div className="flex items-center justify-between px-6 py-3 border-b border-border bg-card shrink-0">
        <h1 className="text-sm font-medium">
          {t("skills.editor.title", { name })}
        </h1>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => navigate(`/skills/${name}`)}>
            {t("common.cancel")}
          </Button>
          <Button size="sm" onClick={handleSave} disabled={createVersion.isPending || !content}>
            {t("common.save")}
          </Button>
        </div>
      </div>

      {createVersion.error && (
        <p className="text-destructive px-6 py-2 text-sm shrink-0">{createVersion.error.message}</p>
      )}

      {/* Editor area */}
      <textarea
        value={content}
        onChange={(e) => setContent(e.target.value)}
        placeholder={t("skills.editor.placeholder")}
        className="flex-1 resize-none px-6 py-4 font-mono text-sm bg-background focus:outline-none"
        spellCheck={false}
      />
    </div>
  )
}
