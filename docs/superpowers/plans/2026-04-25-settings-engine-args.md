# Settings Page: Engine Args System Config Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two new cards to `/settings` for `engine_args_global` and `engine_args_bee` system config keys, reusing the existing `EngineArgsSection` component.

**Architecture:** Add a private `EngineArgsConfigSection` component inside `settings.tsx` that encapsulates pending state + mutation for one engine-args config key; render it twice. Parse JSON ↔ `Record<string,string>` in `SystemSettings` before passing down; serialize on save.

**Tech Stack:** React, TypeScript, @tanstack/react-query, react-i18next, sonner (toasts), Vitest

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `web/src/lib/types.ts` | Modify | Add two `SYSTEM_CONFIG_KEY_ENGINE_ARGS_*` constants |
| `web/src/locales/en.json` | Modify | Add `globalArgsSection` and `beeArgsSection` keys under `systemSettings` |
| `web/src/locales/zh.json` | Modify | Same keys, Chinese translations |
| `web/src/pages/settings.tsx` | Modify | Add `EngineArgsConfigSection` component + two card instances |

---

## Task 1: Add type constants

**Files:**
- Modify: `web/src/lib/types.ts` (around line 166)

- [ ] **Step 1: Add constants after `SYSTEM_CONFIG_KEY_DEFAULT_ENGINE`**

Open `web/src/lib/types.ts`. After line 166:
```ts
// Mirrors model.SystemConfigKeyDefaultEngine in Go — keep in sync.
export const SYSTEM_CONFIG_KEY_DEFAULT_ENGINE = "default_engine"
```

Add two lines immediately after:
```ts
// Mirrors model.SystemConfigKeyEngineArgsGlobal in Go — keep in sync.
export const SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL = "engine_args_global"
// Mirrors model.SystemConfigKeyEngineArgsBee in Go — keep in sync.
export const SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE = "engine_args_bee"
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/types.ts
git commit -m "feat(web): add SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL/BEE constants"
```

---

## Task 2: Add i18n keys

**Files:**
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`

- [ ] **Step 1: Add English keys**

In `web/src/locales/en.json`, find the `systemSettings` block (around line 463):
```json
  "systemSettings": {
    "title": "Settings",
    "engineSection": {
      "title": "Default Engine",
      "hint": "The default AI engine Bee uses for scheduling"
    },
    "updated": "Default engine updated"
  }
```

Replace it with:
```json
  "systemSettings": {
    "title": "Settings",
    "engineSection": {
      "title": "Default Engine",
      "hint": "The default AI engine Bee uses for scheduling"
    },
    "updated": "Default engine updated",
    "globalArgsSection": {
      "title": "Global Worker Args",
      "hint": "CLI flags appended to every worker's engine invocation. Applied before per-worker args.",
      "updated": "Global worker args updated"
    },
    "beeArgsSection": {
      "title": "Bee Engine Args",
      "hint": "CLI flags appended to the Bee orchestrator's own engine invocation. Merged on top of global args.",
      "updated": "Bee engine args updated"
    }
  }
```

- [ ] **Step 2: Add Chinese keys**

In `web/src/locales/zh.json`, find the `systemSettings` block (around line 462):
```json
  "systemSettings": {
    "title": "系统设置",
    "engineSection": {
      "title": "默认引擎",
      "hint": "Bee 调度时使用的默认 AI 引擎"
    },
    "updated": "默认引擎已更新"
  }
```

Replace it with:
```json
  "systemSettings": {
    "title": "系统设置",
    "engineSection": {
      "title": "默认引擎",
      "hint": "Bee 调度时使用的默认 AI 引擎"
    },
    "updated": "默认引擎已更新",
    "globalArgsSection": {
      "title": "全局 Worker 参数",
      "hint": "追加到所有 Worker 引擎调用的 CLI 参数，优先级低于各 Worker 自身参数。",
      "updated": "全局 Worker 参数已更新"
    },
    "beeArgsSection": {
      "title": "Bee 引擎参数",
      "hint": "追加到 Bee 编排进程自身引擎调用的 CLI 参数，覆盖全局参数中对应项。",
      "updated": "Bee 引擎参数已更新"
    }
  }
```

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/locales/en.json web/src/locales/zh.json
git commit -m "feat(web): add i18n keys for engine-args settings sections"
```

---

## Task 3: Update settings page

**Files:**
- Modify: `web/src/pages/settings.tsx`

- [ ] **Step 1: Replace the entire file with the new implementation**

Replace the full contents of `web/src/pages/settings.tsx` with:

```tsx
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { DetailSection } from "@/components/detail-primitives"
import {
  Select,
  SelectContent,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Button } from "@/components/ui/button"
import { EngineSelectItems } from "@/components/engine-select-items"
import { EngineArgsSection } from "@/components/engine-args-section"
import { useEnabledEngines } from "@/hooks/use-config"
import { api } from "@/lib/api"
import {
  SYSTEM_CONFIG_KEY_DEFAULT_ENGINE,
  SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL,
  SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE,
} from "@/lib/types"

interface EngineArgsConfigSectionProps {
  configKey: string
  savedValue: Record<string, string>
  title: string
  hint: string
  successMessage: string
}

function EngineArgsConfigSection({
  configKey,
  savedValue,
  title,
  hint,
  successMessage,
}: EngineArgsConfigSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const enabledEngines = useEnabledEngines()
  const [pendingValue, setPendingValue] = useState<Record<string, string> | null>(null)
  const value = pendingValue ?? savedValue

  const { mutate: save, isPending } = useMutation({
    mutationFn: (v: Record<string, string>) =>
      api.systemConfigs.set(configKey, JSON.stringify(v)),
    onError: () => setPendingValue(null),
    onSuccess: () => {
      setPendingValue(null)
      queryClient.invalidateQueries({ queryKey: ["system-configs"] })
      toast.success(successMessage)
    },
  })

  const isDirty =
    pendingValue !== null &&
    JSON.stringify(pendingValue) !== JSON.stringify(savedValue)

  return (
    <DetailSection className="p-5 sm:p-6 space-y-4">
      <div>
        <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
          {title}
        </p>
        <p className="mt-1 text-sm leading-6 text-muted-foreground">{hint}</p>
      </div>
      <EngineArgsSection
        engines={enabledEngines}
        value={value}
        onChange={setPendingValue}
      />
      <div>
        <Button onClick={() => save(value)} disabled={isPending || !isDirty}>
          {t("common.save")}
        </Button>
      </div>
    </DetailSection>
  )
}

export function SystemSettings() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const enabledEngines = useEnabledEngines()

  const { data: sysConfigs } = useQuery({
    queryKey: ["system-configs"],
    queryFn: () => api.systemConfigs.get(),
  })

  const savedEngine = sysConfigs?.[SYSTEM_CONFIG_KEY_DEFAULT_ENGINE] ?? ""
  const [pendingEngine, setPendingEngine] = useState<string | null>(null)
  const engine = pendingEngine ?? savedEngine

  const { mutate: saveEngine, isPending } = useMutation({
    mutationFn: (value: string) =>
      api.systemConfigs.set(SYSTEM_CONFIG_KEY_DEFAULT_ENGINE, value),
    onError: () => setPendingEngine(null),
    onSuccess: () => {
      setPendingEngine(null)
      queryClient.invalidateQueries({ queryKey: ["system-configs"] })
      toast.success(t("systemSettings.updated"))
    },
  })

  function parseEngineArgs(key: string): Record<string, string> {
    const raw = sysConfigs?.[key]
    if (!raw) return {}
    try {
      return JSON.parse(raw) as Record<string, string>
    } catch {
      return {}
    }
  }

  return (
    <FadeIn>
      <div className="space-y-6">
        <PageHeader title={t("systemSettings.title")} />

        <DetailSection className="p-5 sm:p-6 space-y-4">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
              {t("systemSettings.engineSection.title")}
            </p>
            <p className="mt-1 text-sm leading-6 text-muted-foreground">
              {t("systemSettings.engineSection.hint")}
            </p>
          </div>
          <div className="flex items-center gap-3">
            <Select
              value={engine}
              onValueChange={setPendingEngine}
              disabled={isPending}
            >
              <SelectTrigger className="w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <EngineSelectItems engines={enabledEngines} />
              </SelectContent>
            </Select>
            <Button
              onClick={() => saveEngine(engine)}
              disabled={
                isPending ||
                pendingEngine === null ||
                pendingEngine === savedEngine
              }
            >
              {t("common.save")}
            </Button>
          </div>
        </DetailSection>

        <EngineArgsConfigSection
          configKey={SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL}
          savedValue={parseEngineArgs(SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL)}
          title={t("systemSettings.globalArgsSection.title")}
          hint={t("systemSettings.globalArgsSection.hint")}
          successMessage={t("systemSettings.globalArgsSection.updated")}
        />

        <EngineArgsConfigSection
          configKey={SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE}
          savedValue={parseEngineArgs(SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE)}
          title={t("systemSettings.beeArgsSection.title")}
          hint={t("systemSettings.beeArgsSection.hint")}
          successMessage={t("systemSettings.beeArgsSection.updated")}
        />
      </div>
    </FadeIn>
  )
}
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Run existing tests**

```bash
cd web && npm test
```

Expected: all tests pass (no settings-specific unit tests exist; this verifies nothing regressed in shared utilities).

- [ ] **Step 4: Build to confirm no bundler errors**

```bash
cd web && npm run build
```

Expected: build succeeds with no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/settings.tsx
git commit -m "feat(web): add engine-args config sections to settings page"
```
