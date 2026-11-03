import { useState, useCallback, useEffect, useId, useRef, useMemo } from "react"
import { WorkerAvatar } from "@/components/worker-avatar"
import { cn, isImeComposing } from "@/lib/utils"
import type { Worker } from "@/lib/types"

type MentionWorker = Pick<Worker, "id" | "name" | "status">

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
  ariaLabel?: string
  rows?: number
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
  ariaLabel,
  rows = 1,
}: MentionTextareaProps) {
  const listId = useId()
  const [mentionState, setMentionState] = useState<MentionState | null>(null)
  const blurTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  useEffect(() => () => clearTimeout(blurTimerRef.current), [])

  const query = mentionState?.query
  const filteredWorkers = useMemo(() => {
    if (query === undefined) return []
    const q = query.toLowerCase()
    return workers.filter((w) => w.name.toLowerCase().includes(q)).slice(0, 8)
  }, [query, workers])

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      const newValue = e.target.value
      onChange(newValue)

      const caret = e.target.selectionStart ?? newValue.length
      const detected = detectMention(newValue, caret)

      setMentionState(detected ? { ...detected, activeIndex: 0 } : null)
    },
    [onChange]
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
    clearTimeout(blurTimerRef.current)
    blurTimerRef.current = setTimeout(() => setMentionState(null), 150)
  }, [])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (mentionState && filteredWorkers.length > 0 && !isImeComposing(e)) {
        if (e.key === "ArrowDown" || e.key === "ArrowUp") {
          e.preventDefault()
          const delta = e.key === "ArrowDown" ? 1 : -1
          setMentionState((s) =>
            s ? { ...s, activeIndex: Math.min(Math.max(s.activeIndex + delta, 0), filteredWorkers.length - 1) } : null
          )
          return
        }
        if (e.key === "Enter") {
          e.preventDefault()
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

  const open = Boolean(mentionState && filteredWorkers.length > 0)
  const activeId = open && mentionState ? `${listId}-${mentionState.activeIndex}` : undefined

  return (
    <div className="relative">
      {open && mentionState && (
        <MentionPanel
          id={listId}
          workers={filteredWorkers}
          activeIndex={mentionState.activeIndex}
          onSelect={handleSelect}
        />
      )}
      <textarea
        ref={textareaRef}
        value={value}
        rows={rows}
        onChange={handleChange}
        onKeyDown={handleKeyDown}
        onBlur={handleBlur}
        onPaste={onPaste}
        placeholder={placeholder}
        disabled={disabled}
        aria-label={ariaLabel}
        aria-autocomplete="list"
        aria-controls={open ? listId : undefined}
        aria-activedescendant={activeId}
        className={className}
      />
    </div>
  )
}

function MentionPanel({
  id,
  workers,
  activeIndex,
  onSelect,
}: {
  id: string
  workers: MentionWorker[]
  activeIndex: number
  onSelect: (worker: MentionWorker) => void
}) {
  return (
    <div className="absolute bottom-full left-0 z-50 mb-3 w-64 max-w-[calc(100vw-2rem)] animate-fade-in overflow-hidden rounded-panel border border-border bg-surface-raised p-2 shadow-floating">
      <ul id={id} role="listbox" className="max-h-72 overflow-y-auto">
        {workers.map((worker, index) => (
          <li
            key={worker.id}
            id={`${id}-${index}`}
            role="option"
            aria-selected={index === activeIndex}
            className={cn(
              "flex min-h-11 cursor-pointer items-center gap-3 rounded-control px-3 text-sm text-foreground transition-colors",
              index === activeIndex ? "bg-surface-muted" : "hover:bg-surface-muted"
            )}
            onMouseDown={(e) => {
              e.preventDefault()
              onSelect(worker)
            }}
          >
            <WorkerAvatar workerId={worker.id} name={worker.name} status={worker.status} size="sm" />
            <span className="truncate">{worker.name}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}
