# Worker Permission Scopes Web UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add permission scope configuration to the Web admin UI — a toggle-card panel in both the Create Worker form and a new Worker Detail "Permissions" tab.

**Architecture:** Add a `lib/scopes.ts` data layer, a reusable `ScopeToggleCard` component backed by a new `ui/switch.tsx`, then wire both into the Create Worker sheet and a new Permissions tab in the Worker Detail page. Backend requires no changes.

**Tech Stack:** React 19, TypeScript, `@base-ui/react/switch`, `react-i18next`, Tailwind CSS v4, Vitest

---

## File Map

| File | Status | Responsibility |
|------|--------|----------------|
| `web/src/lib/scopes.ts` | Create | Scope constant list + parse/serialize helpers |
| `web/src/lib/__tests__/scopes.test.ts` | Create | Unit tests for parse/serialize helpers |
| `web/src/components/ui/switch.tsx` | Create | Styled Switch primitive wrapping `@base-ui/react/switch` |
| `web/src/components/scope-toggle-card.tsx` | Create | Reusable toggle card for a single scope |
| `web/src/locales/zh.json` | Modify | Add scope + permissions i18n keys |
| `web/src/locales/en.json` | Modify | Add scope + permissions i18n keys |
| `web/src/pages/workers.tsx` | Modify | Add Permissions section to Create Worker sheet |
| `web/src/pages/worker-detail.tsx` | Modify | Add Permissions tab |

---

## Task 1: Scope data layer + tests

**Files:**
- Create: `web/src/lib/scopes.ts`
- Create: `web/src/lib/__tests__/scopes.test.ts`

- [ ] **Step 1: Write the failing tests**

Create `web/src/lib/__tests__/scopes.test.ts`:

```ts
import { describe, expect, it } from "vitest"
import { parseScopes, serializeScopes, KNOWN_SCOPES } from "../scopes"

describe("parseScopes", () => {
  it("returns empty array for empty string", () => {
    expect(parseScopes("")).toEqual([])
  })

  it("splits comma-separated scopes", () => {
    expect(parseScopes("read:workers,read:tasks")).toEqual(["read:workers", "read:tasks"])
  })

  it("trims whitespace around entries", () => {
    expect(parseScopes("read:workers, read:tasks")).toEqual(["read:workers", "read:tasks"])
  })

  it("filters out blank entries", () => {
    expect(parseScopes(",read:workers,")).toEqual(["read:workers"])
  })
})

describe("serializeScopes", () => {
  it("joins scopes with comma", () => {
    expect(serializeScopes(["read:workers", "read:tasks"])).toBe("read:workers,read:tasks")
  })

  it("returns empty string for empty array", () => {
    expect(serializeScopes([])).toBe("")
  })
})

describe("KNOWN_SCOPES", () => {
  it("contains exactly 3 entries", () => {
    expect(KNOWN_SCOPES).toHaveLength(3)
  })

  it("all entries have id, titleKey, descriptionKey", () => {
    for (const s of KNOWN_SCOPES) {
      expect(s.id).toBeTruthy()
      expect(s.titleKey).toBeTruthy()
      expect(s.descriptionKey).toBeTruthy()
    }
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd web && npx vitest run src/lib/__tests__/scopes.test.ts
```

Expected: FAIL — "Cannot find module '../scopes'"

- [ ] **Step 3: Create `web/src/lib/scopes.ts`**

```ts
export interface ScopeDef {
  id: string
  titleKey: string
  descriptionKey: string
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

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd web && npx vitest run src/lib/__tests__/scopes.test.ts
```

Expected: PASS — 7 tests

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/scopes.ts web/src/lib/__tests__/scopes.test.ts
git commit -m "feat: add scope data layer with parse/serialize helpers"
```

---

## Task 2: i18n keys

**Files:**
- Modify: `web/src/locales/zh.json`
- Modify: `web/src/locales/en.json`

- [ ] **Step 1: Add keys to `web/src/locales/zh.json`**

In `zh.json`, add a top-level `"scopes"` key before the closing `}` of the root object, and extend `"workerDetail"` and `"workers"."form"`:

Add top-level `"scopes"` block (before the closing `}` of root, after `"emptyState"`):

```json
  "scopes": {
    "readWorkers": {
      "title": "查询 Worker",
      "description": "允许列出和查看 Worker 的基本信息与状态"
    },
    "readDepartments": {
      "title": "查询部门",
      "description": "允许列出和查看部门信息"
    },
    "readTasks": {
      "title": "查询任务",
      "description": "允许列出和查看任务记录"
    }
  }
