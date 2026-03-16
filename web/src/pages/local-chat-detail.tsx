import { useState, useRef, useEffect, useCallback } from "react"
import { useParams, Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  useLocalMessages,
  useSendMessage,
  useLocalChatStream,
} from "@/hooks/use-local-chat"
import { api } from "@/lib/api"
import { Button } from "@/components/ui/button"
import type { ChatMessage } from "@/lib/types"

export function LocalChatDetail() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const sessionId = id!

  const { data: history = [] } = useLocalMessages(sessionId)
  const sendMessage = useSendMessage(sessionId)

  const [localMessages, setLocalMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState("")
  const [isProcessing, setIsProcessing] = useState(false)
  const [pendingMediaPath, setPendingMediaPath] = useState<string | undefined>()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const bottomRef = useRef<HTMLDivElement>(null)

  // Sync history into local state when it loads/refetches
  useEffect(() => {
    setLocalMessages(history)
  }, [history])

  // SSE: append new replies as they arrive
  const handleReply = useCallback((msg: ChatMessage) => {
    setLocalMessages((prev) => [...prev, msg])
    setIsProcessing(false)
  }, [])
  useLocalChatStream(sessionId, handleReply)

  // Auto-scroll to bottom on new messages
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" })
  }, [localMessages, isProcessing])

  const handleSend = async () => {
    const content = input.trim()
    if (!content && !pendingMediaPath) return

    const userMsg: ChatMessage = { role: "user", content, ts: Date.now() }
    setLocalMessages((prev) => [...prev, userMsg])
    setInput("")
    setIsProcessing(true)

    await sendMessage.mutateAsync({ content: content || " ", mediaPath: pendingMediaPath })
    setPendingMediaPath(undefined)
  }

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const { path } = await api.localChat.uploadMedia(sessionId, file)
    setPendingMediaPath(path)
  }

  return (
    <div className="flex flex-col h-[calc(100vh-8rem)]">
      {/* Header */}
      <div className="flex items-center gap-3 mb-4">
        <Link to="/local-chat" className="text-sm text-muted-foreground hover:underline">
          ← {t("localChat.title")}
        </Link>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto space-y-3 pb-2">
        {localMessages.map((msg, i) => (
          <div
            key={i}
            className={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}
          >
            <div
              className={`max-w-[70%] rounded-xl px-4 py-2 text-sm whitespace-pre-wrap ${
                msg.role === "user"
                  ? "bg-primary text-primary-foreground rounded-br-sm"
                  : "bg-muted rounded-bl-sm"
              }`}
            >
              {msg.role === "bee" && (
                <p className="text-xs text-muted-foreground mb-1">🤖 bee</p>
              )}
              {msg.content}
            </div>
          </div>
        ))}

        {isProcessing && (
          <div className="flex justify-start">
            <div className="bg-muted rounded-xl px-4 py-2 text-sm text-muted-foreground">
              {t("localChat.processing")}
            </div>
          </div>
        )}
        <div ref={bottomRef} />
      </div>

      {/* Input */}
      <div className="border-t pt-3">
        {pendingMediaPath && (
          <p className="text-xs text-muted-foreground mb-1 truncate">
            📎 {pendingMediaPath}
          </p>
        )}
        <div className="flex gap-2 items-end">
          <textarea
            className="flex-1 min-h-[40px] max-h-[120px] resize-none rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
            placeholder={t("localChat.inputPlaceholder")}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault()
                handleSend()
              }
            }}
          />
          <input
            ref={fileInputRef}
            type="file"
            className="hidden"
            onChange={handleFileChange}
          />
          <Button variant="outline" size="icon" onClick={() => fileInputRef.current?.click()}>
            📎
          </Button>
          <Button onClick={handleSend} disabled={sendMessage.isPending}>
            {t("localChat.send")}
          </Button>
        </div>
      </div>
    </div>
  )
}
