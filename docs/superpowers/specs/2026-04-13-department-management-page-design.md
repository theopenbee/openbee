# Department Management Page Design

**Date:** 2026-04-13
**Branch:** feature/env-config
**Status:** Approved

---

## Overview

Add a dedicated Department Management page (`/departments`) to the web app for creating, editing, and deleting departments in a tree-list layout. Reorganize the left navigation by introducing a "通讯录" (Directory) group containing both the Workers and Departments pages. Remove the existing standalone department components (`department-dialog.tsx`, `department-tree.tsx`) and inline all department management logic directly in the new page.

---

## Requirements

- **Scope:** Lightweight — department CRUD only. Worker-department assignment remains in the Worker Detail page.
- **Layout:** Pure tree list — no left/right split. Full-page tree with action buttons per row.
- **Navigation:** Independent top-level page under a new "通讯录" nav group alongside Workers.
- **Component strategy:** No standalone department components; all logic lives in `pages/departments.tsx`.

---

## Architecture

### Files Changed

| File | Change | Notes |
|------|--------|-------|
| `web/src/components/department-dialog.tsx` | **Delete** | Replaced by inline logic in new page |
| `web/src/components/department-tree.tsx` | **Modify** | Remove `onManage` prop and bottom "管理部门" button |
| `web/src/pages/workers.tsx` | **Modify** | Remove `DepartmentManageDialog` import, `manageDeptOpen` state, `onManage` prop |
| `web/src/pages/departments.tsx` | **Create** | New department management page |
| `web/src/components/app-sidebar.tsx` | **Modify** | Add "通讯录" nav group; move Workers into it; add Departments link |
| `web/src/app.tsx` | **Modify** | Add `/departments` route |

### Files Not Changed

| File | Reason |
|------|--------|
| `web/src/pages/worker-detail.tsx` | Inline dept assignment dialog (checkboxes) is worker-dept association management, not dept CRUD — stays as-is |
| `web/src/hooks/use-departments.ts` | No changes needed |
| `web/src/lib/department-utils.ts` | No changes needed |
| All backend files | No changes needed |

---

## Navigation Structure

**Before:**
```
(no group label)
  Dashboard
  Local Chat
  Workers
  Executions
  Tasks
```

**After:**
```
通讯录
  Workers
  Departments  ← new

(no group label)
  Dashboard
  Local Chat
  Executions
  Tasks
```

The `NavMain` component already supports an optional `label` prop for group labels. The sidebar will render two `NavMain` instances: one with the "通讯录" label and one without for the remaining items.

---

## Department Management Page (`/departments`)

### Layout

```
┌─────────────────────────────────────────────────┐
│ Page Header: "部门管理"           [+ 新建部门]    │
├─────────────────────────────────────────────────┤
│                                                 │
│  📁 Engineering                  [+] [✏] [🗑]  │
│    📁 Frontend                   [+] [✏] [🗑]  │
│    📁 Backend                    [+] [✏] [🗑]  │
│  📁 Product                      [+] [✏] [🗑]  │
│                                                 │
│  (empty state if no departments)                │
└─────────────────────────────────────────────────┘
```

### Tree List Behavior

- Departments are rendered as a flat-ish indented list using `flattenDeptTree` from `department-utils.ts`
- Each row shows: indent (16px per level) + folder icon + dept name + action buttons on hover
- Action buttons per row:
  - **＋** — create child department (pre-fills parent)
  - **✏** — edit (name + parent)
  - **🗑** — delete with confirmation

### Create / Edit Dialog

Inline `<Dialog>` within the page (not a separate component). Fields:
- **Name** — required text input
- **Parent department** — `<Select>` dropdown, options built from `flattenDeptTree` with indentation, default "无上级"

On submit: call `useCreateDepartment` or `useUpdateDepartment` mutation; dialog closes on success.

### Delete Confirmation Dialog

Separate inline `<Dialog>` for deletion. Shows the department name and asks for confirmation. On confirm: call `useDeleteDepartment`.

Note: The backend rejects deletion if the department has sub-departments or associated workers, returning an error such as "department is not empty: has sub-departments". This error is displayed inline in the dialog — no pre-emptive warning is needed in the UI.

### State

```typescript
type Mode = "idle" | "create" | "edit" | "delete"
// mode: controls which dialog is open
// editingDept: Department | null
// deletingDept: Department | null
// formName: string
// formParentId: string | null
// error: string
```

### Empty State

When no departments exist, show `<EmptyState>` with a "新建部门" action button.

---

## Workers Page Changes

Remove:
- `import { DepartmentManageDialog } from "@/components/department-dialog"`
- `const [manageDeptOpen, setManageDeptOpen] = useState(false)`
- `<DepartmentManageDialog open={manageDeptOpen} onOpenChange={setManageDeptOpen} />`
- `onManage={() => setManageDeptOpen(true)}` prop on `<DepartmentTreeSidebar>`

The department filter sidebar (`DepartmentTreeSidebar`) stays as-is, just without the manage callback.

---

## DepartmentTree Component Changes

Remove:
- `onManage: () => void` from the `DepartmentTreeProps` interface
- The `onManage` parameter from the function signature
- The bottom `<div className="border-t ...">` block containing the "管理部门" button

---

## App Sidebar Changes

Split current single `navMain` array into two groups:

```typescript
const navDirectory = [
  { title: t("nav.workers"), url: "/workers", icon: <BotIcon /> },
  { title: t("nav.departments"), url: "/departments", icon: <Building2Icon /> },
]

const navMain = [
  { title: t("nav.dashboard"), url: "/", icon: <LayoutDashboardIcon /> },
  { title: t("localChat.title"), url: "/chat", icon: <MessageCircleIcon /> },
  { title: t("nav.executions"), url: "/sessions", icon: <ActivityIcon /> },
  { title: t("nav.tasks"), url: "/tasks", icon: <ClockIcon /> },
]
```

Render two `<NavMain>` instances in `<SidebarContent>`:

```tsx
<NavMain label={t("nav.directory")} items={navDirectory} />
<NavMain items={navMain} />
```

---

## i18n Keys Required

Add to `en.json` and `zh.json`:

```json
// en.json additions
"nav": {
  "departments": "Departments",
  "directory": "Directory"
}

// zh.json additions
"nav": {
  "departments": "部门",
  "directory": "通讯录"
}
```

---

## Data Flow

```
departments.tsx
  └─ useDepartments()          → GET /api/departments (tree)
  └─ useCreateDepartment()     → POST /api/departments
  └─ useUpdateDepartment()     → PUT /api/departments/:id
  └─ useDeleteDepartment()     → DELETE /api/departments/:id
  └─ flattenDeptTree()         → utility: tree → flat list with depth
```

All hooks already exist in `use-departments.ts`. No new hooks or API changes needed.

---

## Error Handling

- Mutation errors displayed inline above the dialog footer as `<p className="text-sm text-destructive">` — consistent with existing Workers page pattern.
- Loading states: disable submit button while mutation is pending (`isPending`).

---

## Out of Scope

- Drag-and-drop reordering of departments
- Bulk operations
- Worker listing within the departments page (use Workers page filter for that)
- Department search/filter
