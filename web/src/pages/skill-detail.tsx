import { useParams, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useSkill, useSkillVersionContent, useSetGlobalVersion } from "@/hooks/use-skills"
import { sortVersionsDescending } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { SkeletonPage } from "@/components/skeleton-loader"

export function SkillDetail() {
  const { t } = useTranslation()
  const { name } = useParams<{ name: string }>()
  const navigate = useNavigate()

  const { data: skill, isLoading } = useSkill(name!)
  const { data: contentData } = useSkillVersionContent(name!, skill?.global_version ?? "")
  const setGlobalVersion = useSetGlobalVersion(name!)

  if (isLoading || !skill) return <SkeletonPage />

  const versions = sortVersionsDescending(Object.keys(skill.versions)).map(
    (v) => [v, skill.versions[v]] as const
  )

  return (
    <FadeIn>
      <PageHeader
        title={name!}
        subtitle={skill.description || t("common.noDescription")}
        actions={
          <Button size="sm" onClick={() => navigate(`/skills/${name}/edit`)}>
            {t("skills.newVersion")}
          </Button>
        }
      />

      {setGlobalVersion.error && (
        <p className="text-destructive mb-4 text-sm">{setGlobalVersion.error.message}</p>
      )}

      <div className="space-y-6">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm font-medium">
              {t("skills.currentContent")}
              <span className="ml-2 font-mono text-xs text-muted-foreground">{skill.global_version}</span>
            </CardTitle>
          </CardHeader>
          <CardContent>
            {contentData?.content ? (
              <pre className="whitespace-pre-wrap text-sm font-mono bg-secondary rounded-lg p-4 max-h-96 overflow-y-auto">
                {contentData.content}
              </pre>
            ) : (
              <p className="text-muted-foreground text-sm">—</p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm font-medium">{t("skills.versions")}</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("skills.globalVersion")}</TableHead>
                  <TableHead>{t("executionDetail.started")}</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {versions.map(([version, entry]) => (
                  <TableRow key={version}>
                    <TableCell className="font-mono text-sm">
                      {version}
                      {version === skill.global_version && (
                        <Badge className="ml-2" variant="default">{t("skills.globalVersion")}</Badge>
                      )}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {new Date(entry.created_at).toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right">
                      {version !== skill.global_version && (
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => setGlobalVersion.mutate(version)}
                          disabled={setGlobalVersion.isPending}
                        >
                          {t("skills.setGlobal")}
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
    </FadeIn>
  )
}
