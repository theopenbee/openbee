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

  const handleSelect = useCallback(
    (worker: MentionWorker) => {
      if (!mentionState) return
      const textarea = textareaRef?.current
      const caret = textarea?.selectionStart ?? value.length

      const before = value.slice(0, mentionState.triggerIndex)
      const after = value.slice(caret)
      const inserted = `@${worker.name} `
      const newValue = before + inserted + after

      onChange(newValue)
      setMentionState(null)

      // Move caret to end of inserted text
      requestAnimationFrame(() => {
        if (textarea) {
          const pos = mentionState.triggerIndex + inserted.length
          textarea.setSelectionRange(pos, pos)
          textarea.focus()
        }
      })
    },
    [mentionState, value, onChange, textareaRef]
  )

  const handleBlur = useCallback(() => {
    // Delay so onMouseDown on a candidate fires before the panel closes
    setTimeout(() => setMentionState(null), 150)
  }, [])

  // Temporary stub — replaced in Task 5
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      onKeyDown?.(e)
    },
    [onKeyDown]
  )

  return (
    <div className="relative">
      {mentionState && filteredWorkers.length > 0 && (
        <MentionPanel
          workers={filteredWorkers}
          activeIndex={mentionState.activeIndex}
          onSelect={handleSelect}
        />
      )}
      <textarea
        ref={textareaRef}
        value={value}
        onChange={handleChange}
        onKeyDown={handleKeyDown}
        onBlur={handleBlur}
        onPaste={onPaste}
        placeholder={placeholder}
        disabled={disabled}
        className={className}
      />
    </div>
  )
}

function MentionPanel({
  workers,
  activeIndex,
  onSelect,
}: {
  workers: MentionWorker[]
  activeIndex: number
  onSelect: (worker: MentionWorker) => void
}) {
  return (
    <div
      className="absolute bottom-full left-0 right-0 mb-1 z-50 rounded-2xl border border-border/70 bg-popover shadow-lg overflow-hidden"
    >
      <ul role="listbox" className="max-h-[280px] overflow-y-auto py-1">
        {workers.map((worker, index) => (
          <li
            key={worker.id}
            role="option"
            aria-selected={index === activeIndex}
            className={cn(
              "flex items-center px-4 py-2.5 text-sm cursor-pointer transition-colors",
              index === activeIndex
                ? "bg-accent text-accent-foreground"
                : "hover:bg-accent/50"
            )}
            onMouseDown={(e) => {
              // Prevent textarea blur before selection fires
              e.preventDefault()
              onSelect(worker)
            }}
          >
            <span className="font-medium truncate">{worker.name}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}
