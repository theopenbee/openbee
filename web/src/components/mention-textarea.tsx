// web/src/components/mention-textarea.tsx
import { useState, useCallback, useMemo } from "react"
import { cn } from "@/lib/utils"

interface MentionWorker {
  id: string
  name: string
}

interface MentionTextareaProps {
  value: string
  onChange: (value: string) => void
  onKeyDown?: (e: React.KeyboardEvent<HTMLTextAreaElement>) => void
  onPaste?: (e: React.ClipboardEvent<HTMLTextAreaElement>) => void
  workers: MentionWorker[]
  placeholder?: string
  disabled?: boolean
  textareaRef?: React.RefObject<HTMLTextAreaElement>
  className?: string
}

type MentionState = {
  query: string        // text typed after @
  triggerIndex: number // index of the @ character in value
  activeIndex: number  // keyboard-highlighted candidate index
}

export function MentionTextarea({
  value,
  onChange,
  onKeyDown,
  onPaste,
  workers,
  placeholder,
  disabled,
  textareaRef,
  className,
}: MentionTextareaProps) {
  const [mentionState, setMentionState] = useState<MentionState | null>(null)

  return (
    <div className="relative">
      <textarea
        ref={textareaRef}
        value={value}
        placeholder={placeholder}
        disabled={disabled}
        className={className}
      />
    </div>
  )
}
