# Chat @ Mention UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `@` worker mention to the `/chat` input box — typing `@` shows a worker picker panel; selecting a worker inserts `@name ` into the textarea.

**Architecture:** Extract a `MentionTextarea` component that wraps the existing `<textarea>` with mention detection logic and a floating candidate panel. `LocalChat` passes the worker list in; the component manages all mention state internally. No new API endpoints — reuses the existing `useWorkers` hook.

**Tech Stack:** React 19, TypeScript, Tailwind CSS v4, existing `cn` utility, `useWorkers` hook from `@/hooks/use-workers`

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `web/src/components/mention-textarea.tsx` | **Create** | `MentionTextarea` component: detection logic, candidate panel, keyboard handling, text replacement |
| `web/src/pages/local-chat.tsx` | **Modify** | Replace `<textarea>` with `<MentionTextarea>`, add `useWorkers`, derive `workerList` |

---

### Task 1: Create `MentionTextarea` — scaffold and types

**Files:**
- Create: `web/src/components/mention-textarea.tsx`

- [ ] **Step 1: Create the file with types and empty component**

```tsx
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
```

- [ ] **Step 2: Type-check**

```bash
cd web && npx tsc -b --noEmit
```

Expected: No errors (or only pre-existing errors unrelated to the new file).

- [ ] **Step 3: Commit**

```bash
git add web/src/components/mention-textarea.tsx
git commit -m "feat: scaffold MentionTextarea component with types"
```

---

### Task 2: Implement `detectMention` and `onChange` handler

**Files:**
- Modify: `web/src/components/mention-textarea.tsx`

- [ ] **Step 1: Add `detectMention` function inside the component file (above the component)**

```ts
// Place this above the MentionTextarea function

function detectMention(value: string, caretPos: number): Omit<MentionState, "activeIndex"> | null {
  const textBefore = value.slice(0, caretPos)
  const atIndex = textBefore.lastIndexOf("@")
  if (atIndex === -1) return null

  const fragment = textBefore.slice(atIndex + 1)
  // If there's a space or newline between @ and caret, the mention is finished
  if (fragment.includes(" ") || fragment.includes("\n")) return null

  return { query: fragment, triggerIndex: atIndex }
}
```

- [ ] **Step 2: Add `filteredWorkers` derivation and `handleChange` inside `MentionTextarea`**

Replace the body of `MentionTextarea` with:

```tsx
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
          setMentionState(null) // no match → auto-close (user chose option A)
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
```

- [ ] **Step 3: Type-check**

```bash
cd web && npx tsc -b --noEmit
```

Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/components/mention-textarea.tsx
git commit -m "feat: add mention detection logic to MentionTextarea"
```

---

### Task 3: Implement candidate panel UI

**Files:**
- Modify: `web/src/components/mention-textarea.tsx`

- [ ] **Step 1: Add `MentionPanel` as an internal component at the bottom of the file**

Add this below `MentionTextarea`:

```tsx
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
```

- [ ] **Step 2: Wire `MentionPanel` into the `MentionTextarea` render**

Replace the `return` block in `MentionTextarea`:

```tsx
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
        onPaste={onPaste}
        placeholder={placeholder}
        disabled={disabled}
        className={className}
      />
    </div>
  )
```

Note: `handleSelect` is added in Task 4.

- [ ] **Step 3: Add a temporary stub for `handleSelect` so the file compiles**

Add inside `MentionTextarea` body (after `handleChange`):

```ts
  // Temporary stub — replaced in Task 4
  const handleSelect = useCallback((_worker: MentionWorker) => {
    setMentionState(null)
  }, [])
```

- [ ] **Step 4: Type-check**

```bash
cd web && npx tsc -b --noEmit
```

Expected: No errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/mention-textarea.tsx
git commit -m "feat: add MentionPanel UI to MentionTextarea"
```

---

### Task 4: Implement `handleSelect` and `onBlur`

**Files:**
- Modify: `web/src/components/mention-textarea.tsx`

- [ ] **Step 1: Replace the stub `handleSelect` with the real implementation**

Replace the stub inside `MentionTextarea`:

```ts
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
```

- [ ] **Step 2: Add `handleBlur` and wire it to the textarea**

Add inside `MentionTextarea` body (after `handleSelect`):

```ts
  const handleBlur = useCallback(() => {
    // Delay so onMouseDown on a candidate fires before the panel closes
    setTimeout(() => setMentionState(null), 150)
  }, [])
```

Update the `<textarea>` JSX to include `onBlur`:

```tsx
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
```

Note: `handleKeyDown` is added in Task 5. Add a temporary stub now:

```ts
  // Temporary stub — replaced in Task 5
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      onKeyDown?.(e)
    },
    [onKeyDown]
  )
```

- [ ] **Step 3: Type-check**

```bash
cd web && npx tsc -b --noEmit
```

Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/components/mention-textarea.tsx
git commit -m "feat: implement handleSelect and handleBlur in MentionTextarea"
```

---

### Task 5: Implement keyboard navigation

**Files:**
- Modify: `web/src/components/mention-textarea.tsx`

- [ ] **Step 1: Replace the stub `handleKeyDown` with the real implementation**

```ts
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
      // Panel not active — forward to parent (e.g. Enter to send message)
      onKeyDown?.(e)
    },
    [mentionState, filteredWorkers, handleSelect, onKeyDown]
  )
