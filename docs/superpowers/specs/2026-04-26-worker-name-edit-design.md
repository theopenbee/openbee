# Worker Name Edit Design

**Date:** 2026-04-26  
**Feature:** Allow editing worker name via EditWorkerInfoSheet

## Summary

Add a `name` field to the existing `EditWorkerInfoSheet` component so users can rename a worker from the detail page. The field includes a random name generation button, consistent with the create flow.

## Scope

**One file changed:** `web/src/components/edit-worker-info-sheet.tsx`

No changes required to:
- Backend API (already supports `name` in `PUT /workers/{id}`)
- API client (`web/src/lib/api.ts`)
- React Query hooks (`web/src/hooks/use-workers.ts`)
- TypeScript types (`web/src/lib/types.ts`)

## UI Layout

The name field is inserted at the top of the form, above the existing description field:

```
Label: Name
[ input: worker name      ] [ 🎲 Random ]
Label: Description
[ textarea: description   ]
...rest of existing fields
```

The random button reuses the existing `useRandomWorkerName` hook already used in `CreateWorkerSheet`.

## State Changes

Add `name: string` to component state alongside existing fields:

```ts
const [name, setName] = useState(worker?.name ?? '')
```

Sync in the existing `useEffect` that initialises form state when the sheet opens:

```ts
setName(worker.name ?? '')
```

## Submission Logic

Only include `name` in the update payload when it has changed, consistent with how other fields are handled:

```ts
if (name !== worker.name) payload.name = name
```

## Random Name Button Behaviour

- Calls `useRandomWorkerName()` on click and fills the name input with the result
- While loading: button shows spinner and is disabled
- If the random name pool is exhausted (empty response): button stays disabled
- Identical behaviour to `CreateWorkerSheet`

## Validation & Error Handling

| Scenario | Handling |
|----------|----------|
| Name is empty | Submit button disabled (frontend) |
| Name already taken | Backend returns error → shown in existing `submitError` area |
| Name conflicts with a bot name | Backend returns error → shown in existing `submitError` area |
| Network error | Generic error shown in `submitError` area |

No new inline error UI is introduced. All server-side validation errors surface through the existing `submitError` state.

## Success Flow

1. `PUT /workers/{id}` returns the updated worker
2. Sheet closes
3. React Query invalidates the worker cache
4. Worker detail page header refreshes automatically

## Manual Test Plan

1. Open Edit Sheet → Name field pre-filled with current name
2. Change name → Save → Detail page header shows new name
3. Change name to an existing worker's name → Save → error message appears
4. Click random button → Name input auto-filled with a new unique name
5. Leave name unchanged → Save → succeeds without error (name field not sent)
