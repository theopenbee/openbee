# Worker Permission Scopes — Web UI Design

**Date:** 2026-04-12
**Branch:** feature/worker-permission-scopes

---

## Background

The backend already fully supports `permission_scopes` on the Worker model:

- `Worker.PermissionScopes` is a comma-separated string (e.g. `read:workers,read:tasks`)
- The Create and Update REST APIs both accept `permission_scopes`
- Three scopes are currently defined in `internal/infra/auth/scopes.go`:
  - `read:workers` — required to list/get Worker info via MCP tools
  - `read:departments` — required to list/get Department info via MCP tools
  - `read:tasks` — required to list/get Task info via MCP tools
- When `permission_scopes` is empty, Worker tokens are **denied** access to all system built-in tools (tools listed in `auth.ToolScopeMap`)

The Web admin UI currently exposes **none** of this. This spec covers adding permission configuration to the Web UI.

---

## Goals

1. Let admins configure `permission_scopes` when **creating** a Worker
2. Let admins view and edit `permission_scopes` on the **Worker Detail** page
3. Use a toggle-card UI (one card per scope) for clarity — admins see what each scope does at a glance
4. When no scopes are configured, display a warning that the token is denied access to all system built-in tools

---

## Non-Goals

- Adding a backend API to expose the scope list (hardcoded on the frontend for now)
- Changing backend behavior (empty scopes = denied for system built-in tools, already the current behavior)
- Supporting custom/arbitrary scopes outside the known set

---

## Data Layer

### New file: `web/src/lib/scopes.ts`

```ts
export interface ScopeDef {
  id: string
  titleKey: string        // i18n key
  descriptionKey: string  // i18n key
}

export const KNOWN_SCOPES: ScopeDef[] = [
  {
    id: "read:workers",
    titleKey: "scopes.readWorkers.title",
    descriptionKey: "scopes.readWorkers.description",
  },
  {
    id: "read:departments",
    titleKey: "scopes.readDepartments.title",
    descriptionKey: "scopes.readDepartments.description",
  },
  {
    id: "read:tasks",
    titleKey: "scopes.readTasks.title",
    descriptionKey: "scopes.readTasks.description",
  },
]

export function parseScopes(raw: string): string[] {
  return raw ? raw.split(",").map((s) => s.trim()).filter(Boolean) : []
}

export function serializeScopes(scopes: string[]): string {
  return scopes.join(",")
}
```

No changes needed to existing hooks — `useUpdateWorker` already accepts `permission_scopes`.

---

## New Component: `ScopeToggleCard`

**File:** `web/src/components/scope-toggle-card.tsx`

Props:
- `scope: ScopeDef`
- `checked: boolean`
- `onToggle: (id: string, checked: boolean) => void`
- `disabled?: boolean`

Each card renders:
- Left: icon placeholder + title (from i18n) + description (from i18n)
- Right: a `Switch` toggle

This component is shared between the Create form and the Detail page tab.

---

## Create Worker Form (`workers.tsx`)

Add a new **Permissions** section between the Config section and the Department section:

```
─── PERMISSIONS ──────────────────────────
┌────────────────────────────────────────┐
│  查询 Worker                  [Toggle] │
│  允许列出和查看 Worker 的基本信息与状态  │
├────────────────────────────────────────┤
│  查询部门                     [Toggle] │
│  允许列出和查看部门信息                 │
├────────────────────────────────────────┤
│  查询任务                     [Toggle] │
│  允许列出和查看任务记录                 │
└────────────────────────────────────────┘
helper: 未勾选任何权限时，Worker token 禁止访问任何系统内置工具
```

State: `const [selectedScopes, setSelectedScopes] = useState<string[]>([])`

On submit: include `permission_scopes: serializeScopes(selectedScopes)` in the `createWorker.mutateAsync(...)` payload.

After successful creation, reset `selectedScopes` to `[]`.

---

## Worker Detail Page — Permissions Tab (`worker-detail.tsx`)

Add a fourth tab `Permissions` alongside Sessions / Tasks / Memory.

**Tab content:**

1. **Warning banner** (shown only when the **saved** `worker.permission_scopes` is empty — based on server data, not local unsaved state):
   > "未配置任何权限，Worker token 将禁止访问任何系统内置工具。请根据需要开启对应权限。"

2. **Toggle cards** — one per scope in `KNOWN_SCOPES`, initialized from `parseScopes(worker.permission_scopes)`

3. **Save button** — appears only when local state differs from the saved value. On click: call `updateWorker.mutateAsync({ id, data: { permission_scopes: serializeScopes(localScopes) } })`. After success: show a brief "已保存" toast/indicator and hide the button.

State:
```ts
const [localScopes, setLocalScopes] = useState<string[]>(() =>
  parseScopes(worker.permission_scopes)
)
const isDirty = serializeScopes(localScopes) !== worker.permission_scopes
```

Re-initialize `localScopes` when `worker.permission_scopes` changes (e.g. after a successful save).

---

## i18n Keys

Add to both `locales/zh.json` and `locales/en.json`:

```json
{
  "scopes.readWorkers.title": "查询 Worker",
  "scopes.readWorkers.description": "允许列出和查看 Worker 的基本信息与状态",
  "scopes.readDepartments.title": "查询部门",
  "scopes.readDepartments.description": "允许列出和查看部门信息",
  "scopes.readTasks.title": "查询任务",
  "scopes.readTasks.description": "允许列出和查看任务记录",

  "workers.form.sectionPermissions": "权限配置",
  "workers.form.permissionsHelper": "未勾选任何权限时，Worker token 禁止访问任何系统内置工具",

  "workerDetail.permissions": "权限",
  "workerDetail.permissionsEmpty": "未配置任何权限，Worker token 将禁止访问任何系统内置工具。请根据需要开启对应权限。",
  "workerDetail.permissionsSaved": "已保存"
}
```

English equivalents:
```json
{
  "scopes.readWorkers.title": "Read Workers",
  "scopes.readWorkers.description": "Allows listing and viewing Worker information and status",
  "scopes.readDepartments.title": "Read Departments",
  "scopes.readDepartments.description": "Allows listing and viewing department information",
  "scopes.readTasks.title": "Read Tasks",
  "scopes.readTasks.description": "Allows listing and viewing task records",

  "workers.form.sectionPermissions": "Permissions",
  "workers.form.permissionsHelper": "When no permissions are selected, the Worker token is denied access to all system built-in tools",

  "workerDetail.permissions": "Permissions",
  "workerDetail.permissionsEmpty": "No permissions configured. The Worker token will be denied access to all system built-in tools. Enable the permissions your Worker needs.",
  "workerDetail.permissionsSaved": "Saved"
}
```

---

## Files Changed

| File | Change |
|------|--------|
| `web/src/lib/scopes.ts` | New — scope definitions, parse/serialize helpers |
| `web/src/components/scope-toggle-card.tsx` | New — reusable toggle card component |
| `web/src/pages/workers.tsx` | Add Permissions section to Create form |
| `web/src/pages/worker-detail.tsx` | Add Permissions tab |
| `web/src/locales/zh.json` | Add scope/permissions i18n keys |
| `web/src/locales/en.json` | Add scope/permissions i18n keys |

Backend: **no changes required**.

---

## Error Handling

- If `updateWorker` fails on the Permissions tab, show the existing error display pattern (the tab stays dirty so the user can retry)
- The Save button shows a loading spinner while the mutation is in flight (`updateWorker.isPending`)
