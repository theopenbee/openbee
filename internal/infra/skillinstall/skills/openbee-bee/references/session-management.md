# Session Context Management

## View Current Context State

When the user asks "which workers have context", "what conversation history exists", etc., run the following command to list all coordinators and workers with conversation records in the current session:

```bash
openbee ctl session list --session-key <session_key>
```

## Clear Entire Session

When the user sends a message indicating they want to clear/reset the entire conversation (e.g., "clear", "reset context", etc.):

1. Run `openbee ctl session clear --session-key <key>` (without `--force` by default):
   - If it returns `requires_confirmation=true` with `reason=running_tasks`: per notification spec (item 5 — active tasks found before clearing), via `openbee ctl message send`, show the user the running task list and ask: "There are N tasks currently being processed (Tasks: [list of instructions]). Clearing the context will terminate these tasks. Do you confirm continuing?" After user confirms, re-run with `--force`.
   - If it returns `requires_confirmation=true` with `reason=multiple_workers` (or no `reason` field): per notification spec (item 5 — clearing requires second confirmation), via `openbee ctl message send`, show the user the list of affected workers and inform them "This operation will reset the conversation context of the following workers: [list]. Please confirm whether to continue. After confirmation, the history of all the above workers will be cleared." After user confirms, re-run with `--force`.
   - If it returns `cleared=true`: per notification spec (item 5 — clearing succeeds), inform the user: "Session cleared. All workers' conversation contexts have been reset; you can start a new conversation."

## Reset a Single Worker's Context

When the user wants to reset only one worker's conversation memory (e.g., "reset XX's context", "make XX forget the previous conversation"):

```bash
openbee ctl session clear-worker --session-key <key> --worker-id <id>
```

Per notification spec (item 5 — single worker context reset completes), inform the user that this worker's context has been reset and the next interaction will start from a fresh state.
