# Dept Env Dialog: Hide on Sub-operation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When the department environment variable dialog is open, any create/edit/delete sub-operation hides the parent dialog temporarily and restores it after the operation completes or is cancelled.

**Architecture:** Add an optional `onSubDialogChange` callback prop to `EnvConfigPanel` that fires `true` when a sub-dialog opens and `false` when it closes. `Departments.tsx` tracks `subDialogOpen` state and gates the dept env dialog's `open` prop on `!!envTarget && !subDialogOpen`.

**Tech Stack:** React (useState), TypeScript

---

## File Map

| File | Change |
|------|--------|
| `web/src/components/env-config-panel.tsx` | Add `onSubDialogChange` prop; call it at each sub-dialog open/close site |
| `web/src/pages/departments.tsx` | Add `subDialogOpen` state; wire `open` condition; add `closeEnvDialog` helper |

---

### Task 1: Add `onSubDialogChange` prop to EnvConfigPanel

**Files:**
- Modify: `web/src/components/env-config-panel.tsx`

- [ ] **Step 1: Add the prop to EnvConfigPanelProps**

In `env-config-panel.tsx`, find `EnvConfigPanelProps` (line ~269) and add the new optional prop:

```tsx
interface EnvConfigPanelProps {
  scope: "global" | "bee" | "department" | "worker"
  scopeId?: string
  onSubDialogChange?: (open: boolean) => void
}
```

- [ ] **Step 2: Destructure the new prop in EnvConfigPanel**

Change the function signature from:

```tsx
export function EnvConfigPanel({ scope, scopeId }: EnvConfigPanelProps) {
```

to:

```tsx
export function EnvConfigPanel({ scope, scopeId, onSubDialogChange }: EnvConfigPanelProps) {
```

- [ ] **Step 3: Wire the Add button click**

Find the Add button (line ~297):

```tsx
<Button size="sm" onClick={() => setAddDialogOpen(true)}>
```

Change to:

```tsx
<Button size="sm" onClick={() => { setAddDialogOpen(true); onSubDialogChange?.(true) }}>
```

- [ ] **Step 4: Wire AddEnvDialog close**

Find the `<AddEnvDialog>` usage (line ~347):

```tsx
<AddEnvDialog
  open={addDialogOpen}
  onOpenChange={setAddDialogOpen}
  scope={scope}
  scopeId={scopeId}
  existingKeys={existingKeys}
/>
```

Change to:

```tsx
<AddEnvDialog
  open={addDialogOpen}
  onOpenChange={(open) => { setAddDialogOpen(open); if (!open) onSubDialogChange?.(false) }}
  scope={scope}
  scopeId={scopeId}
  existingKeys={existingKeys}
/>
```

- [ ] **Step 5: Wire the Edit icon click**

Find the edit button in the table row (line ~324):

```tsx
<Button
  variant="ghost"
  size="icon-xs"
  onClick={() => setEditTarget(env)}
  title={t("common.edit")}
>
```

Change to:

```tsx
<Button
  variant="ghost"
  size="icon-xs"
  onClick={() => { setEditTarget(env); onSubDialogChange?.(true) }}
  title={t("common.edit")}
>
```

- [ ] **Step 6: Wire EditEnvDialog close**

Find the `<EditEnvDialog>` usage (line ~355):

```tsx
<EditEnvDialog
  target={editTarget}
  onClose={() => setEditTarget(null)}
  scope={scope}
  scopeId={scopeId}
/>
```

Change to:

```tsx
<EditEnvDialog
  target={editTarget}
  onClose={() => { setEditTarget(null); onSubDialogChange?.(false) }}
  scope={scope}
  scopeId={scopeId}
/>
```

- [ ] **Step 7: Wire the Delete icon click**

Find the delete button in the table row (line ~330):

```tsx
<Button
  variant="ghost"
  size="icon-xs"
  onClick={() => setDeleteTarget(env)}
  title={t("common.delete")}
>
```

Change to:

```tsx
<Button
  variant="ghost"
  size="icon-xs"
  onClick={() => { setDeleteTarget(env); onSubDialogChange?.(true) }}
  title={t("common.delete")}
>
```

- [ ] **Step 8: Wire delete confirm Dialog close**

Find the delete confirm Dialog (line ~362):

```tsx
<Dialog open={!!deleteTarget} onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}>
```

Change to:

