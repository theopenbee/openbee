import { useState, useCallback, useEffect, useRef, useMemo } from "react"
import { cn } from "@/lib/utils"
import type { Worker } from "@/lib/types"

type MentionWorker = Pick<Worker, "id" | "name">

interface MentionTextareaProps {
  value: string
  onChange: (value: string) => void
  onKeyDown?: (e: React.KeyboardEvent<HTMLTextAreaElement>) => void
  onPaste?: (e: React.ClipboardEvent<HTMLTextAreaElement>) => void
  workers: MentionWorker[]
  placeholder?: string
  disabled?: boolean
  textareaRef?: React.RefObject<HTMLTextAreaElement | null>
  className?: string
}

type MentionState = {
  query: string
  triggerIndex: number
  activeIndex: number
}

function detectMention(value: string, caretPos: number): Omit<MentionState, "activeIndex"> | null {
  const textBefore = value.slice(0, caretPos)
  const atIndex = textBefore.lastIndexOf("@")
  if (atIndex === -1) return null

  const fragment = textBefore.slice(atIndex + 1)
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
  const blurTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  useEffect(() => () => clearTimeout(blurTimerRef.current), [])

  const filteredWorkers = useMemo(() => {
    if (!mentionState) return []
    const q = mentionState.query.toLowerCase()
    return workers.filter(w => w.name.toLowerCase().startsWith(q)).slice(0, 8)
  }, [mentionState?.query, workers])

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      const newValue = e.target.value
      onChange(newValue)

      const caret = e.target.selectionStart ?? newValue.length
      const detected = detectMention(newValue, caret)

      if (detected) {
        const q = detected.query.toLowerCase()
        const hasMatch = workers.some(w => w.name.toLowerCase().startsWith(q))
        if (hasMatch) {
          setMentionState({ ...detected, activeIndex: 0 })
        } else {
          setMentionState(null)
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

      // requestAnimationFrame defers until after React flushes the controlled value,
      // so setSelectionRange lands on the updated DOM
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
    clearTimeout(blurTimerRef.current)
    blurTimerRef.current = setTimeout(() => setMentionState(null), 150)
  }, [])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (mentionState && filteredWorkers.length > 0) {
        if (e.key === "ArrowDown") {
          e.preventDefault()
          setMentionState(s =>
            s ? { ...s, activeIndex: Math.min(s.activeIndex + 1, filteredWorkers.length - 1) } : null
          )
          return
        }
        if (e.key === "ArrowUp") {
          e.preventDefault()
          setMentionState(s =>
            s ? { ...s, activeIndex: Math.max(s.activeIndex - 1, 0) } : null
          )
          return
        }
        if (e.key === "Enter") {
          e.preventDefault() // prevent message send while panel is open
          handleSelect(filteredWorkers[mentionState.activeIndex])
          return
        }
        if (e.key === "Escape") {
          e.preventDefault()
          setMentionState(null)
          return
        }
      }
      onKeyDown?.(e)
    },
    [mentionState, filteredWorkers, handleSelect, onKeyDown]
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
