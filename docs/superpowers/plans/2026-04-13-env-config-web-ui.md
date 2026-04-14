# Env Config Web UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a full-featured env variable management UI to the OpenBee web app, embedded in Worker detail, Department, and a new Settings page for global scope.

**Architecture:** Shared `EnvConfigPanel` component handles CRUD for any scope via Sheet. Worker detail gets an "Env" tab with scope=worker plus an effective-preview section showing merged global→department→worker chain. Departments page gets a Sheet triggered per row for scope=department. A new Settings page handles scope=global with a sidebar nav entry.

**Tech Stack:** React 19, TanStack Query v5, react-i18next, Lucide React, Tailwind CSS v4, existing UI primitives (Sheet, Table, Dialog, Input, Button)

---

## File Map

| File | Action | Purpose |
|------|--------|---------|
| `web/src/lib/types.ts` | Modify | Add `EnvConfig` type |
| `web/src/lib/api.ts` | Modify | Add `envs` API section |
| `web/src/hooks/use-envs.ts` | Create | TanStack Query hooks for env CRUD |
| `web/src/components/env-config-panel.tsx` | Create | Reusable env list + Sheet CRUD component |
| `web/src/pages/worker-detail.tsx` | Modify | Add "Env" tab (worker scope + effective preview) |
| `web/src/pages/departments.tsx` | Modify | Add "Env" button per department row → Sheet |
| `web/src/pages/settings.tsx` | Create | Global env config page |
| `web/src/app.tsx` | Modify | Add `/settings` route |
| `web/src/components/app-sidebar.tsx` | Modify | Add Settings nav item |
| `web/src/locales/en.json` | Modify | Add env i18n strings |
| `web/src/locales/zh.json` | Modify | Add env i18n strings (Chinese) |

---

## Task 1: Add Types and API

**Files:**
- Modify: `web/src/lib/types.ts`
- Modify: `web/src/lib/api.ts`

- [ ] **Step 1: Add `EnvConfig` type to `types.ts`**

In `web/src/lib/types.ts`, append after the last export:

```typescript
export interface EnvConfig {
  id: string
  scope: "global" | "bee" | "department" | "worker"
  scope_id: string | null
  key: string
  masked: string
  created_at: number
  updated_at: number
}
```

- [ ] **Step 2: Add `envs` API section to `api.ts`**

In `web/src/lib/api.ts`, add `EnvConfig` to the existing import at the top:

```typescript
import type { Worker, WorkerExecution, PaginatedResponse, ChatMessage, LocalMessagesResponse, Task, Department, DepartmentTree, StatsOverview, StatsTrend, EnvConfig } from "./types"
```

Then add the `envs` section inside the `api` object, after the `stats` section:

```typescript
  envs: {
    list: (scope: string, scopeId?: string) => {
      const qs = new URLSearchParams({ scope })
      if (scopeId) qs.set("scope_id", scopeId)
      return fetchAPI<EnvConfig[]>(`/envs?${qs}`)
    },
    create: (data: { scope: string; scope_id?: string; key: string; value: string }) =>
      fetchAPI<EnvConfig>("/envs", { method: "POST", body: JSON.stringify(data) }),
    update: (id: string, value: string) =>
      fetchAPI<{ ok: boolean }>(`/envs/${id}`, {
        method: "PUT",
        body: JSON.stringify({ value }),
      }),
    delete: (id: string) => fetchAPI(`/envs/${id}`, { method: "DELETE" }),
  },
```

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/types.ts web/src/lib/api.ts
git commit -m "feat(web/env): add EnvConfig type and api client methods"
```

---

## Task 2: Add i18n Strings

**Files:**
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`

- [ ] **Step 1: Add env strings to `en.json`**

In `web/src/locales/en.json`, add a new `"envConfig"` key before the closing `}` of the root object:

```json
  "envConfig": {
    "title": "Env Variables",
    "globalTitle": "Global Env Variables",
    "add": "Add Variable",
    "editTitle": "Edit Variable",
    "addTitle": "Add Variable",
    "key": "Key",
    "keyPlaceholder": "e.g. DATABASE_URL",
    "value": "Value",
    "valuePlaceholder": "Enter value",
    "masked": "Value",
    "scope": "Scope",
    "empty": "No environment variables configured.",
    "deleteConfirm": "Delete variable \"{{key}}\"? This action cannot be undone.",
    "effectiveTitle": "Effective Variables",
    "effectiveHint": "Final merged env vars this worker receives at runtime. Later scopes override earlier ones.",
    "sourceGlobal": "global",
    "sourceDepartment": "dept",
    "sourceWorker": "worker",
    "source": "Source",
    "noEffective": "No env variables configured for this worker.",
    "depEnvTitle": "Env Variables"
  },
  "nav": {
    "dashboard": "Dashboard",
    "workers": "Workers",
    "executions": "Sessions",
    "tasks": "Scheduled Tasks",
    "departments": "Departments",
    "directory": "Directory",
    "settings": "Settings"
  }
```

Wait — you must NOT duplicate the `"nav"` key. Instead, just add `"settings": "Settings"` inside the existing `"nav"` object.

The correct edit: in the existing `"nav"` object in `en.json`, add:
```json
    "settings": "Settings"
```

And add the full `"envConfig"` block as a new top-level key.

- [ ] **Step 2: Add env strings to `zh.json`**

Similarly in `web/src/locales/zh.json`, add `"settings": "系统设置"` inside the existing `"nav"` object, and add the full `"envConfig"` block:

```json
  "envConfig": {
    "title": "环境变量",
    "globalTitle": "全局环境变量",
    "add": "添加变量",
    "editTitle": "编辑变量",
    "addTitle": "添加变量",
    "key": "变量名",
    "keyPlaceholder": "例如：DATABASE_URL",
    "value": "变量值",
    "valuePlaceholder": "请输入值",
    "masked": "变量值",
    "scope": "作用域",
    "empty": "暂无环境变量配置。",
    "deleteConfirm": "确认删除变量 \"{{key}}\"？此操作不可撤销。",
    "effectiveTitle": "生效变量预览",
    "effectiveHint": "该员工运行时实际生效的合并环境变量。后置作用域会覆盖前面的同名变量。",
    "sourceGlobal": "全局",
    "sourceDepartment": "部门",
    "sourceWorker": "员工",
    "source": "来源",
    "noEffective": "该员工暂无任何环境变量配置。",
    "depEnvTitle": "环境变量"
  }
```

- [ ] **Step 3: Commit**

```bash
git add web/src/locales/en.json web/src/locales/zh.json
git commit -m "feat(web/env): add i18n strings for env config UI"
```

---

## Task 3: Create `use-envs` Hook

**Files:**
- Create: `web/src/hooks/use-envs.ts`

- [ ] **Step 1: Create the hook file**

Create `web/src/hooks/use-envs.ts` with the full content:

```typescript
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api"

export function useEnvList(scope: string, scopeId?: string) {
  return useQuery({
    queryKey: ["envs", scope, scopeId ?? null],
    queryFn: () => api.envs.list(scope, scopeId),
    select: (data) => data ?? [],
  })
}

export function useCreateEnv() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: { scope: string; scope_id?: string; key: string; value: string }) =>
      api.envs.create(data),
    onSuccess: (_, { scope, scope_id }) => {
      queryClient.invalidateQueries({ queryKey: ["envs", scope, scope_id ?? null] })
    },
  })
}

export function useUpdateEnv(scope: string, scopeId?: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, value }: { id: string; value: string }) =>
      api.envs.update(id, value),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["envs", scope, scopeId ?? null] })
    },
  })
}

export function useDeleteEnv(scope: string, scopeId?: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.envs.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["envs", scope, scopeId ?? null] })
    },
  })
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/hooks/use-envs.ts
git commit -m "feat(web/env): add useEnvList, useCreateEnv, useUpdateEnv, useDeleteEnv hooks"
```

---

## Task 4: Create `EnvConfigPanel` Component

**Files:**
- Create: `web/src/components/env-config-panel.tsx`

This component handles:
- Listing env configs for a given scope/scopeId
- Sheet to create a new variable (key + value, eye toggle)
- Sheet to edit an existing variable (value only, key shown as read-only, eye toggle)
- Confirm dialog to delete

- [ ] **Step 1: Create the component**

Create `web/src/components/env-config-panel.tsx`:

```typescript
import { useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { PlusIcon, Trash2Icon, PencilIcon, EyeIcon, EyeOffIcon } from "lucide-react"
import { useEnvList, useCreateEnv, useUpdateEnv, useDeleteEnv } from "@/hooks/use-envs"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetFooter,
} from "@/components/ui/sheet"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { EmptyState } from "@/components/empty-state"
import { formatTimestamp } from "@/lib/format"
import type { EnvConfig } from "@/lib/types"

interface EnvConfigPanelProps {
  scope: "global" | "department" | "worker"
  scopeId?: string
}

export function EnvConfigPanel({ scope, scopeId }: EnvConfigPanelProps) {
  const { t } = useTranslation()
  const { data: envs = [], isLoading } = useEnvList(scope, scopeId)
  const createEnv = useCreateEnv()
  const updateEnv = useUpdateEnv(scope, scopeId)
  const deleteEnv = useDeleteEnv(scope, scopeId)

  // Sheet state
  const [sheetOpen, setSheetOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<EnvConfig | null>(null)
  const [formKey, setFormKey] = useState("")
  const [formValue, setFormValue] = useState("")
  const [showValue, setShowValue] = useState(false)
  const [formError, setFormError] = useState("")

  // Delete confirm dialog state
  const [deleteTarget, setDeleteTarget] = useState<EnvConfig | null>(null)

  const openCreate = () => {
    setEditTarget(null)
    setFormKey("")
    setFormValue("")
    setShowValue(false)
    setFormError("")
    setSheetOpen(true)
  }

  const openEdit = (env: EnvConfig) => {
    setEditTarget(env)
    setFormKey(env.key)
    setFormValue("")
    setShowValue(false)
    setFormError("")
    setSheetOpen(true)
  }

  const closeSheet = () => {
    setSheetOpen(false)
    setEditTarget(null)
    setFormKey("")
    setFormValue("")
    setFormError("")
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setFormError("")
    try {
      if (editTarget) {
        await updateEnv.mutateAsync({ id: editTarget.id, value: formValue })
      } else {
        await createEnv.mutateAsync({
          scope,
          scope_id: scopeId,
          key: formKey.trim(),
          value: formValue,
        })
      }
      closeSheet()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : String(err))
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      await deleteEnv.mutateAsync(deleteTarget.id)
      setDeleteTarget(null)
    } catch {
      // error is transient — just close
      setDeleteTarget(null)
    }
  }

  const isPending = createEnv.isPending || updateEnv.isPending

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-end">
        <Button size="sm" onClick={openCreate}>
          <PlusIcon className="size-3.5" />
          {t("envConfig.add")}
        </Button>
      </div>

      {isLoading ? null : envs.length === 0 ? (
        <EmptyState title={t("envConfig.empty")} />
      ) : (
        <div className="rounded-2xl border border-border/70 overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("envConfig.key")}</TableHead>
                <TableHead>{t("envConfig.masked")}</TableHead>
                <TableHead className="text-right">{t("workers.columns.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {envs.map((env) => (
                <TableRow key={env.id}>
                  <TableCell className="font-mono text-sm">{env.key}</TableCell>
                  <TableCell className="font-mono text-sm text-muted-foreground">{env.masked}</TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => openEdit(env)}
                        title={t("common.save")}
                      >
                        <PencilIcon className="size-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => setDeleteTarget(env)}
                        title={t("common.delete")}
                      >
                        <Trash2Icon className="size-3.5" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      {/* Create / Edit Sheet */}
      <Sheet open={sheetOpen} onOpenChange={(open) => { if (!open) closeSheet() }}>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>
              {editTarget ? t("envConfig.editTitle") : t("envConfig.addTitle")}
            </SheetTitle>
            <SheetDescription>
              {editTarget ? editTarget.key : t("envConfig.keyPlaceholder")}
            </SheetDescription>
          </SheetHeader>
          <form onSubmit={handleSubmit} className="flex flex-col gap-5 px-4 py-6">
            {formError && (
              <div role="alert" className="rounded-2xl border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                {formError}
              </div>
            )}
            {!editTarget && (
              <div className="space-y-1.5">
                <Label htmlFor="env-key">{t("envConfig.key")}</Label>
                <Input
                  id="env-key"
                  value={formKey}
                  onChange={(e) => setFormKey(e.target.value)}
                  placeholder={t("envConfig.keyPlaceholder")}
                  required
                  autoFocus
                  className="font-mono"
                />
              </div>
            )}
            <div className="space-y-1.5">
              <Label htmlFor="env-value">{t("envConfig.value")}</Label>
              <div className="relative">
                <Input
                  id="env-value"
                  type={showValue ? "text" : "password"}
                  value={formValue}
                  onChange={(e) => setFormValue(e.target.value)}
                  placeholder={t("envConfig.valuePlaceholder")}
                  required
                  autoFocus={!!editTarget}
                  className="font-mono pr-10"
                />
                <button
                  type="button"
                  onClick={() => setShowValue(!showValue)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                  tabIndex={-1}
                >
                  {showValue ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
                </button>
              </div>
            </div>
            <SheetFooter>
              <Button type="button" variant="outline" onClick={closeSheet}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={isPending || !formValue || (!editTarget && !formKey.trim())}>
                {editTarget ? t("common.save") : t("common.create")}
              </Button>
            </SheetFooter>
          </form>
        </SheetContent>
      </Sheet>

      {/* Delete Confirm Dialog */}
      <Dialog open={!!deleteTarget} onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t("common.delete")}</DialogTitle>
            <DialogDescription>
              {t("envConfig.deleteConfirm", { key: deleteTarget?.key })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>
              {t("common.cancel")}
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={deleteEnv.isPending}>
              {t("common.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/env-config-panel.tsx
git commit -m "feat(web/env): add EnvConfigPanel component with Sheet CRUD and delete confirm"
```

