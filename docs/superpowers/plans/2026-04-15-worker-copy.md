# Worker Copy Feature Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a copy button to the Worker list and detail pages that opens a pre-filled Sheet form so users can create a new Worker based on an existing one.

**Architecture:** Extract the existing inline create-worker Sheet from `workers.tsx` into a standalone `CreateWorkerSheet` component that accepts optional `initialValues`. When `initialValues` is provided the Sheet pre-fills all fields (name gets " 副本" suffix) and shows a "Copy Worker" title; the submit path is identical to create. Both the list page dropdown and the detail page header render this component.

**Tech Stack:** React 19, TypeScript, TanStack React Query v5, Base UI (Sheet/Dialog), Tailwind CSS v4, i18next, lucide-react

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `web/src/components/create-worker-sheet.tsx` | Reusable Sheet for both create and copy |
| Modify | `web/src/pages/workers.tsx` | Replace inline Sheet; add copy to dropdown |
| Modify | `web/src/pages/worker-detail.tsx` | Add copy button + Sheet to header |
| Modify | `web/src/locales/zh.json` | Add `workers.copyWorker` and `workers.form.copyPanelDescription` |
| Modify | `web/src/locales/en.json` | Same two keys in English |

---

## Task 1: Add i18n keys

**Files:**
- Modify: `web/src/locales/zh.json`
- Modify: `web/src/locales/en.json`

- [ ] **Step 1: Add Chinese keys**

In `web/src/locales/zh.json`, find `"createWorker": "创建员工",` and add `copyWorker` after it. Also find `"panelDescription": "为你的蜂巢配置一名新的 AI 员工。",` and add `copyPanelDescription` after it:

```json
// After "createWorker": "创建员工",
"copyWorker": "复制员工",

// After "panelDescription": "为你的蜂巢配置一名新的 AI 员工。",
"copyPanelDescription": "修改后创建一个新的员工副本。",
```

- [ ] **Step 2: Add English keys**

In `web/src/locales/en.json`, find `"createWorker": "Create Worker",` and add `copyWorker` after it. Also find `"panelDescription": "Configure a new AI worker for your hive.",` and add `copyPanelDescription` after it:

```json
// After "createWorker": "Create Worker",
"copyWorker": "Copy Worker",

// After "panelDescription": "Configure a new AI worker for your hive.",
"copyPanelDescription": "Modify and create a new copy of this worker.",
```

- [ ] **Step 3: Type-check**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/locales/zh.json web/src/locales/en.json
git commit -m "feat(i18n): add worker copy i18n keys"
```

---

## Task 2: Create `CreateWorkerSheet` component

**Files:**
- Create: `web/src/components/create-worker-sheet.tsx`

This component is extracted from the inline Sheet in `workers.tsx` and extended with `initialValues` support.

- [ ] **Step 1: Create the file**

Create `web/src/components/create-worker-sheet.tsx` with the full content below:

```tsx
import { useState, useMemo, useEffect, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { useCreateWorker } from "@/hooks/use-workers"
import { useDepartments, useSetWorkerDepartments } from "@/hooks/use-departments"
import { flattenDeptTree } from "@/lib/department-utils"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Separator } from "@/components/ui/separator"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetFooter,
} from "@/components/ui/sheet"
import { ScopeToggleCard } from "@/components/scope-toggle-card"
import { KNOWN_SCOPES, serializeScopes, parseScopes, toggleScope } from "@/lib/scopes"
import { getErrorMessage } from "@/lib/utils"

export interface WorkerInitialValues {
  name: string
  description: string
  memory: string
  work_dir: string
  permission_scopes: string
  departmentIds: string[]
}

interface CreateWorkerSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialValues?: WorkerInitialValues
}

