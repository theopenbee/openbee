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

function detectMention(value: string, caretPos: number): Omit<MentionState, "activeIndex"> | null {
  const textBefore = value.slice(0, caretPos)
  const atIndex = textBefore.lastIndexOf("@")
  if (atIndex === -1) return null

  const fragment = textBefore.slice(atIndex + 1)
  // If there's a space or newline between @ and caret, the mention is finished
  if (fragment.includes(" ") || fragment.includes("\n")) return null

  return { query: fragment, triggerIndex: atIndex }
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

  const filteredWorkers = useMemo(() => {
    if (!mentionState) return []
    return workers
      .filter(w => w.name.toLowerCase().startsWith(mentionState.query.toLowerCase()))
      .slice(0, 8)
  }, [mentionState, workers])

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      const newValue = e.target.value
      onChange(newValue)

      const caret = e.target.selectionStart ?? newValue.length
      const detected = detectMention(newValue, caret)

      if (detected) {
        const matched = workers.filter(w =>
          w.name.toLowerCase().startsWith(detected.query.toLowerCase())
        )
        if (matched.length > 0) {
          setMentionState({ ...detected, activeIndex: 0 })
        } else {
          setMentionState(null) // no match → auto-close
        }
      } else {
        setMentionState(null)
      }
    },
    [onChange, workers]
  )

  return (
    <div className="relative">
      <textarea
        ref={textareaRef}
        value={value}
        onChange={handleChange}
        onPaste={onPaste}
        placeholder={placeholder}
        disabled={disabled}
        className={className}
      />
    </div>
  )
}
