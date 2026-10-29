# Session Context Management

## View Current Context State

When the user asks "which workers have context", "what conversation history exists", etc., list all coordinators and workers with conversation records in the current session with `execute`:

```js
return await openbee.list_session_contexts({ session_key: "<session_key>" });
```

## Clear Entire Session

When the user sends a message indicating they want to clear/reset the entire conversation (e.g., "clear", "reset context", etc.):

1. Call `openbee.clear_session({ session_key: "<key>" })` in `execute` (without `force` by default):
   - If it returns `requires_confirmation: true` with `reason: "active_tasks"`: per notification spec (item 5 — active tasks found before clearing), via `send_message`, show the user the running task list and ask: "There are N tasks currently being processed (Tasks: [list of instructions]). Clearing the context will terminate these tasks. Do you confirm continuing?" After user confirms, call again with `force: true`.
   - If it returns `requires_confirmation: true` with `reason: "multiple_workers"`: per notification spec (item 5 — clearing requires second confirmation), via `send_message`, show the user the list of affected workers and inform them "This operation will reset the conversation context of the following workers: [list]. Please confirm whether to continue. After confirmation, the history of all the above workers will be cleared." After user confirms, call again with `force: true`.
   - If it returns `cleared: false` with `reason: "no_context"`: tell the user there is no conversation context to clear.
   - If it returns `cleared: true`: per notification spec (item 5 — clearing succeeds), inform the user: "Session cleared. All workers' conversation contexts have been reset; you can start a new conversation."

## Reset a Single Worker's Context

When the user wants to reset only one worker's conversation memory (e.g., "reset XX's context", "make XX forget the previous conversation"):

1. Call `openbee.clear_worker_session({ session_key: "<key>", worker_id: "<id>" })` in `execute` (without `force` by default):
   - If it returns `requires_confirmation: true` with `reason: "active_tasks"`: per notification spec (item 5 — active tasks found before clearing), via `send_message`, show the user the running task list and ask: "Worker XX has N tasks currently active (Tasks: [list of instructions]). Clearing the context will terminate these tasks. Do you confirm continuing?" After user confirms, call again with `force: true`.
   - If it returns `cleared: true`: per notification spec (item 5 — single worker context reset completes), inform the user that this worker's context has been reset and the next interaction will start from a fresh state.

Only the worker's session context for the currently active engine is removed — contexts on other engines are preserved.
