# Department Management Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a dedicated `/departments` page for department CRUD, introduce a "通讯录" nav group containing Workers and Departments, and remove the standalone department components.

**Architecture:** Create a new `pages/departments.tsx` with all department management logic inlined (tree list + create/edit/delete dialogs). Remove `department-dialog.tsx` entirely and strip the `onManage` button from `department-tree.tsx`. Update the sidebar to render two `NavMain` groups.

**Tech Stack:** React 19, TypeScript, TanStack Query v5, React Router DOM v7, Tailwind CSS v4, i18next, Lucide React icons

> **Note:** This codebase has no frontend test infrastructure. Verification steps use `npm run dev` and manual browser checks instead of automated tests.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `web/src/locales/en.json` | Modify | Add `nav.departments`, `nav.directory` keys |
| `web/src/locales/zh.json` | Modify | Add `nav.departments`, `nav.directory` keys |
| `web/src/components/department-tree.tsx` | Modify | Remove `onManage` prop and bottom button |
| `web/src/pages/workers.tsx` | Modify | Remove `DepartmentManageDialog` import/usage |
| `web/src/components/department-dialog.tsx` | Delete | No longer needed |
| `web/src/pages/departments.tsx` | Create | Full department management page |
| `web/src/components/app-sidebar.tsx` | Modify | Add "通讯录" nav group |
| `web/src/app.tsx` | Modify | Register `/departments` route |

---

## Task 1: Add i18n keys

**Files:**
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`

- [ ] **Step 1: Add keys to en.json**

In `web/src/locales/en.json`, find the `"nav"` object and add two new keys:

```json
"nav": {
  "dashboard": "Dashboard",
  "workers": "Workers",
  "executions": "Sessions",
  "tasks": "Scheduled Tasks",
  "departments": "Departments",
  "directory": "Directory"
},
```

- [ ] **Step 2: Add keys to zh.json**

In `web/src/locales/zh.json`, find the `"nav"` object and add two new keys:

```json
"nav": {
  "dashboard": "仪表板",
  "workers": "员工",
  "executions": "会话",
  "tasks": "定时任务",
  "departments": "部门",
  "directory": "通讯录"
},
```

- [ ] **Step 3: Commit**

```bash
git add web/src/locales/en.json web/src/locales/zh.json
git commit -m "feat: add i18n keys for department page and directory nav group"
```

---

## Task 2: Remove `onManage` from DepartmentTreeSidebar

**Files:**
- Modify: `web/src/components/department-tree.tsx`

- [ ] **Step 1: Remove `onManage` from the interface and function signature**

Replace the `DepartmentTreeProps` interface and `DepartmentTreeSidebar` signature. Current code at lines 9–16:

```typescript
// BEFORE
interface DepartmentTreeProps {
  departments: DepartmentTreeType[]
  selectedId: string | null
  onSelect: (id: string | null) => void
  onManage: () => void
}

