import { memo, useState, useRef, useEffect, useCallback } from "react"
import { Streamdown } from "streamdown"
import { useParams, Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  useLocalMessages,
  useSendMessage,
  useLocalChatStream,
} from "@/hooks/use-local-chat"
import { api } from "@/lib/api"
import { config } from "@/lib/config"
import { getAccessToken } from "@/lib/auth"
import { Button } from "@/components/ui/button"
import { ChevronLeft, Paperclip, Send } from "lucide-react"
import type { ChatMessage } from "@/lib/types"

const IMAGE_EXTS = new Set(["jpg", "jpeg", "png", "gif", "webp", "svg", "bmp"])

function basename(filePath: string): string {
  return filePath.split("/").pop() ?? filePath
}

function isImage(filePath: string): boolean {
  const ext = filePath.split(".").pop()?.toLowerCase() ?? ""
  return IMAGE_EXTS.has(ext)
}

const AttachmentPreview = memo(function AttachmentPreview({ sessionId, mediaPath }: { sessionId: string; mediaPath: string }) {
  const filename = basename(mediaPath)
  const token = getAccessToken()
  const url = `${config.apiUrl}/local/sessions/${sessionId}/media/${encodeURIComponent(filename)}${token ? `?token=${encodeURIComponent(token)}` : ""}`
  if (isImage(mediaPath)) {
    return (
      <img
        src={url}
        alt={filename}
        className="max-w-full max-h-60 rounded-lg object-contain mb-1"
      />
    )
  }
  return (
    <a
      href={url}
      target="_blank"
      rel="noreferrer"
      className="flex items-center gap-1 text-xs underline opacity-80 mb-1 break-all"
    >
      <Paperclip className="h-3 w-3 shrink-0" />
      {filename}
    </a>
  )
})

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

  useEffect(() => {
    setLocalMessages(history)
  }, [history])

  const handleReply = useCallback((msg: ChatMessage) => {
    setLocalMessages((prev) => [...prev, msg])
    setIsProcessing(false)
  }, [])
  useLocalChatStream(sessionId, handleReply)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" })
  }, [localMessages, isProcessing])

  const handleSend = async () => {
    const content = input.trim()
    if (!content && !pendingMediaPath) return

    const userMsg: ChatMessage = { role: "user", content, media_path: pendingMediaPath, ts: Date.now() }
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
      <div className="flex items-center gap-2 mb-4">
        <Link
          to="/local-chat"
          className="flex items-center gap-1 text-sm text-muted-foreground hover:text-primary transition-colors"
        >
          <ChevronLeft className="h-4 w-4" />
          {t("localChat.title")}
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
              className={`max-w-[70%] rounded-xl px-4 py-2 text-sm ${
                msg.role === "user"
                  ? "whitespace-pre-wrap bg-primary text-primary-foreground rounded-br-sm"
                  : "bg-card ring-1 ring-foreground/10 rounded-bl-sm"
              }`}
            >
              {msg.role === "bee" && (
                <p className="text-xs text-muted-foreground mb-1">🐝 bee</p>
              )}
              {msg.role === "bee" ? (
                <div className="prose prose-sm dark:prose-invert max-w-none">
                  <Streamdown mode="static">{msg.content}</Streamdown>
                </div>
              ) : (
                <>
                  {msg.media_path && (
                    <AttachmentPreview sessionId={sessionId} mediaPath={msg.media_path} />
                  )}
                  {msg.content.trim() && <span>{msg.content}</span>}
                </>
              )}
            </div>
          </div>
        ))}

        {isProcessing && (
          <div className="flex justify-start">
            <div className="bg-card ring-1 ring-foreground/10 rounded-xl px-4 py-3">
              <div className="flex gap-1.5">
                <span className="w-2 h-2 rounded-full bg-primary animate-pulse-amber" style={{ animationDelay: "0ms" }} />
                <span className="w-2 h-2 rounded-full bg-primary animate-pulse-amber" style={{ animationDelay: "300ms" }} />
                <span className="w-2 h-2 rounded-full bg-primary animate-pulse-amber" style={{ animationDelay: "600ms" }} />
              </div>
            </div>
          </div>
        )}
        <div ref={bottomRef} />
      </div>

      {/* Input */}
      <div className="border-t border-border pt-3">
        {pendingMediaPath && (
          <p className="text-xs text-muted-foreground mb-1 truncate font-mono">
            📎 {basename(pendingMediaPath)}
          </p>
        )}
        <div className="flex gap-2 items-end bg-card rounded-xl ring-1 ring-foreground/10 p-2">
          <textarea
            className="flex-1 min-h-[40px] max-h-[120px] resize-none bg-transparent px-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none"
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
          <Button
            variant="ghost"
            size="icon"
            className="text-muted-foreground hover:text-primary"
            onClick={() => fileInputRef.current?.click()}
          >
            <Paperclip className="h-4 w-4" />
          </Button>
          <Button size="icon" onClick={handleSend} disabled={sendMessage.isPending}>
            <Send className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  )
}
