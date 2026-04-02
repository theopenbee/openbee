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
import { tokenParam } from "@/lib/auth"
import { basename, isImage } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { ChevronLeft, Paperclip, Send } from "lucide-react"
import type { ChatMessage } from "@/lib/types"


const AttachmentPreview = memo(function AttachmentPreview({ sessionId, mediaPath }: { sessionId: string; mediaPath: string }) {
  const filename = basename(mediaPath)
  const url = `${config.apiUrl}/local/sessions/${sessionId}/media/${encodeURIComponent(filename)}${tokenParam()}`
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
  const [pendingMediaPaths, setPendingMediaPaths] = useState<string[]>([])
  const [uploadError, setUploadError] = useState<string | null>(null)
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

  const handleSend = useCallback(async () => {
    const content = input.trim()
    if (!content && pendingMediaPaths.length === 0) return

    const paths = [...pendingMediaPaths]
    const userMsg: ChatMessage = {
      role: "user",
      content,
      media_paths: paths.length > 0 ? paths : undefined,
      ts: Date.now(),
    }
    setLocalMessages((prev) => [...prev, userMsg])
    setInput("")
    setPendingMediaPaths([])
    setUploadError(null)
    setIsProcessing(true)

    try {
      await sendMessage.mutateAsync({
        content: content || " ",
        mediaPaths: paths.length > 0 ? paths : undefined,
      })
    } catch {
      setLocalMessages((prev) => prev.filter((m) => m !== userMsg))
      setPendingMediaPaths(paths)
      setIsProcessing(false)
    }
  }, [input, pendingMediaPaths, sendMessage])

  const handleFileChange = useCallback(async (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files ?? [])
    if (files.length === 0) return
    e.target.value = ""
    setUploadError(null)
    const results = await Promise.allSettled(
      files.map((file) => api.localChat.uploadMedia(sessionId, file))
    )
    const succeeded = results.filter(
      (r): r is PromiseFulfilledResult<{ path: string }> => r.status === "fulfilled"
    )
    const failedCount = results.length - succeeded.length
    if (failedCount > 0) {
      setUploadError(`${failedCount} 个文件上传失败，请重试`)
    }
    setPendingMediaPaths((prev) => [...prev, ...succeeded.map((r) => r.value.path)])
  }, [sessionId])

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
                  {msg.media_paths && msg.media_paths.map((p) => (
                    <AttachmentPreview key={p} sessionId={sessionId} mediaPath={p} />
                  ))}
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
        {uploadError && (
          <p className="text-xs text-destructive mb-1">{uploadError}</p>
        )}
        {pendingMediaPaths.length > 0 && (
          <div className="flex flex-wrap gap-1 mb-1">
            {pendingMediaPaths.map((p) => (
              <span
                key={p}
                className="flex items-center gap-1 text-xs text-muted-foreground font-mono bg-muted px-2 py-0.5 rounded"
              >
                📎 {basename(p)}
                <button
                  type="button"
                  className="ml-1 hover:text-destructive"
                  onClick={() =>
                    setPendingMediaPaths((prev) => prev.filter((path) => path !== p))
                  }
                >
                  ×
                </button>
              </span>
            ))}
          </div>
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
            multiple
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
