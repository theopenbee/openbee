import { useState } from "react"
import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useLocalSessions, useCreateSession, useDeleteSession } from "@/hooks/use-local-chat"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Dialog, DialogContent, DialogHeader } from "@/components/ui/dialog"
import { Card, CardContent } from "@/components/ui/card"

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

  if (isLoading) return <p>Loading...</p>

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold">{t("localChat.title")}</h1>
        <Button onClick={() => setDialogOpen(true)}>{t("localChat.newChat")}</Button>
      </div>

      {sessions.length === 0 ? (
        <p className="text-muted-foreground">{t("localChat.emptyState")}</p>
      ) : (
        <div className="space-y-2">
          {sessions.map((sess) => (
            <Card
              key={sess.id}
              className="cursor-pointer hover:bg-accent transition-colors"
              onClick={() => navigate(`/local-chat/${sess.id}`)}
            >
              <CardContent className="py-3 px-4 flex items-center justify-between">
                <div>
                  <p className="font-medium">{sess.name}</p>
                  <p className="text-xs text-muted-foreground">
                    {new Date(sess.updated_at).toLocaleString()}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={(e) => {
                    e.stopPropagation()
                    deleteSession.mutate(sess.id)
                  }}
                >
                  ✕
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
    </div>
  )
}
