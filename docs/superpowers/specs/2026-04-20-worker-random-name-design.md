# Worker Random Name Generation

**Date:** 2026-04-20
**Status:** Approved

## Overview

Add a "random name" button to the worker creation form. Clicking it calls a backend endpoint that picks an unused name from a built-in pool of 200 characters — Three Kingdoms figures for Chinese environments, historical scientists/explorers for English environments. Names already used by existing workers are skipped automatically.

## Backend

### New Endpoint

```
GET /workers/random-name
```

No query parameters. The handler reads `cfg.Language` to select the appropriate name pool.

**Responses:**

```json
// Unused name found
{ "name": "诸葛亮" }

// All names exhausted
{ "exhausted": true }
```

HTTP status `200 OK` in both cases. Frontend distinguishes via the `exhausted` field.

### Name Data

New file: `internal/domain/worker/names.go`

```go
var zhNames = []string{ /* 200 Three Kingdoms characters */ }
var enNames = []string{ /* 200 scientists/explorers */ }
```

### Handler Logic

Location: `internal/api/worker_handler.go` — new method `RandomName`.

1. Select name pool based on `h.cfg.Language` (`"zh"` → `zhNames`, else `enNames`)
2. Query all existing worker names from store (`workerStore.ListNames()` or equivalent)
3. Build a set of used names (case-insensitive)
4. Filter pool to unused names
5. If empty → return `{"exhausted": true}`
6. Pick a random entry → return `{"name": "<chosen>"}`

### Route Registration

`internal/app/app.go` — add `GET /workers/random-name` alongside existing worker routes.

## Frontend

### UI Change

In `create-worker-sheet.tsx`, the Name field becomes a flex row:

```
[ Name *                         ]
[ ________________________________ ] [🔀]
  Used for identification
```

The shuffle button (`Shuffle` icon from lucide-react) sits inline to the right of the Input.

**Button states:**

| State | Appearance | Condition |
|-------|-----------|-----------|
| Default | Enabled | Names available |
| Loading | Spinner, disabled | Fetching in progress |
| Exhausted | Grayed out + tooltip | API returns `exhausted: true` |

Tooltip text (i18n key `workers.form.randomNameExhausted`): "All names are in use".

### New API Method

`web/src/lib/api.ts`:

```ts
randomName: () => fetch('/workers/random-name').then(r => r.json())
```

### New Hook

`web/src/hooks/use-workers.ts`:

```ts
export function useRandomWorkerName() {
  return useMutation({ mutationFn: () => api.workers.randomName() })
}
```

### Component Integration

`create-worker-sheet.tsx`:

- Import `useRandomWorkerName` hook
- Add `exhausted` state (boolean, resets when sheet opens)
- On button click: call mutation → on success fill `name` state or set `exhausted`

## i18n Keys

Both `en.json` and `zh.json`:

| Key | English | Chinese |
|-----|---------|---------|
| `workers.form.randomName` | "Random name" | "随机姓名" |
| `workers.form.randomNameExhausted` | "All names are in use" | "所有名字已被使用" |

## Scope Boundaries (YAGNI)

- No uniqueness constraint added to the database — existing behavior preserved
- No user-configurable name preferences
- No custom name pool support
- Copy-worker flow unchanged (inherits original name + suffix)

## Files Changed

| File | Change |
|------|--------|
| `internal/domain/worker/names.go` | **New** — name pool constants |
| `internal/api/worker_handler.go` | Add `RandomName` handler method |
| `internal/app/app.go` | Register new route |
| `web/src/lib/api.ts` | Add `workers.randomName()` |
| `web/src/hooks/use-workers.ts` | Add `useRandomWorkerName()` hook |
| `web/src/components/create-worker-sheet.tsx` | Add shuffle button to name field |
| `web/src/locales/en.json` | Add 2 i18n keys |
| `web/src/locales/zh.json` | Add 2 i18n keys |
