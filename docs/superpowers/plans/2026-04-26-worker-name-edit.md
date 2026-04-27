# Worker Name Edit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an editable name field (with random-name button) to `EditWorkerInfoSheet` so users can rename a worker from the detail page.

**Architecture:** Single-file change — `web/src/components/edit-worker-info-sheet.tsx`. The backend `PUT /workers/{id}` already accepts `name`; the `useRandomWorkerName` hook already exists. This task wires those together in the edit sheet UI, exactly mirroring the pattern used in `CreateWorkerSheet`.

**Tech Stack:** React, TypeScript, shadcn/ui, lucide-react, react-i18next, React Query

---

### Task 1: Add name field to EditWorkerInfoSheet

**Files:**
- Modify: `web/src/components/edit-worker-info-sheet.tsx`

- [ ] **Step 1: Update imports**

  Replace the existing imports block at the top of the file with the additions below. The changes are:
  - Add `Shuffle, Loader2` to the lucide-react import
  - Add `useRandomWorkerName` to the use-workers import
  - Add `Tooltip, TooltipContent, TooltipTrigger` import

  ```tsx
  import { useState, useEffect, type FormEvent } from "react"
  import { useTranslation } from "react-i18next"
  import { Search, Shuffle, Loader2 } from "lucide-react"
  import { useUpdateWorker, useRandomWorkerName } from "@/hooks/use-workers"
  import { useFlatDepartments, useSetWorkerDepartments } from "@/hooks/use-departments"
  import { useEnabledEngines } from "@/hooks/use-config"
  import { Button } from "@/components/ui/button"
  import { Input } from "@/components/ui/input"
  import { Label } from "@/components/ui/label"
  import { Textarea } from "@/components/ui/textarea"
  import { Separator } from "@/components/ui/separator"
  import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
  } from "@/components/ui/tooltip"
  import {
    Sheet,
    SheetContent,
    SheetHeader,
    SheetTitle,
    SheetFooter,
  } from "@/components/ui/sheet"
  import {
    Select,
    SelectContent,
    SelectTrigger,
    SelectValue,
  } from "@/components/ui/select"
  import { EngineSelectItems } from "@/components/engine-select-items"
  import { EngineArgsSection } from "@/components/engine-args-section"
  import { SectionHeading } from "@/components/section-heading"
  import { getErrorMessage } from "@/lib/utils"
  import { engineArgsEqual, stripEmptyEngineArgs } from "@/lib/engine-args"
  import type { Worker, Engine } from "@/lib/types"
  import { DEFAULT_ENGINE, pickDefaultEngine } from "@/lib/types"
  ```

- [ ] **Step 2: Add name state and random name hook**

  Inside `EditWorkerInfoSheet`, add `name` state and the `randomName` hook alongside the existing state declarations. Replace the existing state block (lines 46–51):

  ```tsx
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [engine, setEngine] = useState<Engine>(DEFAULT_ENGINE)
  const [selectedDeptIds, setSelectedDeptIds] = useState<Set<string>>(new Set())
  const [engineArgs, setEngineArgs] = useState<Record<string, string>>({})
  const [deptSearch, setDeptSearch] = useState("")
  const [submitError, setSubmitError] = useState("")
  const randomName = useRandomWorkerName()
  const nameExhausted = randomName.data?.exhausted ?? false
  ```

- [ ] **Step 3: Initialise name in useEffect and reset randomName**

  Replace the existing `useEffect` body (lines 53–62) to also set `name` and reset the random name mutation:

  ```tsx
  useEffect(() => {
    if (open) {
      setName(worker.name ?? "")
      setDescription(worker.description ?? "")
      setEngine(pickDefaultEngine(worker.engine, enabledEngines))
      setSelectedDeptIds(new Set(worker.departments?.map((d) => d.id) ?? []))
      setEngineArgs(worker.engine_args ?? {})
      setDeptSearch("")
      setSubmitError("")
      randomName.reset()
    }
  }, [open, worker, enabledEngines])
  ```

