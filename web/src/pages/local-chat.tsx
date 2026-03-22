import { useState } from "react"
import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useLocalSessions, useCreateSession, useDeleteSession } from "@/hooks/use-local-chat"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Dialog, DialogContent, DialogHeader } from "@/components/ui/dialog"
import { Card, CardContent } from "@/components/ui/card"
import { X } from "lucide-react"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { EmptyState } from "@/components/empty-state"
import { SkeletonLine } from "@/components/skeleton-loader"

export function LocalChat() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data: sessions = [], isLoading } = useLocalSessions()
  const createSession = useCreateSession()
  const deleteSession = useDeleteSession()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [newName, setNewName] = useState("")

  const handleCreate = async () => {
    if (!newName.trim()) return
    const session = await createSession.mutateAsync(newName.trim())
    setNewName("")
    setDialogOpen(false)
    navigate(`/local-chat/${session.id}`)
  }

  return (
    <FadeIn>
      <PageHeader
        title={t("localChat.title")}
        actions={
          <Button onClick={() => setDialogOpen(true)}>{t("localChat.newChat")}</Button>
        }
      />

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="rounded-xl bg-card ring-1 ring-foreground/5 p-4">
              <SkeletonLine className="w-48" />
              <SkeletonLine className="w-32 mt-2" />
            </div>
          ))}
        </div>
      ) : sessions.length === 0 ? (
        <EmptyState
          title={t("emptyState.noSessions")}
          description={t("emptyState.noSessionsDesc")}
          action={
            <Button onClick={() => setDialogOpen(true)}>{t("localChat.newChat")}</Button>
          }
        />
      ) : (
        <div className="space-y-2">
          {sessions.map((sess) => (
            <Card
              key={sess.id}
              className="cursor-pointer hover:ring-1 hover:ring-primary/30 transition-all duration-200"
              onClick={() => navigate(`/local-chat/${sess.id}`)}
            >
              <CardContent className="py-3 px-4 flex items-center justify-between">
                <div>
                  <p className="font-medium">{sess.name}</p>
                  <p className="text-xs text-muted-foreground font-mono">
                    {new Date(sess.updated_at).toLocaleString()}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  className="text-muted-foreground hover:text-destructive hover:bg-destructive/10"
                  onClick={(e) => {
                    e.stopPropagation()
                    deleteSession.mutate(sess.id)
                  }}
                >
                  <X className="h-4 w-4" />
                </Button>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>{t("localChat.newChat")}</DialogHeader>
          <Input
            placeholder={t("localChat.sessionNamePlaceholder")}
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleCreate()}
            autoFocus
          />
          <Button onClick={handleCreate} disabled={!newName.trim() || createSession.isPending}>
            {t("localChat.newChat")}
          </Button>
        </DialogContent>
      </Dialog>
    </FadeIn>
  )
}