```tsx
<Dialog open={!!deleteTarget} onOpenChange={(open) => { if (!open) { setDeleteTarget(null); onSubDialogChange?.(false) } }}>
```

- [ ] **Step 9: Type-check**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors related to `env-config-panel.tsx`.

- [ ] **Step 10: Commit**

```bash
git add web/src/components/env-config-panel.tsx
git commit -m "feat(web/env): add onSubDialogChange callback prop to EnvConfigPanel"
```

---

### Task 2: Wire subDialogOpen state in Departments.tsx

**Files:**
- Modify: `web/src/pages/departments.tsx`

- [ ] **Step 1: Add subDialogOpen state**

In `departments.tsx`, find the existing state declarations (line ~48):

```tsx
const [mode, setMode] = useState<Mode>("idle")
const [targetDept, setTargetDept] = useState<Department | null>(null)
const [envTarget, setEnvTarget] = useState<Department | null>(null)
```

Add `subDialogOpen` after `envTarget`:

```tsx
const [mode, setMode] = useState<Mode>("idle")
const [targetDept, setTargetDept] = useState<Department | null>(null)
const [envTarget, setEnvTarget] = useState<Department | null>(null)
const [subDialogOpen, setSubDialogOpen] = useState(false)
```

- [ ] **Step 2: Add closeEnvDialog helper**

After `resetForm` (line ~55), add a new helper:

```tsx
const closeEnvDialog = () => {
  setEnvTarget(null)
  setSubDialogOpen(false)
}
```

- [ ] **Step 3: Update dept env Dialog open condition and onOpenChange**

Find the dept env Dialog (line ~283):

```tsx
<Dialog open={!!envTarget} onOpenChange={(open) => { if (!open) setEnvTarget(null) }}>
```

Change to:

```tsx
<Dialog open={!!envTarget && !subDialogOpen} onOpenChange={(open) => { if (!open) closeEnvDialog() }}>
```

- [ ] **Step 4: Pass onSubDialogChange to EnvConfigPanel**

Find the `<EnvConfigPanel>` inside the dept env Dialog (line ~297):

```tsx
{envTarget && (
  <EnvConfigPanel scope="department" scopeId={envTarget.id} />
)}
```

Change to:

```tsx
{envTarget && (
  <EnvConfigPanel scope="department" scopeId={envTarget.id} onSubDialogChange={setSubDialogOpen} />
)}
```

- [ ] **Step 5: Type-check**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/departments.tsx
git commit -m "feat(web/dept): hide dept env dialog during sub-operations"
```

---

### Task 3: Manual verification

- [ ] **Step 1: Start the dev server**

```bash
cd web && npm run dev
```

- [ ] **Step 2: Verify Add flow**

1. Navigate to the Departments page
2. Hover over any department row — click the 🔑 (key) icon
3. Confirm the dept env dialog opens
4. Click the **Add** button inside the dialog
5. **Expected:** dept env dialog disappears, Add dialog appears
6. Fill in a key/value and click **Create** (or **Cancel**)
7. **Expected:** Add dialog closes, dept env dialog reappears with the updated list

- [ ] **Step 3: Verify Edit flow**

1. With at least one env var in the list, click the ✏️ (edit) icon for any row
2. **Expected:** dept env dialog disappears, Edit dialog appears
3. Change the value and click **Save** (or **Cancel**)
4. **Expected:** Edit dialog closes, dept env dialog reappears

- [ ] **Step 4: Verify Delete flow**

1. Click the 🗑️ (delete) icon for any row
2. **Expected:** dept env dialog disappears, Delete confirmation dialog appears
3. Click **Delete** (or **Cancel**)
4. **Expected:** Delete dialog closes, dept env dialog reappears

- [ ] **Step 5: Verify global/bee scope is unaffected**

1. Navigate to the Env Config page (`/env`)
2. Add/edit/delete an env var in the global or bee section
3. **Expected:** dialogs stack as before (no change in behavior)

- [ ] **Step 6: Verify subDialogOpen reset**

1. Open a dept env dialog
2. Click Add — parent dialog hides
3. Without completing the Add, close the Add dialog (Cancel or Escape)
4. **Expected:** dept env dialog reappears
5. Now close the dept env dialog (Close button or Escape)
6. **Expected:** dialog closes cleanly; re-open another department's env dialog — it opens immediately without being stuck
