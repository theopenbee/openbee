# Worker Copy Feature Design

**Date:** 2026-04-15
**Status:** Approved

## Overview

Add a copy button to the Worker list page and Worker detail page. Clicking it opens a pre-filled Sheet form allowing the user to modify fields before creating a new Worker based on an existing one.

## Requirements

- Copy button appears in the Worker list row dropdown menu (between "View" and "Delete")
- Copy button also appears in the Worker detail page header (alongside the status badge)
- All fields are pre-filled from the source Worker: name (with " 副本" suffix), description, memory, work_dir, permission_scopes, and department assignments
- All fields remain editable before submission
- Submitting creates a brand-new Worker (same API as create)
- On success: Sheet closes, list refreshes
- On failure: error shown inside Sheet, Sheet stays open

## Architecture

### New File

**`web/src/components/create-worker-sheet.tsx`**

Extracted from the inline Sheet in `workers.tsx`. Accepts an optional `initialValues` prop:

```typescript
interface CreateWorkerSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialValues?: {
    name: string
    description: string
    memory: string
    work_dir: string
    permission_scopes: string
    departmentIds: string[]
  }
}
```

Behavior:
- When `initialValues` is absent → title: "Create Worker", name field empty (original behavior)
- When `initialValues` is present → title: "Copy Worker", name pre-filled as `{originalName} 副本`, all other fields pre-filled
- Submit button text: "Create" in both cases
- Form state is fully reset whenever the Sheet closes

### Modified Files

**`web/src/pages/workers.tsx`**
- Remove inline Sheet, replace with `<CreateWorkerSheet>`
- Add `copySource: Worker | null` state (null = closed, Worker = copy mode)
- Add "Copy" `DropdownMenuItem` (using `CopyIcon` from lucide-react) in each row's dropdown, between "View" and the separator before "Delete"
- Pass `copySource` data as `initialValues` when set

**`web/src/pages/worker-detail.tsx`**
- Import `<CreateWorkerSheet>` and `useCreateWorker`, `useSetWorkerDepartments`
- Add `copySheetOpen: boolean` state
- Add "Copy" Button (outline variant, with `CopyIcon`) to `PageHeader` actions, to the left of the existing status badge
- Render `<CreateWorkerSheet open={copySheetOpen} onOpenChange={setCopySheetOpen} initialValues={...worker fields...} />`

## Data Flow

```
User clicks "Copy"
  → copySource = worker (list page) OR copySheetOpen = true (detail page)
  → CreateWorkerSheet opens, initialValues injected
  → User edits fields (name defaults to "{original} 副本")
  → User submits
  → createWorker.mutateAsync({ name, description, memory, work_dir, permission_scopes })
  → if departmentIds.length > 0:
       setWorkerDepts.mutateAsync({ workerId: newWorker.id, departmentIds })
  → success → Sheet closes, query cache invalidated, list refreshes
  → failure → error displayed in Sheet, Sheet stays open
```

## i18n Keys

New keys to add to `web/src/locales/zh.json` and `web/src/locales/en.json`:

| Key | Chinese | English |
|-----|---------|---------|
| `workers.copyWorker` | 复制 Worker | Copy Worker |
| `workers.form.copyPanelDescription` | 修改后创建一个新的 Worker 副本 | Modify and create a new copy of this worker |

Existing keys reused: `common.copy`, `common.create`, `common.cancel`.

## UI Details

### List Page Dropdown Menu

```
View
───────
Copy        ← new item (CopyIcon)
───────
Delete
```

### Detail Page Header

```
PageHeader title="Worker Name"
  actions:
    [Copy]  [Status Badge]   ← Copy button added to the left
```

### Copy Sheet

- Title: "Copy Worker"
- Description: "Modify and create a new copy of this worker"
- Name field: pre-filled with `{originalName} 副本`, autoFocus
- All other fields: pre-filled from source Worker
- Department checkboxes: pre-checked to match source Worker's departments
- Submit button: "Create" (same text and behavior as create)
- Error display: same pattern as existing create form

## Test Checklist

- [ ] Clicking "Copy" in list dropdown opens Sheet with all fields pre-filled
- [ ] Name field defaults to `{original} 副本`
- [ ] All fields are editable before submitting
- [ ] Submitting creates a new Worker (does not modify original)
- [ ] New Worker appears in the list after creation
- [ ] Department assignments are correctly copied to the new Worker
- [ ] Permission scopes are correctly copied
- [ ] Closing the Sheet without submitting leaves original Worker unchanged
- [ ] Re-opening the Sheet after close shows fresh state (no leftover data)
- [ ] Copy button in detail page opens Sheet pre-filled with that Worker's data
- [ ] Error from API is displayed inside the Sheet
