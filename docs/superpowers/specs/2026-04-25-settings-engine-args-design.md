# Settings Page: Engine Args System Config

**Date:** 2026-04-25
**Branch:** feat/engine-extra-args

## Background

The `feat/engine-extra-args` branch introduced two new system config keys:

| Key | Description | Runtime consumer |
|---|---|---|
| `engine_args_global` | CLI flags appended to every worker's engine invocation, before per-worker args | `domain/worker/manager.go` |
| `engine_args_bee` | CLI flags appended to the Bee orchestrator process's own engine invocation, merged on top of global | `domain/bee/bee_process.go` |

Both keys store a JSON string mapping engine name → CLI flag string, e.g.:
```json
{"claude": "--model claude-sonnet-4-6 --verbose", "codex": "--effort high"}
```

The merge hierarchy is:
- **Workers:** `engine_args_global` → per-worker `engine_args`
- **Bee process:** `engine_args_global` → `engine_args_bee`

The `/settings` page currently only renders a `default_engine` card and has no UI for these two keys.

## Design

### Page Structure

Three independent `DetailSection` cards, stacked vertically:

```
/settings
├── [Card 1] Default Engine      (existing, no changes)
├── [Card 2] Global Worker Args  (new)
└── [Card 3] Bee Engine Args     (new)
```

### New Cards

Both new cards follow the same pattern as the existing Default Engine card: a heading, a hint line, an input area, and a per-section Save button that activates only when the value has changed.

**Card 2 — Global Worker Args**
- Section title: `systemSettings.globalArgsSection.title`
- Hint: `systemSettings.globalArgsSection.hint`
- Input: `EngineArgsSection` (existing component, already used in worker create/edit sheets)
- Save button: disabled when value equals the saved value or save is in flight
- Success toast: `systemSettings.globalArgsSection.updated`

**Card 3 — Bee Engine Args**
- Section title: `systemSettings.beeArgsSection.title`
- Hint: `systemSettings.beeArgsSection.hint`
- Input: `EngineArgsSection`
- Save button: same disabled logic
- Success toast: `systemSettings.beeArgsSection.updated`

### Data Flow

1. `GET /system-configs` already returns all three keys in one response (no API change needed).
2. On load: parse the stored JSON string → `Record<string, string>` (empty object `{}` for missing/empty values).
3. On save: `JSON.stringify(Record<string, string>)` → `PUT /system-configs/:key` with `{ value }`.
4. After a successful save: invalidate `["system-configs"]` query and show a toast.

The `EngineArgsSection` component receives enabled engines from `useEnabledEngines()` (same as worker forms).

### Component Structure

Extract an internal `EngineArgsConfigSection` component in `settings.tsx` to avoid duplicating state management logic:

```tsx
// Internal to settings.tsx (not exported)
function EngineArgsConfigSection({
  configKey,
  savedValue,        // already-parsed Record<string,string> from parent query
  title,
  hint,
  successMessage,
}: EngineArgsConfigSectionProps)
```

This component owns:
- `pendingValue` state (null = no pending change)
- `useMutation` for the save call
- Renders heading, hint, `EngineArgsSection`, Save button

`SystemSettings` passes down `sysConfigs` data, parses each key's JSON string once, and renders two `EngineArgsConfigSection` instances.

### New Constants (`web/src/lib/types.ts`)

```ts
export const SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL = "engine_args_global"
export const SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE = "engine_args_bee"
```

### New i18n Keys

**English (`web/src/locales/en.json`):**

```json
"systemSettings": {
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

**Chinese (`web/src/locales/zh.json`):**

```json
"systemSettings": {
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

## Files to Change

| File | Change |
|---|---|
| `web/src/lib/types.ts` | Add two new `SYSTEM_CONFIG_KEY_*` constants |
| `web/src/locales/en.json` | Add `globalArgsSection` and `beeArgsSection` under `systemSettings` |
| `web/src/locales/zh.json` | Same, Chinese translations |
| `web/src/pages/settings.tsx` | Add `EngineArgsConfigSection` component and two card instances |

No backend changes required. No new files beyond the spec.
