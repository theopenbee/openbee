import { useCallback, useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { ArrowUp, Paperclip, X } from "lucide-react"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { FileCategoryIcon } from "@/components/chat/attachment-preview"
import { MentionTextarea } from "@/components/mention-textarea"
import composerStyles from "@/components/chat-composer.module.css"
import { useMediaQuery } from "@/hooks/use-media-query"
import { api } from "@/lib/api"
import { CHAT_COLUMN } from "@/lib/styles"
import type { Worker } from "@/lib/types"
import { basename, cn, isImeComposing } from "@/lib/utils"

const TOUCH_ONLY_QUERY = "(hover: none) and (pointer: coarse)"

const COMPOSER_MAX_HEIGHT = 160

const TEXTAREA_CLASS =
  "block max-h-40 min-h-11 w-full resize-none bg-transparent px-2 py-2.5 text-base leading-6 text-foreground placeholder:text-muted disabled:cursor-not-allowed disabled:opacity-60 sm:min-h-12 sm:px-3 sm:py-3 sm:text-sm"

interface ComposerProps {
  placeholder: string
  disabled?: boolean
  mentions?: Worker[]
  onSend: (text: string, mediaPaths: string[]) => Promise<void>
}

export function Composer({ placeholder, disabled, mentions, onSend }: ComposerProps) {
  const { t } = useTranslation()

  const [input, setInput] = useState("")
  const [isSending, setIsSending] = useState(false)
  const [pendingMediaPaths, setPendingMediaPaths] = useState<string[]>([])
  const [uploadError, setUploadError] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    const textarea = textareaRef.current
    if (!textarea) return
    textarea.style.height = "0px"
    textarea.style.height = `${Math.min(textarea.scrollHeight, COMPOSER_MAX_HEIGHT)}px`
  }, [input])

  const handleSend = useCallback(async () => {
    const content = input.trim()
    if (!content && pendingMediaPaths.length === 0) return

    const paths = [...pendingMediaPaths]

    setInput("")
    setPendingMediaPaths([])
    setUploadError(null)
    setIsSending(true)

    try {
      await onSend(content, paths)
    } catch {
      setPendingMediaPaths((prev) => [...paths, ...prev])
    } finally {
      setIsSending(false)
    }
  }, [input, pendingMediaPaths, onSend])

  const uploadFiles = useCallback(async (files: File[]) => {
    if (files.length === 0) return

    setUploadError(null)

    const results = await Promise.allSettled(
      files.map((file) => api.localChat.uploadMedia(file))
    )
    const succeeded = results.filter(
      (result): result is PromiseFulfilledResult<{ path: string }> => result.status === "fulfilled"
    )
    const failedCount = results.length - succeeded.length

    if (failedCount > 0) {
      setUploadError(t("localChat.uploadError", { count: failedCount }))
    }

    setPendingMediaPaths((prev) => [...prev, ...succeeded.map((result) => result.value.path)])
  }, [t])

  const handleFileChange = useCallback(async (event: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? [])
    event.target.value = ""
    await uploadFiles(files)
  }, [uploadFiles])

  const handlePaste = useCallback(async (event: React.ClipboardEvent<HTMLTextAreaElement>) => {
    const files = Array.from(event.clipboardData.items)
      .map((item) => (item.type.startsWith("image/") ? item.getAsFile() : null))
      .filter((file): file is File => file !== null)
    if (files.length === 0) return

    event.preventDefault()
    await uploadFiles(files)
  }, [uploadFiles])

  const isTouchOnly = useMediaQuery(TOUCH_ONLY_QUERY)

  const handleKeyDown = useCallback((event: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key !== "Enter" || event.shiftKey || isImeComposing(event)) return
    if (isTouchOnly && !event.metaKey && !event.ctrlKey) return
    event.preventDefault()
    void handleSend()
  }, [handleSend, isTouchOnly])

  const canSend = input.trim().length > 0 || pendingMediaPaths.length > 0
  const controlSize = isTouchOnly ? "md" : "sm"

  return (
    <div className="shrink-0 bg-background">
      <div className={cn(CHAT_COLUMN, "pb-3 sm:pb-6")}>
        {uploadError && (
          <div className="mb-3">
            <Alert tone="danger" title={uploadError} open onDismiss={() => setUploadError(null)} />
          </div>
        )}

        <div
          className={cn(
            composerStyles.composer,
            "rounded-surface border border-border bg-surface p-2 transition-colors focus-within:border-border-strong"
          )}
        >
          {pendingMediaPaths.length > 0 && (
            <ul className={cn(composerStyles.files, "flex flex-wrap gap-2 px-2 pt-1 pb-2")} aria-label={t("localChat.attachments")}>
              {pendingMediaPaths.map((path) => (
                <li
                  key={path}
                  className="inline-flex min-h-8 max-w-full items-center gap-2 rounded-full border border-border bg-surface-muted py-1 pr-1 pl-3 text-xs text-foreground"
                >
                  <FileCategoryIcon filePath={path} className="text-secondary" />
                  <span className="min-w-0 max-w-40 truncate sm:max-w-56">{basename(path)}</span>
                  <button
                    type="button"
                    className="inline-grid size-6 shrink-0 place-items-center rounded-full text-secondary transition-colors hover:bg-surface hover:text-foreground"
                    aria-label={t("localChat.removeNamedAttachment", { name: basename(path) })}
                    onClick={() =>
                      setPendingMediaPaths((prev) => prev.filter((entry) => entry !== path))
                    }
                  >
                    <X size={16} strokeWidth={1.75} aria-hidden="true" />
                  </button>
                </li>
              ))}
            </ul>
          )}

          <div className={composerStyles.input}>
            {mentions ? (
              <MentionTextarea
                textareaRef={textareaRef}
                ariaLabel={t("localChat.messageLabel")}
                className={TEXTAREA_CLASS}
                placeholder={placeholder}
                value={input}
                onChange={setInput}
                onKeyDown={handleKeyDown}
                onPaste={handlePaste}
                workers={mentions}
                disabled={disabled}
              />
            ) : (
              <textarea
                ref={textareaRef}
                aria-label={t("localChat.messageLabel")}
                className={TEXTAREA_CLASS}
                placeholder={placeholder}
                rows={1}
                value={input}
                onChange={(event) => setInput(event.target.value)}
                onKeyDown={handleKeyDown}
                onPaste={handlePaste}
                disabled={disabled}
              />
            )}
          </div>

          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="hidden"
            onChange={handleFileChange}
          />
          <span className={composerStyles.attach}>
            <Button
              variant="ghost"
              size={controlSize}
              onClick={() => fileInputRef.current?.click()}
              aria-label={t("localChat.uploadFile")}
            >
              <Paperclip size={16} strokeWidth={1.75} aria-hidden="true" />
            </Button>
          </span>
          <span className={cn(composerStyles.hint, "truncate text-xs text-muted")}>
            {isTouchOnly ? t("localChat.composerHintTouch") : t("localChat.composerHint")}
          </span>
          <span className={composerStyles.send}>
            <Button
              size={controlSize}
              onClick={() => void handleSend()}
              disabled={!canSend || isSending}
              aria-label={t("localChat.send")}
            >
              <ArrowUp size={16} strokeWidth={1.75} aria-hidden="true" />
            </Button>
          </span>
        </div>
      </div>
    </div>
  )
}