export function CreateWorkerSheet({ open, onOpenChange, initialValues }: CreateWorkerSheetProps) {
  const { t } = useTranslation()
  const createWorker = useCreateWorker()
  const setWorkerDepts = useSetWorkerDepartments()
  const { data: departments = [] } = useDepartments()
  const flatDepts = useMemo(() => flattenDeptTree(departments), [departments])
  const isCopy = initialValues !== undefined

  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [memory, setMemory] = useState("")
  const [workDir, setWorkDir] = useState("")
  const [selectedScopes, setSelectedScopes] = useState<string[]>([])
  const [selectedDeptIds, setSelectedDeptIds] = useState<Set<string>>(new Set())
  const [submitError, setSubmitError] = useState("")

  // Re-initialize form fields each time the sheet opens
  useEffect(() => {
    if (open) {
      setName(initialValues ? `${initialValues.name} 副本` : "")
      setDescription(initialValues?.description ?? "")
      setMemory(initialValues?.memory ?? "")
      setWorkDir(initialValues?.work_dir ?? "")
      setSelectedScopes(initialValues ? parseScopes(initialValues.permission_scopes) : [])
      setSelectedDeptIds(initialValues ? new Set(initialValues.departmentIds) : new Set())
      setSubmitError("")
    }
  }, [open]) // eslint-disable-line react-hooks/exhaustive-deps

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setSubmitError("")
    try {
      const worker = await createWorker.mutateAsync({
        name: name.trim(),
        description,
        memory: memory || undefined,
        work_dir: workDir || undefined,
        permission_scopes: serializeScopes(selectedScopes) || undefined,
      })
      if (selectedDeptIds.size > 0) {
        await setWorkerDepts.mutateAsync({ workerId: worker.id, departmentIds: [...selectedDeptIds] })
      }
      onOpenChange(false)
    } catch (err) {
      setSubmitError(getErrorMessage(err))
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-[26rem] p-0 gap-0">
        <SheetHeader className="px-6 pt-6 pb-4">
          <SheetTitle>
            {isCopy ? t("workers.copyWorker") : t("workers.createWorker")}
          </SheetTitle>
          <SheetDescription>
            {isCopy ? t("workers.form.copyPanelDescription") : t("workers.form.panelDescription")}
          </SheetDescription>
        </SheetHeader>
        <Separator />
        <form
          id="create-worker-form"
          onSubmit={handleSubmit}
          className="flex-1 overflow-y-auto px-6 py-5 space-y-6"
        >
          {submitError && (
            <div role="alert" className="rounded-2xl border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
              {submitError}
            </div>
          )}

          <div className="space-y-4">
            <p className="text-xs font-medium uppercase tracking-[0.1em] text-muted-foreground">
              {t("workers.form.sectionBasic")}
            </p>
            <div className="space-y-1.5">
              <Label htmlFor="cws-name">
                {t("workers.form.name")}
                <span className="ml-1 text-destructive" aria-hidden>*</span>
              </Label>
              <Input
                id="cws-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={t("workers.form.namePlaceholder")}
                required
                autoFocus
              />
              <p className="text-xs text-muted-foreground">{t("workers.form.nameHelper")}</p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="cws-desc">{t("workers.form.description")}</Label>
              <Textarea
                id="cws-desc"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder={t("workers.form.descriptionPlaceholder")}
                rows={2}
              />
              <p className="text-xs text-muted-foreground">{t("workers.form.descriptionHelper")}</p>
            </div>
          </div>

          <Separator />

          <div className="space-y-4">
            <p className="text-xs font-medium uppercase tracking-[0.1em] text-muted-foreground">
              {t("workers.form.sectionConfig")}
            </p>
            <div className="space-y-1.5">
              <Label htmlFor="cws-workdir">{t("workers.form.workDir")}</Label>
              <Input
                id="cws-workdir"
                value={workDir}
                onChange={(e) => setWorkDir(e.target.value)}
                placeholder={t("workers.form.workDirPlaceholder")}
                className="font-mono text-xs"
              />
              <p className="text-xs text-muted-foreground">{t("workers.form.workDirHelper")}</p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="cws-memory">{t("workers.form.memory")}</Label>
              <Textarea
                id="cws-memory"
                value={memory}
                onChange={(e) => setMemory(e.target.value)}
                placeholder={t("workers.form.memoryPlaceholder")}
                rows={5}
              />
              <p className="text-xs text-muted-foreground">{t("workers.form.memoryHelper")}</p>
            </div>
          </div>

          <Separator />

          <div className="space-y-4">
            <p className="text-xs font-medium uppercase tracking-[0.1em] text-muted-foreground">
              {t("workers.form.sectionPermissions")}
            </p>
            <div className="space-y-2">
              {KNOWN_SCOPES.map((scope) => (
                <ScopeToggleCard
                  key={scope.id}
                  scope={scope}
                  checked={selectedScopes.includes(scope.id)}
                  onToggle={(scopeId, val) =>
                    setSelectedScopes((prev) => toggleScope(prev, scopeId, val))
                  }
                  disabled={createWorker.isPending}
                />
              ))}
            </div>
          </div>

          {flatDepts.length > 0 && (
            <>
              <Separator />
              <div className="space-y-4">
                <p className="text-xs font-medium uppercase tracking-[0.1em] text-muted-foreground">
                  {t("workers.form.sectionDepartment")}
                </p>
                <div className="space-y-2 max-h-40 overflow-y-auto">
                  {flatDepts.map(({ dept, depth }) => (
                    <div
                      key={dept.id}
                      className="flex items-center gap-2"
                      style={{ paddingLeft: `${depth * 12}px` }}
                    >
                      <input
                        type="checkbox"
                        id={`cws-dept-${dept.id}`}
                        checked={selectedDeptIds.has(dept.id)}
                        onChange={(e) => {
                          const next = new Set(selectedDeptIds)
                          if (e.target.checked) next.add(dept.id)
                          else next.delete(dept.id)
                          setSelectedDeptIds(next)
                        }}
                        className="size-4 cursor-pointer rounded accent-primary"
                      />
                      <Label htmlFor={`cws-dept-${dept.id}`} className="cursor-pointer text-sm font-normal">
                        {dept.name}
                      </Label>
                    </div>
                  ))}
                </div>
                <p className="text-xs text-muted-foreground">{t("workers.form.departmentHelper")}</p>
              </div>
            </>
          )}
        </form>
        <Separator />
        <SheetFooter className="px-6 py-4 flex-row gap-2">
          <Button
            type="button"
            variant="outline"
            className="flex-1"
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="submit"
            form="create-worker-form"
            disabled={createWorker.isPending || setWorkerDepts.isPending || !name.trim()}
            className="flex-1"
          >
            {t("workers.createWorker")}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
```

- [ ] **Step 2: Type-check**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/create-worker-sheet.tsx
git commit -m "feat(workers): add CreateWorkerSheet component with initialValues support"
```

---

## Task 3: Update `workers.tsx` — use `CreateWorkerSheet`, add copy to dropdown

**Files:**
- Modify: `web/src/pages/workers.tsx`

- [ ] **Step 1: Add import for `CreateWorkerSheet` and `Copy` icon**

At the top of `web/src/pages/workers.tsx`, add these imports (replace the existing lucide-react import line and add the component import):

```tsx
// Replace:
import { EyeIcon, MoreHorizontalIcon, Trash2Icon } from "lucide-react"
// With:
import { Copy, EyeIcon, MoreHorizontalIcon, Trash2Icon } from "lucide-react"
```

Add after the last existing import block:

```tsx
import { CreateWorkerSheet } from "@/components/create-worker-sheet"
import type { Worker } from "@/lib/types"
```

- [ ] **Step 2: Add `copySource` state**

In `Workers()`, after `const [open, setOpen] = useState(false)`, add:

```tsx
const [copySource, setCopySource] = useState<Worker | null>(null)
```

Also add a combined open-change handler after that line:

```tsx
const handleSheetOpenChange = (isOpen: boolean) => {
  if (!isOpen) {
    setOpen(false)
    setCopySource(null)
  }
}
```

- [ ] **Step 3: Replace inline Sheet with `<CreateWorkerSheet>`**

In the `actions` prop of `<PageHeader>`, replace everything from `<Sheet open={open}...` through `</Sheet>` (lines 159–311) with:

```tsx
<CreateWorkerSheet
  open={open || copySource !== null}
  onOpenChange={handleSheetOpenChange}
  initialValues={
    copySource
      ? {
          name: copySource.name,
          description: copySource.description,
          memory: copySource.memory,
          work_dir: copySource.work_dir,
          permission_scopes: copySource.permission_scopes ?? "",
          departmentIds: copySource.departments?.map((d) => d.id) ?? [],
        }
      : undefined
  }
/>
```

- [ ] **Step 4: Remove now-unused state and imports from `workers.tsx`**

Remove the following state variables that are now managed inside `CreateWorkerSheet`:

```tsx
// Remove these lines:
const [name, setName] = useState("")
const [description, setDescription] = useState("")
const [memory, setMemory] = useState("")
const [workDir, setWorkDir] = useState("")
const [selectedScopes, setSelectedScopes] = useState<string[]>([])
const [selectedCreateDeptIds, setSelectedCreateDeptIds] = useState<Set<string>>(new Set())
```

Remove the `handleCreate` function (it is now inside `CreateWorkerSheet`).

Remove unused imports that were only needed for the inline Sheet:

```tsx
// Remove from import blocks (keep others):
import { useCreateWorker } from "@/hooks/use-workers"        // if only used in handleCreate
import { useSetWorkerDepartments } from "@/hooks/use-departments"  // if only used in handleCreate
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetFooter,
} from "@/components/ui/sheet"
import { ScopeToggleCard } from "@/components/scope-toggle-card"
import { KNOWN_SCOPES, serializeScopes, toggleScope } from "@/lib/scopes"
```

> Note: Keep `useWorkers`, `useDeleteWorker`, `useDepartments`, `flattenDeptTree`, and the department-related state used by the sidebar filter.

- [ ] **Step 5: Add "Copy" item to the dropdown menu**

In each row's `<DropdownMenuContent>`, add a Copy item between the View item and the separator before Delete:

```tsx
// Existing:
<DropdownMenuItem onClick={() => navigate(`/workers/${w.id}`)}>
  <EyeIcon className="size-4" />
  {t("common.view")}
</DropdownMenuItem>
<DropdownMenuSeparator />
<DropdownMenuItem
  variant="destructive"
  onClick={() => openDeleteDialog({ id: w.id, name: w.name })}
>

// Replace with:
<DropdownMenuItem onClick={() => navigate(`/workers/${w.id}`)}>
  <EyeIcon className="size-4" />
  {t("common.view")}
</DropdownMenuItem>
<DropdownMenuSeparator />
<DropdownMenuItem onClick={() => setCopySource(w)}>
  <Copy className="size-4" />
  {t("common.copy")}
</DropdownMenuItem>
<DropdownMenuSeparator />
<DropdownMenuItem
  variant="destructive"
  onClick={() => openDeleteDialog({ id: w.id, name: w.name })}
>
```

- [ ] **Step 6: Also update the error line to remove stale mutation refs**

The existing error line references `createWorker.error` and `setWorkerDepts.error`. These mutations are now inside `CreateWorkerSheet`, so remove them from the error string in `workers.tsx`:

```tsx
// Replace:
const error = fetchError?.message || createWorker.error?.message || deleteWorker.error?.message || setWorkerDepts.error?.message || ""
// With:
const error = fetchError?.message || deleteWorker.error?.message || ""
```

- [ ] **Step 7: Type-check**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 8: Commit**

```bash
git add web/src/pages/workers.tsx
git commit -m "feat(workers): replace inline Sheet with CreateWorkerSheet, add copy to dropdown"
```

---

## Task 4: Update `worker-detail.tsx` — add copy button + Sheet

**Files:**
- Modify: `web/src/pages/worker-detail.tsx`

- [ ] **Step 1: Add import for `CreateWorkerSheet`**

After the last existing import in `web/src/pages/worker-detail.tsx`, add:

```tsx
import { CreateWorkerSheet } from "@/components/create-worker-sheet"
```

- [ ] **Step 2: Add `copySheetOpen` state**

In `WorkerDetail()`, after the line `const [deptDialogOpen, setDeptDialogOpen] = useState(false)`, add:

```tsx
const [copySheetOpen, setCopySheetOpen] = useState(false)
```

- [ ] **Step 3: Add copy button to `PageHeader` and render `CreateWorkerSheet`**

Replace the existing `<PageHeader>` element:

```tsx
// Replace:
<PageHeader
  title={worker.name}
  actions={<StatusBadge status={worker.status} />}
/>

// With:
<PageHeader
  title={worker.name}
  actions={
    <>
      <Button variant="outline" size="sm" onClick={() => setCopySheetOpen(true)}>
        <Copy className="size-4" />
        {t("common.copy")}
      </Button>
      <StatusBadge status={worker.status} />
    </>
  }
/>
```

Then, directly after the closing `</PageHeader>` tag, add:

```tsx
<CreateWorkerSheet
  open={copySheetOpen}
  onOpenChange={setCopySheetOpen}
  initialValues={{
    name: worker.name,
    description: worker.description,
    memory: worker.memory,
    work_dir: worker.work_dir,
    permission_scopes: worker.permission_scopes ?? "",
    departmentIds: worker.departments?.map((d) => d.id) ?? [],
  }}
/>
```

- [ ] **Step 4: Type-check**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 5: Build verification**

```bash
cd web && npm run build
```

Expected: build succeeds with no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/worker-detail.tsx
git commit -m "feat(workers): add copy button and CreateWorkerSheet to worker detail page"
```

---

## Manual Test Checklist

After all tasks complete, verify in the browser:

- [ ] Worker list: dropdown shows Copy item between View and Delete
- [ ] Clicking Copy opens Sheet titled "复制员工" / "Copy Worker"
- [ ] Name field shows `{original} 副本`
- [ ] All other fields (description, memory, work dir, permissions, departments) are pre-filled
- [ ] Modifying fields and submitting creates a new Worker (original unchanged)
- [ ] New Worker appears in the list
- [ ] Closing Sheet without submitting leaves original unchanged
- [ ] Re-opening Copy Sheet shows fresh pre-filled state (no leftover edits from previous open)
- [ ] Worker detail page header shows a "复制" button to the left of the status badge
- [ ] Copy button on detail page opens Sheet with that worker's data pre-filled
- [ ] Error from API is displayed inside the Sheet (test by submitting duplicate name if API enforces uniqueness)