export function DepartmentTreeSidebar({ departments, selectedId, onSelect, onManage }: DepartmentTreeProps) {
```

Replace with:

```typescript
// AFTER
interface DepartmentTreeProps {
  departments: DepartmentTreeType[]
  selectedId: string | null
  onSelect: (id: string | null) => void
}

export function DepartmentTreeSidebar({ departments, selectedId, onSelect }: DepartmentTreeProps) {
```

- [ ] **Step 2: Remove the bottom "管理部门" button block**

Delete the closing `<div>` block at the bottom of `DepartmentTreeSidebar` (lines 66–73):

```typescript
// DELETE this entire block:
      <div className="border-t px-3 py-2">
        <button
          onClick={onManage}
          className="w-full text-xs text-muted-foreground hover:text-foreground transition-colors text-center py-1"
        >
          {t("departments.manage")}
        </button>
      </div>
```

The `return` block should now end after the `<div className="flex-1 overflow-y-auto ...">` closing tag:

```typescript
  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto py-2 space-y-0.5">
        {/* ... all existing filter buttons and tree nodes ... */}
      </div>
    </div>
  )
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/department-tree.tsx
git commit -m "feat: remove onManage from DepartmentTreeSidebar"
```

---

## Task 3: Clean up workers.tsx

**Files:**
- Modify: `web/src/pages/workers.tsx`

- [ ] **Step 1: Remove DepartmentManageDialog import**

Delete line 9:

```typescript
// DELETE this line:
import { DepartmentManageDialog } from "@/components/department-dialog"
```

- [ ] **Step 2: Remove manageDeptOpen state**

In the `Workers` component body, find and delete:

```typescript
// DELETE this line:
const [manageDeptOpen, setManageDeptOpen] = useState(false)
```

- [ ] **Step 3: Remove onManage prop from DepartmentTreeSidebar**

Find the `<DepartmentTreeSidebar ... />` JSX and remove the `onManage` prop:

```typescript
// BEFORE:
<DepartmentTreeSidebar
  departments={departments}
  selectedId={selectedDeptId}
  onSelect={setSelectedDeptId}
  onManage={() => setManageDeptOpen(true)}
/>

// AFTER:
<DepartmentTreeSidebar
  departments={departments}
  selectedId={selectedDeptId}
  onSelect={setSelectedDeptId}
/>
```

- [ ] **Step 4: Remove DepartmentManageDialog JSX**

Search for `<DepartmentManageDialog` in the file and delete the entire element:

```typescript
// DELETE this block (wherever it appears in the JSX):
<DepartmentManageDialog open={manageDeptOpen} onOpenChange={setManageDeptOpen} />
```

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/workers.tsx
git commit -m "feat: remove department manage dialog from workers page"
```

---

## Task 4: Delete department-dialog.tsx

**Files:**
- Delete: `web/src/components/department-dialog.tsx`

- [ ] **Step 1: Delete the file**

```bash
git rm web/src/components/department-dialog.tsx
```

- [ ] **Step 2: Commit**

```bash
git commit -m "feat: remove standalone department-dialog component"
```

---

## Task 5: Create the Departments page

**Files:**
- Create: `web/src/pages/departments.tsx`

- [ ] **Step 1: Create the file with full implementation**

Create `web/src/pages/departments.tsx` with the following content:

```typescript
import { useMemo, useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { PlusIcon, PencilIcon, Trash2Icon, FolderIcon, ChevronRightIcon } from "lucide-react"
import { useDepartments, useCreateDepartment, useUpdateDepartment, useDeleteDepartment } from "@/hooks/use-departments"
import { flattenDeptTree } from "@/lib/department-utils"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { EmptyState } from "@/components/empty-state"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"
import type { Department } from "@/lib/types"

const NO_PARENT_VALUE = "__no_parent__"

type Mode = "idle" | "create" | "edit" | "delete"

export function Departments() {
  const { t } = useTranslation()
  const { data: departments = [] } = useDepartments()
  const createDept = useCreateDepartment()
  const updateDept = useUpdateDepartment()
  const deleteDept = useDeleteDepartment()

  const flatDepts = useMemo(() => flattenDeptTree(departments), [departments])

  const [mode, setMode] = useState<Mode>("idle")
  const [editingDept, setEditingDept] = useState<Department | null>(null)
  const [deletingDept, setDeletingDept] = useState<Department | null>(null)
  const [formName, setFormName] = useState("")
  const [formParentId, setFormParentId] = useState<string | null>(null)
  const [error, setError] = useState("")

  const resetForm = () => {
    setMode("idle")
    setEditingDept(null)
    setDeletingDept(null)
    setFormName("")
    setFormParentId(null)
    setError("")
  }

  const openCreate = (parentId?: string | null) => {
    setFormName("")
    setFormParentId(parentId ?? null)
    setError("")
    setMode("create")
  }

  const openEdit = (dept: Department) => {
    setEditingDept(dept)
    setFormName(dept.name)
    setFormParentId(dept.parent_id)
    setError("")
    setMode("edit")
  }

  const openDelete = (dept: Department) => {
    setDeletingDept(dept)
    setError("")
    setMode("delete")
  }

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault()
    if (!formName.trim()) return
    try {
      await createDept.mutateAsync({ name: formName.trim(), parent_id: formParentId })
      resetForm()
    } catch (err: any) {
      setError(err.message)
    }
  }

  const handleUpdate = async (e: FormEvent) => {
    e.preventDefault()
    if (!editingDept || !formName.trim()) return
    try {
      await updateDept.mutateAsync({
        id: editingDept.id,
        data: { name: formName.trim(), parent_id: formParentId },
      })
      resetForm()
    } catch (err: any) {
      setError(err.message)
    }
  }

  const handleDelete = async () => {
    if (!deletingDept) return
    try {
      await deleteDept.mutateAsync(deletingDept.id)
      resetForm()
    } catch (err: any) {
      setError(err.message)
    }
  }

  const isFormOpen = mode === "create" || mode === "edit"
  const isDeleteOpen = mode === "delete"

  return (
    <FadeIn>
      <div className="space-y-6">
        <PageHeader
          title={t("nav.departments")}
          actions={
            <Button onClick={() => openCreate()}>
              <PlusIcon className="size-4 mr-1" />
              {t("departments.create")}
            </Button>
          }
        />

        {flatDepts.length === 0 ? (
          <EmptyState
            title={t("departments.empty")}
            action={
              <Button onClick={() => openCreate()}>
                <PlusIcon className="size-4 mr-1" />
                {t("departments.create")}
              </Button>
            }
          />
        ) : (
          <div className="rounded-lg border border-border">
            {flatDepts.map(({ dept, depth }) => (
              <div
                key={dept.id}
                className="flex items-center gap-2 px-3 py-2.5 border-b border-border/60 last:border-b-0 hover:bg-muted/50 group transition-colors"
                style={{ paddingLeft: `${depth * 20 + 12}px` }}
              >
                {depth > 0 && (
                  <ChevronRightIcon className="size-3 shrink-0 text-muted-foreground/50" />
                )}
                <FolderIcon className="size-4 shrink-0 text-muted-foreground" />
                <span className="flex-1 text-sm">{dept.name}</span>
                <div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => openCreate(dept.id)}
                    title={t("departments.addChild")}
                  >
                    <PlusIcon className="size-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => openEdit(dept)}
                    title={t("departments.rename")}
                  >
                    <PencilIcon className="size-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => openDelete(dept)}
                    title={t("common.delete")}
                  >
                    <Trash2Icon className="size-3.5" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Create / Edit Dialog */}
        <Dialog open={isFormOpen} onOpenChange={(open) => { if (!open) resetForm() }}>
          <DialogContent className="max-w-md">
            <DialogHeader>
              <DialogTitle>
                {mode === "create" ? t("departments.create") : t("departments.rename")}
              </DialogTitle>
              <DialogDescription>{t("departments.manageDescription")}</DialogDescription>
            </DialogHeader>
            <form onSubmit={mode === "create" ? handleCreate : handleUpdate} className="space-y-4">
              {error && <p className="text-sm text-destructive">{error}</p>}
              <div className="space-y-1.5">
                <Label htmlFor="dept-name">{t("departments.form.name")}</Label>
                <Input
                  id="dept-name"
                  value={formName}
                  onChange={(e) => setFormName(e.target.value)}
                  placeholder={t("departments.form.namePlaceholder")}
                  required
                  autoFocus
                />
              </div>
              <div className="space-y-1.5">
                <Label>{t("departments.form.parent")}</Label>
                <Select
                  value={formParentId ?? NO_PARENT_VALUE}
                  onValueChange={(v) => setFormParentId(v === NO_PARENT_VALUE ? null : v)}
                >
                  <SelectTrigger>
                    <SelectValue>
                      {(value: string | null) =>
                        !value || value === NO_PARENT_VALUE
                          ? t("departments.form.noParent")
                          : flatDepts.find(({ dept }) => dept.id === value)?.dept.name ?? value
                      }
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NO_PARENT_VALUE}>{t("departments.form.noParent")}</SelectItem>
                    {flatDepts
                      .filter(({ dept }) => dept.id !== editingDept?.id)
                      .map(({ dept, depth }) => (
                        <SelectItem key={dept.id} value={dept.id}>
                          <span style={{ paddingLeft: `${depth * 12}px` }}>{dept.name}</span>
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={resetForm}>
                  {t("common.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={!formName.trim() || createDept.isPending || updateDept.isPending}
                >
                  {mode === "create" ? t("departments.create") : t("common.save")}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>

        {/* Delete Confirmation Dialog */}
        <Dialog open={isDeleteOpen} onOpenChange={(open) => { if (!open) resetForm() }}>
          <DialogContent className="max-w-sm">
            <DialogHeader>
              <DialogTitle>{t("departments.deleteConfirm.title")}</DialogTitle>
            </DialogHeader>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <p className="text-sm text-muted-foreground">
              {t("departments.deleteConfirm.description", { name: deletingDept?.name })}
            </p>
            <DialogFooter>
              <Button variant="outline" onClick={resetForm}>
                {t("common.cancel")}
              </Button>
              <Button
                variant="destructive"
                onClick={handleDelete}
                disabled={deleteDept.isPending}
              >
                {t("common.delete")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </FadeIn>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/pages/departments.tsx
git commit -m "feat: add department management page"
```

---

## Task 6: Update app-sidebar.tsx

**Files:**
- Modify: `web/src/components/app-sidebar.tsx`

- [ ] **Step 1: Add Building2Icon import**

In `app-sidebar.tsx`, find the lucide-react import line:

```typescript
// BEFORE:
import { LayoutDashboardIcon, BotIcon, ActivityIcon, ClockIcon, MessageCircleIcon, GithubIcon } from "lucide-react"

// AFTER:
import { LayoutDashboardIcon, BotIcon, ActivityIcon, ClockIcon, MessageCircleIcon, GithubIcon, Building2Icon } from "lucide-react"
```

- [ ] **Step 2: Split navMain into two groups**

Replace the existing `navMain` useMemo block:

```typescript
// BEFORE:
  const navMain = React.useMemo(() => [
    { title: t("nav.dashboard"), url: "/", icon: <LayoutDashboardIcon /> },
    { title: t("localChat.title"), url: "/chat", icon: <MessageCircleIcon /> },
    { title: t("nav.workers"), url: "/workers", icon: <BotIcon /> },
    { title: t("nav.executions"), url: "/sessions", icon: <ActivityIcon /> },
    { title: t("nav.tasks"), url: "/tasks", icon: <ClockIcon /> },
  ], [t])

// AFTER:
  const navDirectory = React.useMemo(() => [
    { title: t("nav.workers"), url: "/workers", icon: <BotIcon /> },
    { title: t("nav.departments"), url: "/departments", icon: <Building2Icon /> },
  ], [t])

  const navMain = React.useMemo(() => [
    { title: t("nav.dashboard"), url: "/", icon: <LayoutDashboardIcon /> },
    { title: t("localChat.title"), url: "/chat", icon: <MessageCircleIcon /> },
    { title: t("nav.executions"), url: "/sessions", icon: <ActivityIcon /> },
    { title: t("nav.tasks"), url: "/tasks", icon: <ClockIcon /> },
  ], [t])
```

- [ ] **Step 3: Render two NavMain groups in SidebarContent**

Find the `<SidebarContent>` block:

```typescript
// BEFORE:
      <SidebarContent>
        <NavMain items={navMain} />
        <NavSecondary items={navSecondary} className="mt-auto" />
      </SidebarContent>

// AFTER:
      <SidebarContent>
        <NavMain label={t("nav.directory")} items={navDirectory} />
        <NavMain items={navMain} />
        <NavSecondary items={navSecondary} className="mt-auto" />
      </SidebarContent>
```

- [ ] **Step 4: Commit**

```bash
git add web/src/components/app-sidebar.tsx
git commit -m "feat: add 通讯录 nav group with workers and departments"
```

---

## Task 7: Register /departments route in app.tsx

**Files:**
- Modify: `web/src/app.tsx`

- [ ] **Step 1: Add lazy import for Departments page**

After the existing lazy imports, add:

```typescript
// After existing lazy imports, add:
const Departments = lazy(() => import("@/pages/departments").then(m => ({ default: m.Departments })))
```

- [ ] **Step 2: Add route**

Inside the `<Route element={<AuthGuard><Layout /></AuthGuard>}>` block, add the new route after the workers routes:

```typescript
// BEFORE (inside AuthGuard Layout block):
                <Route path="/workers" element={<Workers />} />
                <Route path="/workers/:id" element={<WorkerDetail />} />

// AFTER:
                <Route path="/workers" element={<Workers />} />
                <Route path="/workers/:id" element={<WorkerDetail />} />
                <Route path="/departments" element={<Departments />} />
```

- [ ] **Step 3: Commit**

```bash
git add web/src/app.tsx
git commit -m "feat: register /departments route"
```

---

## Task 8: Manual verification

- [ ] **Step 1: Start the dev server**

```bash
cd web && npm run dev
```

- [ ] **Step 2: Verify sidebar**

Open the app in the browser. Check that:
- Left sidebar shows a "通讯录" group label
- "Workers" and "部门" (Departments) are under the group
- Dashboard, Local Chat, Sessions, Scheduled Tasks remain ungrouped below

- [ ] **Step 3: Verify Workers page**

Navigate to `/workers`:
- Department tree sidebar still shows on the left with department filters
- The "管理部门" button at the bottom of the sidebar is gone
- Creating/filtering workers still works

- [ ] **Step 4: Verify Departments page**

Navigate to `/departments`:
- Page shows "部门" title with a "+ 新建部门" button
- If no departments: empty state with action button
- If departments exist: indented tree list with folder icons
- Hover a row: ➕ / ✏️ / 🗑️ buttons appear
- Click ➕: create dialog opens with parent pre-filled
- Click ✏️: edit dialog opens with name and parent pre-filled
- Click 🗑️: delete confirmation dialog opens
- Attempting to delete a department with children shows error from backend

- [ ] **Step 5: Verify Worker Detail page**

Navigate to a worker's detail page:
- Department badges still show
- "管理部门" button still works (assigns worker to departments via checkboxes)

- [ ] **Step 6: Final commit if any fixes needed, then done**

```bash
git add -p  # stage any fix-up changes
git commit -m "fix: address issues found during manual verification"
```