```

- [ ] **Step 2: Type-check**

```bash
cd web && npx tsc -b --noEmit
```

Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/mention-textarea.tsx
git commit -m "feat: add keyboard navigation to MentionTextarea"
```

---

### Task 6: Wire `MentionTextarea` into `local-chat.tsx`

**Files:**
- Modify: `web/src/pages/local-chat.tsx`

- [ ] **Step 1: Add imports at the top of `local-chat.tsx`**

After the existing imports, add:

```tsx
import { useWorkers } from "@/hooks/use-workers"
import { MentionTextarea } from "@/components/mention-textarea"
```

- [ ] **Step 2: Derive `workerList` inside the `LocalChat` component**

Add after the existing `useLocalChatStream(handleReply)` call (around line 216):

```tsx
  const { data: workersData } = useWorkers()
  const workerList = useMemo(
    () => (workersData ?? []).map(w => ({ id: w.id, name: w.name })),
    [workersData]
  )
```

Also ensure `useMemo` is imported — it's already in the import list from React at the top of the file. If not, add it.

- [ ] **Step 3: Replace `<textarea>` with `<MentionTextarea>`**

Find the existing `<textarea>` block (around line 474–487):

```tsx
              <textarea
                ref={textareaRef}
                className="max-h-[220px] min-h-[120px] w-full resize-none bg-transparent px-3 py-2 text-sm leading-7 placeholder:text-muted-foreground focus:outline-none"
                placeholder={t("localChat.inputPlaceholder")}
                value={input}
                onChange={(event) => setInput(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" && !event.shiftKey) {
                    event.preventDefault()
                    void handleSend()
                  }
                }}
                onPaste={handlePaste}
              />
```

Replace with:

```tsx
              <MentionTextarea
                textareaRef={textareaRef}
                className="max-h-[220px] min-h-[120px] w-full resize-none bg-transparent px-3 py-2 text-sm leading-7 placeholder:text-muted-foreground focus:outline-none"
                placeholder={t("localChat.inputPlaceholder")}
                value={input}
                onChange={setInput}
                onKeyDown={(event) => {
                  if (event.key === "Enter" && !event.shiftKey) {
                    event.preventDefault()
                    void handleSend()
                  }
                }}
                onPaste={handlePaste}
                workers={workerList}
                disabled={isProcessing}
              />
```

- [ ] **Step 4: Verify height-adjustment useEffect is unaffected**

`local-chat.tsx` has a `useEffect` that watches `input` and sets `textareaRef.current.style.height`. Since `textareaRef` is still the same ref object (now passed into `MentionTextarea` as a prop), this `useEffect` continues to work without any changes. No modification needed.

- [ ] **Step 5: Type-check**

```bash
cd web && npx tsc -b --noEmit
```

Expected: No errors.

- [ ] **Step 6: Full build**

```bash
cd web && npm run build
```

Expected: Build succeeds with no errors.

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/local-chat.tsx
git commit -m "feat: wire MentionTextarea into LocalChat with worker list"
```

---

### Task 7: Manual smoke test

**Files:** None (verification only)

- [ ] **Step 1: Start the dev server**

```bash
cd web && npm run dev
```

- [ ] **Step 2: Open `/chat` and verify the following scenarios**

| Scenario | Expected result |
|---|---|
| Type `@` when workers exist | Candidate panel appears above the input, listing workers |
| Type `@` + partial name | List filters to workers whose name starts with typed text |
| Type `@` + text matching nothing | Panel closes automatically |
| Press `↓` / `↑` | Keyboard highlight moves through candidates |
| Press `Enter` while panel open | Selects highlighted worker, inserts `@name ` into textarea, panel closes, cursor placed after space |
| Click a candidate | Selects worker, inserts `@name `, panel closes |
| Press `Escape` while panel open | Panel closes, `@partial` remains in textarea |
| Click outside the panel | Panel closes after ~150ms |
| Type `@name ` then type `@` again | Second `@` triggers a fresh panel |
| Send a message starting with `@validWorkerName ` | Backend routes directly to that Worker (verify in execution log) |
| Workers list empty (0 workers) | No panel appears when `@` is typed |
| `isProcessing=true` (message sent) | Input disabled, panel closes |

- [ ] **Step 3: Commit if any minor fixes were needed**

```bash
git add -p
git commit -m "fix: <describe what was fixed during smoke test>"
```

---

## Callback Order Note

`handleKeyDown` references `handleSelect` and `filteredWorkers`. Because `filteredWorkers` is derived from `mentionState` (which is set in `handleChange`), there is a React closure dependency. All three (`handleKeyDown`, `handleSelect`, `filteredWorkers`) must be declared in the order: `filteredWorkers` → `handleSelect` → `handleKeyDown` inside the component body.

Correct declaration order inside `MentionTextarea`:

```
useState(mentionState)
useMemo(filteredWorkers)       ← depends on mentionState
useCallback(handleChange)      ← depends on onChange, workers
useCallback(handleSelect)      ← depends on mentionState, value, onChange, textareaRef
useCallback(handleBlur)
useCallback(handleKeyDown)     ← depends on mentionState, filteredWorkers, handleSelect, onKeyDown
```