```

In `"workers"."form"` object, after `"departmentHelper"`:

```json
      "sectionPermissions": "权限配置",
      "permissionsHelper": "未勾选任何权限时，Worker token 禁止访问任何系统内置工具"
```

In `"workerDetail"` object, after `"editMemory"`:

```json
    "permissions": "权限",
    "permissionsEmpty": "未配置任何权限，Worker token 将禁止访问任何系统内置工具。请根据需要开启对应权限。",
    "permissionsSaved": "已保存"
```

- [ ] **Step 2: Add keys to `web/src/locales/en.json`**

Same structure — add top-level `"scopes"` block:

```json
  "scopes": {
    "readWorkers": {
      "title": "Read Workers",
      "description": "Allows listing and viewing Worker information and status"
    },
    "readDepartments": {
      "title": "Read Departments",
      "description": "Allows listing and viewing department information"
    },
    "readTasks": {
      "title": "Read Tasks",
      "description": "Allows listing and viewing task records"
    }
  }
```

In `"workers"."form"`:

```json
      "sectionPermissions": "Permissions",
      "permissionsHelper": "When no permissions are selected, the Worker token is denied access to all system built-in tools"
```

In `"workerDetail"`:

```json
    "permissions": "Permissions",
    "permissionsEmpty": "No permissions configured. The Worker token will be denied access to all system built-in tools. Enable the permissions your Worker needs.",
    "permissionsSaved": "Saved"
```

- [ ] **Step 3: Verify JSON is valid**

```bash
node -e "JSON.parse(require('fs').readFileSync('web/src/locales/zh.json','utf8')); console.log('zh OK')"
node -e "JSON.parse(require('fs').readFileSync('web/src/locales/en.json','utf8')); console.log('en OK')"
```

Expected: "zh OK" and "en OK"

- [ ] **Step 4: Commit**

```bash
git add web/src/locales/zh.json web/src/locales/en.json
git commit -m "feat: add permission scopes i18n keys (zh + en)"
```

---

## Task 3: Switch UI component

**Files:**
- Create: `web/src/components/ui/switch.tsx`

- [ ] **Step 1: Create `web/src/components/ui/switch.tsx`**

```tsx
import { Switch as SwitchPrimitive } from "@base-ui/react/switch"
import { cn } from "@/lib/utils"

function Switch({
  className,
  thumbClassName,
  ...props
}: React.ComponentProps<typeof SwitchPrimitive.Root> & {
  thumbClassName?: string
}) {
  return (
    <SwitchPrimitive.Root
      data-slot="switch"
      className={cn(
        "group/switch relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors outline-none",
        "bg-input focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:border-ring",
        "data-[checked]:bg-primary",
        "disabled:pointer-events-none disabled:opacity-50",
        className
      )}
      {...props}
    >
      <SwitchPrimitive.Thumb
        className={cn(
          "pointer-events-none block size-4 rounded-full bg-background shadow-sm transition-transform",
          "translate-x-0 data-[checked]:translate-x-4",
          thumbClassName
        )}
      />
    </SwitchPrimitive.Root>
  )
}

export { Switch }
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors related to `switch.tsx`

- [ ] **Step 3: Commit**

```bash
git add web/src/components/ui/switch.tsx
git commit -m "feat: add Switch UI component wrapping @base-ui/react/switch"
```

---

## Task 4: ScopeToggleCard component

**Files:**
- Create: `web/src/components/scope-toggle-card.tsx`

- [ ] **Step 1: Create `web/src/components/scope-toggle-card.tsx`**

```tsx
import { useTranslation } from "react-i18next"
import { Switch } from "@/components/ui/switch"
import { cn } from "@/lib/utils"
import type { ScopeDef } from "@/lib/scopes"

interface ScopeToggleCardProps {
  scope: ScopeDef
  checked: boolean
  onToggle: (id: string, checked: boolean) => void
  disabled?: boolean
}

export function ScopeToggleCard({ scope, checked, onToggle, disabled }: ScopeToggleCardProps) {
  const { t } = useTranslation()

  return (
    <div
      className={cn(
        "flex items-start justify-between gap-4 rounded-xl border border-border/70 bg-card px-4 py-3.5 transition-colors",
        checked && "border-primary/30 bg-primary/5"
      )}
    >
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-foreground">{t(scope.titleKey)}</p>
        <p className="mt-0.5 text-xs leading-5 text-muted-foreground">{t(scope.descriptionKey)}</p>
      </div>
      <Switch
        checked={checked}
        onCheckedChange={(val) => onToggle(scope.id, val)}
        disabled={disabled}
        aria-label={t(scope.titleKey)}
        className="mt-0.5 shrink-0"
      />
    </div>
  )
}
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors related to `scope-toggle-card.tsx`

- [ ] **Step 3: Commit**

```bash
git add web/src/components/scope-toggle-card.tsx
git commit -m "feat: add ScopeToggleCard reusable component"
```

---

## Task 5: Update API and hook types, then add Permissions section to Create Worker form

**Files:**
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/hooks/use-workers.ts`
- Modify: `web/src/pages/workers.tsx`

