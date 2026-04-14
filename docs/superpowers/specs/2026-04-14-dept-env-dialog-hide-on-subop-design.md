# Dept Env Dialog: Hide on Sub-operation Design

**Date:** 2026-04-14  
**Branch:** feature/env-config-web-ui

## Overview

When the department environment variable dialog is open and the user initiates a create, edit, or delete operation, the parent dialog should temporarily hide to avoid stacked dialogs. It reappears automatically once the sub-operation dialog closes (success or cancel).

## Scope

This behavior applies **only** to the department environment variable scenario (`departments.tsx`). The global and bee scope `EnvConfigPanel` usages are unaffected since they are embedded directly in pages, not inside dialogs.

## Design

### Architecture

The solution uses a callback prop on `EnvConfigPanel` to notify the parent when any sub-dialog opens or closes. The parent controls visibility of the dept env dialog based on this signal.

```
Departments.tsx
  └─ [dept env Dialog] open={!!envTarget && !subDialogOpen}
       └─ EnvConfigPanel onSubDialogChange={setSubDialogOpen}
            ├─ AddEnvDialog    → calls onSubDialogChange(true/false)
            ├─ EditEnvDialog   → calls onSubDialogChange(true/false)
            └─ Delete Dialog   → calls onSubDialogChange(true/false)
```

### Component Changes

#### `env-config-panel.tsx`

Add optional prop to `EnvConfigPanelProps`:

```ts
onSubDialogChange?: (open: boolean) => void
```

Call this prop at the following points in `EnvConfigPanel`:

| Event | Call |
|-------|------|
| Add button clicked (`setAddDialogOpen(true)`) | `onSubDialogChange?.(true)` |
| AddEnvDialog fully closed (`setAddDialogOpen(false)`) | `onSubDialogChange?.(false)` |
| Edit icon clicked (`setEditTarget(env)`) | `onSubDialogChange?.(true)` |
| EditEnvDialog closed (`setEditTarget(null)`) | `onSubDialogChange?.(false)` |
| Delete icon clicked (`setDeleteTarget(env)`) | `onSubDialogChange?.(true)` |
| Delete confirm Dialog closed (`setDeleteTarget(null)`) | `onSubDialogChange?.(false)` |

Because only one sub-dialog can be open at a time, no debouncing or counting is needed — the boolean tracks correctly.

#### `departments.tsx`

Add a new state variable:

```tsx
const [subDialogOpen, setSubDialogOpen] = useState(false)
```

Change the dept env dialog's `open` condition from:

```tsx
open={!!envTarget}
```

to:

```tsx
open={!!envTarget && !subDialogOpen}
```

Pass the callback to `EnvConfigPanel`:

```tsx
<EnvConfigPanel
  scope="department"
  scopeId={envTarget.id}
  onSubDialogChange={setSubDialogOpen}
/>
```

### State Preservation

`envTarget` is never cleared during sub-operations — it remains set throughout. When the sub-dialog closes and `subDialogOpen` returns to `false`, the parent dialog reopens with `envTarget` still pointing to the same department and `EnvConfigPanel` restoring its state (list refetches via React Query).

`subDialogOpen` must be reset to `false` when `envTarget` is cleared (i.e., in the handler that calls `setEnvTarget(null)`). This prevents a stale `subDialogOpen=true` from blocking the next time the user opens a different department's env dialog:

```tsx
const closeEnvDialog = () => {
  setEnvTarget(null)
  setSubDialogOpen(false)
}
```

Use `closeEnvDialog` as the `onOpenChange` handler for the dept env dialog.

### User-Visible Behavior

1. User clicks 🔑 on a department → dept env dialog opens
2. User clicks Add/Edit/Delete icon → dept env dialog disappears, sub-operation dialog appears
3. User completes or cancels the sub-operation → sub-operation dialog closes, dept env dialog reappears

### No Transition Animation

No animation is added for the hide/show transition. The dialog disappears and reappears instantly. If a transition is desired in the future, it can be added to the Dialog component separately.

## Files Changed

| File | Change |
|------|--------|
| `web/src/components/env-config-panel.tsx` | Add `onSubDialogChange` prop; call it on sub-dialog open/close |
| `web/src/pages/departments.tsx` | Add `subDialogOpen` state; wire `open` condition and callback |