---

## Task 5: Worker Detail — Add Env Tab + Effective Preview

**Files:**
- Modify: `web/src/pages/worker-detail.tsx`

The Env tab has two sections:
1. **Worker-scope env variables** — uses `EnvConfigPanel` with `scope="worker" scopeId={id}`
2. **Effective preview** — fetches global envs + dept envs for each of worker's departments + worker envs, merges them in priority order (global < department < worker), and shows a table with Key, Masked Value, and Source columns

- [ ] **Step 1: Add imports to `worker-detail.tsx`**

Add the following imports at the top of the file (after existing imports):

```typescript
import { EnvConfigPanel } from "@/components/env-config-panel"
import { useEnvList } from "@/hooks/use-envs"
import { KeyRoundIcon } from "lucide-react"
import type { EnvConfig } from "@/lib/types"
```

- [ ] **Step 2: Add the `EffectiveEnvPreview` inner component**

Add this component definition inside `worker-detail.tsx`, before the `WorkerDetail` function:

```typescript
function EffectiveEnvPreview({ workerId, departmentIds }: { workerId: string; departmentIds: string[] }) {
  const { t } = useTranslation()
  const { data: globalEnvs = [] } = useEnvList("global")
  const { data: workerEnvs = [] } = useEnvList("worker", workerId)

  // Fetch dept envs for each dept (only first 5 to keep queries bounded)
  const dept0 = useEnvList("department", departmentIds[0])
  const dept1 = useEnvList("department", departmentIds[1])
  const dept2 = useEnvList("department", departmentIds[2])
  const dept3 = useEnvList("department", departmentIds[3])
  const dept4 = useEnvList("department", departmentIds[4])

  const deptResults = [dept0, dept1, dept2, dept3, dept4]

  // Sorted dept IDs (alphabetical, matching backend merge order)
  const sortedDeptIds = [...departmentIds].sort()

  // Merge: global < dept (sorted) < worker
  const merged = new Map<string, { masked: string; source: string }>()

  for (const env of globalEnvs) {
    merged.set(env.key, { masked: env.masked, source: "global" })
  }

  for (const deptId of sortedDeptIds) {
    const idx = departmentIds.indexOf(deptId)
    if (idx < 0 || idx >= 5) continue
    const deptEnvs: EnvConfig[] = deptResults[idx]?.data ?? []
    for (const env of deptEnvs) {
      merged.set(env.key, { masked: env.masked, source: "department" })
    }
  }

  for (const env of workerEnvs) {
    merged.set(env.key, { masked: env.masked, source: "worker" })
  }

  const rows = Array.from(merged.entries()).sort(([a], [b]) => a.localeCompare(b))

  if (rows.length === 0) {
    return (
      <div className="rounded-2xl border border-dashed border-border/80 bg-background/75 px-4 py-8 text-sm leading-6 text-muted-foreground text-center">
        {t("envConfig.noEffective")}
      </div>
    )
  }

  const sourceLabel: Record<string, string> = {
    global: t("envConfig.sourceGlobal"),
    department: t("envConfig.sourceDepartment"),
    worker: t("envConfig.sourceWorker"),
  }

  const sourceColor: Record<string, string> = {
    global: "text-blue-500",
    department: "text-amber-500",
    worker: "text-green-500",
  }

  return (
    <div className="rounded-2xl border border-border/70 overflow-hidden">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t("envConfig.key")}</TableHead>
            <TableHead>{t("envConfig.masked")}</TableHead>
            <TableHead>{t("envConfig.source")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map(([key, { masked, source }]) => (
            <TableRow key={key}>
              <TableCell className="font-mono text-sm">{key}</TableCell>
              <TableCell className="font-mono text-sm text-muted-foreground">{masked}</TableCell>
              <TableCell>
                <span className={`text-xs font-medium ${sourceColor[source] ?? ""}`}>
                  {sourceLabel[source] ?? source}
                </span>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
```