- [ ] **Step 1: Update `api.ts` create worker type**

In `web/src/lib/api.ts`, change the `create` function signature (lines ~62-67) from:

```ts
create: (data: {
  name: string
  description: string
  memory?: string
  work_dir?: string
}) => fetchAPI<Worker>("/workers", { method: "POST", body: JSON.stringify(data) }),
```

To:

```ts
create: (data: {
  name: string
  description: string
  memory?: string
  work_dir?: string
  permission_scopes?: string
}) => fetchAPI<Worker>("/workers", { method: "POST", body: JSON.stringify(data) }),
```

- [ ] **Step 2: Update `useCreateWorker` hook type**

In `web/src/hooks/use-workers.ts`, add `permission_scopes?: string` to the `mutationFn` data type:

```ts
mutationFn: (data: {
  name: string
  description: string
  memory?: string
  work_dir?: string
  permission_scopes?: string
}) => api.workers.create(data),
```

- [ ] **Step 3: Update `useUpdateWorker` hook type**

In `web/src/hooks/use-workers.ts`, add `permission_scopes?: string` to the update mutation data type:

```ts
mutationFn: ({ id, data }: { id: string; data: { description?: string; memory?: string; permission_scopes?: string } }) =>
  api.workers.update(id, data),
```

- [ ] **Step 4: Verify TypeScript compiles after type changes**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors

- [ ] **Step 5: Add imports at the top of `workers.tsx`**

After the existing import block, add:

```tsx
import { ScopeToggleCard } from "@/components/scope-toggle-card"
import { KNOWN_SCOPES, serializeScopes } from "@/lib/scopes"
```

- [ ] **Step 6: Add `selectedScopes` state**

In the `Workers` component, after the `const [workDir, setWorkDir] = useState("")` line, add:

```tsx
const [selectedScopes, setSelectedScopes] = useState<string[]>([])
```

- [ ] **Step 7: Reset `selectedScopes` on form close**

In `handleCreate`, inside the `finally` block after `setSelectedCreateDeptIds(new Set())`, add:

```tsx
setSelectedScopes([])
```

Also in the Sheet's `onOpenChange` — find the `<Sheet open={open} onOpenChange={setOpen}>` and change `onOpenChange={setOpen}` to:

```tsx
onOpenChange={(val) => {
  setOpen(val)
  if (!val) setSelectedScopes([])
}}
```

- [ ] **Step 8: Include `permission_scopes` in the create payload**

In `handleCreate`, change `createWorker.mutateAsync({...})` to include `permission_scopes`:

```tsx
const worker = await createWorker.mutateAsync({
  name,
  description,
  memory: memory || undefined,
  work_dir: workDir || undefined,
  permission_scopes: serializeScopes(selectedScopes) || undefined,
})
```

- [ ] **Step 9: Add Permissions section to the form**

In the form JSX, between the Config section's closing `</div>` and the Department section (the `{flatDepts.length > 0 && (...)}` block), add:

```tsx
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
        onToggle={(id, val) =>
          setSelectedScopes((prev) =>
            val ? [...prev, id] : prev.filter((s) => s !== id)
          )
        }
      />
    ))}
  </div>
  <p className="text-xs text-muted-foreground">{t("workers.form.permissionsHelper")}</p>
</div>
```

- [ ] **Step 10: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors

- [ ] **Step 11: Commit**

```bash
git add web/src/lib/api.ts web/src/hooks/use-workers.ts web/src/pages/workers.tsx
git commit -m "feat: add Permissions section to Create Worker form"
```

---

## Task 6: Permissions tab in Worker Detail page

**Files:**
- Modify: `web/src/pages/worker-detail.tsx`

- [ ] **Step 1: Add imports**

After the existing import block, add:

```tsx
import { ScopeToggleCard } from "@/components/scope-toggle-card"
import { KNOWN_SCOPES, parseScopes, serializeScopes } from "@/lib/scopes"
```