- [ ] **Step 4: Add handleRandomName and update handleSubmit**

  Add `handleRandomName` right after the `useEffect`. Then update `handleSubmit` to include `name` in the changed-field check and payload. Replace the existing `handleSubmit` function and add the new handler:

  ```tsx
  const handleRandomName = async () => {
    try {
      const result = await randomName.mutateAsync()
      if (!result.exhausted && result.name) {
        setName(result.name)
      }
    } catch {
    }
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setSubmitError("")
    try {
      const originalDeptIds = worker.departments?.map((d) => d.id).sort().join(",") ?? ""
      const newDeptIds = [...selectedDeptIds].sort().join(",")
      const engineArgsChanged = !engineArgsEqual(
        stripEmptyEngineArgs(engineArgs),
        worker.engine_args ?? {},
      )
      const workerChanged =
        name !== worker.name ||
        description !== (worker.description ?? "") ||
        engine !== pickDefaultEngine(worker.engine, enabledEngines) ||
        engineArgsChanged
      const deptsChanged = newDeptIds !== originalDeptIds

      const ops: Promise<unknown>[] = []
      if (workerChanged) {
        const data: Record<string, unknown> = { description, engine, engine_args: engineArgs }
        if (name !== worker.name) data.name = name
        ops.push(updateWorker.mutateAsync({ id: worker.id, data }))
      }
      if (deptsChanged) {
        ops.push(setWorkerDepts.mutateAsync({ workerId: worker.id, departmentIds: [...selectedDeptIds] }))
      }
      await Promise.all(ops)
      onOpenChange(false)
    } catch (err) {
      setSubmitError(getErrorMessage(err))
    }
  }
  ```

- [ ] **Step 5: Add the Name field UI above the Description field**

  Inside the `<div className="px-6 py-5 space-y-5">` block, insert the Name field block right after the `{submitError && ...}` block and before the existing Description block:

  ```tsx
  <div className="space-y-1.5">
    <Label htmlFor="ewis-name">
      {t("workers.form.name")}
      <span className="ml-1 text-destructive" aria-hidden>*</span>
    </Label>
    <div className="flex gap-2">
      <Input
        id="ewis-name"
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder={t("workers.form.namePlaceholder")}
        required
        className="flex-1"
      />
      <Tooltip open={nameExhausted || undefined}>
        <TooltipTrigger render={<span className="inline-flex" />}>
          <Button
            type="button"
            variant="outline"
            size="icon"
            disabled={nameExhausted || randomName.isPending}
            onClick={handleRandomName}
            aria-label={t("workers.form.randomName")}
          >
            {randomName.isPending
              ? <Loader2 className="size-4 animate-spin" />
              : <Shuffle className="size-4" />
            }
          </Button>
        </TooltipTrigger>
        {nameExhausted && (
          <TooltipContent>
            <p>{t("workers.form.randomNameExhausted")}</p>
          </TooltipContent>
        )}
      </Tooltip>
    </div>
    <p className="text-xs text-muted-foreground">{t("workers.form.nameHelper")}</p>
  </div>
  ```

- [ ] **Step 6: Disable Save button when name is empty**

  Update the Save button's `disabled` prop in `SheetFooter` to also block submission when `name` is empty:

  ```tsx
  <Button
    type="submit"
    form="edit-worker-info-form"
    disabled={isPending || !name.trim()}
    className="flex-1"
  >
    {t("common.save")}
  </Button>
  ```

- [ ] **Step 7: Verify TypeScript compiles**

  Run from the `web/` directory:

  ```bash
  cd web && npx tsc --noEmit
  ```

  Expected: no errors.

- [ ] **Step 8: Manual smoke test**

  Start the dev server (`cd web && npm run dev`) and open a worker detail page. Open the Edit Sheet and verify:

  1. Name field is pre-filled with the worker's current name
  2. Clicking 🎲 fills in a new random name
  3. Changing the name and saving updates the header in the detail page
  4. Entering a name that already exists shows the backend error in the error area
  5. Clearing the name disables the Save button
  6. Leaving name unchanged and saving still succeeds (no spurious error)

- [ ] **Step 9: Commit**

  ```bash
  git add web/src/components/edit-worker-info-sheet.tsx
  git commit -m "feat(web): add name field with random button to EditWorkerInfoSheet"
  ```