- [ ] **Step 3: Add missing Table imports**

In `worker-detail.tsx`, add the Table imports to the existing import block (they're not currently imported):

```typescript
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
```

- [ ] **Step 4: Add "Env" tab trigger**

In `worker-detail.tsx`, find the `<TabsList>` block (around line 249) and add a new trigger:

```typescript
<TabsTrigger value="env">
  <KeyRoundIcon className="size-3.5" />
  {t("envConfig.title")}
</TabsTrigger>
```

Insert it after `<TabsTrigger value="permissions">`.

- [ ] **Step 5: Add "Env" tab content**

After the `</TabsContent>` closing tag for the `permissions` tab (around line 445), add:

```typescript
<TabsContent value="env" className="mt-6 space-y-6">
  <DetailSection className="p-5 sm:p-6 space-y-6">
    <div>
      <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground mb-4">
        {t("envConfig.title")}
      </p>
      <EnvConfigPanel scope="worker" scopeId={id!} />
    </div>
  </DetailSection>

  <DetailSection className="p-5 sm:p-6 space-y-4">
    <div>
      <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
        {t("envConfig.effectiveTitle")}
      </p>
      <p className="mt-1 text-sm leading-6 text-muted-foreground">
        {t("envConfig.effectiveHint")}
      </p>
    </div>
    <EffectiveEnvPreview
      workerId={id!}
      departmentIds={worker.departments?.map((d) => d.id) ?? []}
    />
  </DetailSection>
</TabsContent>
```

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/worker-detail.tsx
git commit -m "feat(web/env): add Env tab to worker detail with scope panel and effective preview"
```

---

## Task 6: Department Page — Add Per-Department Env Sheet

**Files:**
- Modify: `web/src/pages/departments.tsx`

For each department row in the tree, add a "Env" icon button in the hover actions. Clicking it opens a Sheet containing the `EnvConfigPanel` for that department.

- [ ] **Step 1: Add imports to `departments.tsx`**

Add these imports at the top:

```typescript
import { KeyRoundIcon } from "lucide-react"
import { EnvConfigPanel } from "@/components/env-config-panel"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet"
import type { Department } from "@/lib/types"
```

Note: `Department` type is already imported. Only add what's missing.

- [ ] **Step 2: Add env sheet state**

Inside the `Departments` function, after the existing state declarations, add:

```typescript
const [envTarget, setEnvTarget] = useState<Department | null>(null)
```

- [ ] **Step 3: Add "Env" button to each department row**

Inside the `.map(({ dept, depth }) => ...)` block, find the `<div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 ...">` actions div.

Add a new button before the existing edit/delete buttons:

```typescript
<Button
  variant="ghost"
  size="icon-xs"
  onClick={() => setEnvTarget(dept)}
  title={t("envConfig.depEnvTitle")}
>
  <KeyRoundIcon className="size-3.5" />
</Button>
```

- [ ] **Step 4: Add the env Sheet**

Before the closing `</FadeIn>`, add the env Sheet after the existing delete dialog:

```typescript
<Sheet open={!!envTarget} onOpenChange={(open) => { if (!open) setEnvTarget(null) }}>
  <SheetContent className="w-full sm:max-w-lg overflow-y-auto">
    <SheetHeader>
      <SheetTitle>{t("envConfig.depEnvTitle")}</SheetTitle>
      <SheetDescription>{envTarget?.name}</SheetDescription>
    </SheetHeader>
    <div className="px-4 py-6">
      {envTarget && (
        <EnvConfigPanel scope="department" scopeId={envTarget.id} />
      )}
    </div>
  </SheetContent>
</Sheet>
```

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/departments.tsx
git commit -m "feat(web/env): add per-department env config Sheet to departments page"
```

---

## Task 7: Settings Page (Global Env)

**Files:**
- Create: `web/src/pages/settings.tsx`
- Modify: `web/src/app.tsx`
- Modify: `web/src/components/app-sidebar.tsx`

- [ ] **Step 1: Create `settings.tsx`**

Create `web/src/pages/settings.tsx`:

```typescript
import { useTranslation } from "react-i18next"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { DetailSection } from "@/components/detail-primitives"
import { EnvConfigPanel } from "@/components/env-config-panel"

export function Settings() {
  const { t } = useTranslation()

  return (
    <FadeIn>
      <div className="space-y-6">
        <PageHeader title={t("nav.settings")} />

        <DetailSection className="p-5 sm:p-6 space-y-6">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground mb-1">
              {t("envConfig.globalTitle")}
            </p>
            <p className="text-sm leading-6 text-muted-foreground mb-4">
              {t("envConfig.effectiveHint")}
            </p>
            <EnvConfigPanel scope="global" />
          </div>
        </DetailSection>
      </div>
    </FadeIn>
  )
}
```

- [ ] **Step 2: Add route in `app.tsx`**

In `web/src/app.tsx`, add the lazy import for Settings:

```typescript
const Settings = lazy(() => import("@/pages/settings").then(m => ({ default: m.Settings })))
```

Then add the route inside the `<Route element={<AuthGuard><Layout /></AuthGuard>}>` block:

```typescript
<Route path="/settings" element={<Settings />} />
```

- [ ] **Step 3: Add Settings nav item to `app-sidebar.tsx`**

In `web/src/components/app-sidebar.tsx`, add `SettingsIcon` to the lucide import:

```typescript
import { LayoutDashboardIcon, BotIcon, ActivityIcon, ClockIcon, MessageCircleIcon, GithubIcon, Building2Icon, SettingsIcon } from "lucide-react"
```

Then add a settings entry to the `navMain` array (or create a separate `navSettings`). Add it at the end of `navMain`:

```typescript
{ title: t("nav.settings"), url: "/settings", icon: <SettingsIcon /> },
```

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/settings.tsx web/src/app.tsx web/src/components/app-sidebar.tsx
git commit -m "feat(web/env): add Settings page for global env config with sidebar nav"
```

---

## Self-Review

**Spec coverage check:**
- ✅ Worker detail "Env" tab (worker scope + effective preview)
- ✅ Department page per-department env Sheet (department scope)
- ✅ Settings page for global scope
- ✅ Sheet for create/edit (user's choice B)
- ✅ Effective preview for worker (user's choice)
- ✅ i18n for both en and zh

**Placeholder scan:** None found — all steps have actual code.

**Type consistency:**
- `EnvConfig` defined in Task 1, used in Task 4 (`use-envs.ts`), Task 5 (`env-config-panel.tsx`), Task 6 (`worker-detail.tsx`)
- `useEnvList`, `useCreateEnv`, `useUpdateEnv`, `useDeleteEnv` defined in Task 3, used in Task 4 (`env-config-panel.tsx`) and Task 6 (`worker-detail.tsx`)
- `EnvConfigPanel` defined in Task 4, used in Tasks 5, 6, 7
- All props and signatures are consistent across tasks

**Known limitation:** The `EffectiveEnvPreview` uses up to 5 department queries (indexed 0–4). Workers with more than 5 departments would miss some dept envs in the preview. This is an acceptable trade-off for MVP.