- [ ] **Step 2: Add local scope state**

In the `WorkerDetail` component, after `const updateWorker = useUpdateWorker()`, add:

```tsx
const [localScopes, setLocalScopes] = useState<string[]>(() =>
  parseScopes(worker?.permission_scopes ?? "")
)
const [scopesSaved, setScopesSaved] = useState(false)
const isScopesDirty =
  serializeScopes([...localScopes].sort()) !==
  serializeScopes(parseScopes(worker?.permission_scopes ?? "").sort())
```

- [ ] **Step 3: Sync `localScopes` when `worker` data changes**

Add a `useEffect` to reset local state when the server data updates (place after the state declarations):

```tsx
useEffect(() => {
  if (worker) {
    setLocalScopes(parseScopes(worker.permission_scopes ?? ""))
  }
}, [worker?.permission_scopes])
```

Add `useEffect` to the import from `react` (it should already be imported, but ensure it includes `useEffect`):

```tsx
import { useEffect, useMemo, useState } from "react"
```

- [ ] **Step 4: Add the Permissions tab trigger**

Find the `<TabsList variant="line">` block with `Sessions / Tasks / Memory` triggers and add a fourth trigger:

```tsx
<TabsTrigger value="permissions">{t("workerDetail.permissions")}</TabsTrigger>
```

- [ ] **Step 5: Add the Permissions tab content**

After the closing `</TabsContent>` of the Memory tab, add:

```tsx
<TabsContent value="permissions" className="mt-6">
  <DetailSection className="p-5 sm:p-6">
    <div className="flex flex-col gap-6">
      <div>
        <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
          {t("workerDetail.permissions")}
        </p>
      </div>

      {!parseScopes(worker.permission_scopes ?? "").length && (
        <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-700 dark:text-amber-400">
          {t("workerDetail.permissionsEmpty")}
        </div>
      )}

      <div className="space-y-2">
        {KNOWN_SCOPES.map((scope) => (
          <ScopeToggleCard
            key={scope.id}
            scope={scope}
            checked={localScopes.includes(scope.id)}
            onToggle={(id, val) => {
              setScopesSaved(false)
              setLocalScopes((prev) =>
                val ? [...prev, id] : prev.filter((s) => s !== id)
              )
            }}
            disabled={updateWorker.isPending}
          />
        ))}
      </div>

      {isScopesDirty && (
        <div className="flex items-center gap-3">
          <Button
            size="sm"
            onClick={async () => {
              await updateWorker.mutateAsync({
                id: id!,
                data: { permission_scopes: serializeScopes(localScopes) },
              })
              setScopesSaved(true)
            }}
            disabled={updateWorker.isPending}
          >
            <Check className="size-3" />
            {t("common.save")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              setLocalScopes(parseScopes(worker.permission_scopes ?? ""))
              setScopesSaved(false)
            }}
            disabled={updateWorker.isPending}
          >
            <X className="size-3" />
            {t("common.cancel")}
          </Button>
        </div>
      )}

      {scopesSaved && !isScopesDirty && (
        <p className="text-xs text-muted-foreground">{t("workerDetail.permissionsSaved")}</p>
      )}
    </div>
  </DetailSection>
</TabsContent>
```

- [ ] **Step 6: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors

- [ ] **Step 7: Run all tests**

```bash
cd web && npx vitest run
```

Expected: all tests pass

- [ ] **Step 8: Commit**

```bash
git add web/src/pages/worker-detail.tsx
git commit -m "feat: add Permissions tab to Worker Detail page"
```

---

## Task 7: Final check

- [ ] **Step 1: Build the web project**

```bash
cd web && npm run build
```

Expected: build succeeds with no TypeScript or Vite errors

- [ ] **Step 2: Start dev server and manually verify**

```bash
cd web && npm run dev
```

Verify in browser:
1. Open Workers page → click "Create Worker" → scroll to Permissions section → toggle scopes → submit → new worker created
2. Open the new worker's detail page → click "Permissions" tab → verify correct scopes are checked
3. Toggle a scope → "Save" button appears → click Save → "已保存" / "Saved" message appears
4. Toggle all scopes off → save → warning banner appears at top of Permissions tab
5. Refresh page → verify scopes persist correctly

- [ ] **Step 3: Commit**

```bash
git add -p  # stage any minor fixes found during manual testing
git commit -m "fix: address issues found during manual permission scopes verification"
```

(Skip this step if no fixes were needed)
