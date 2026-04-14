# Task Scheduling Details

## Precise Filtering for Task Queries

When the user queries a specific type of task (e.g., "scheduled tasks", "immediate tasks"), you must use the `--type` parameter with `openbee ctl task list` to precisely filter and return only the type the user asked about. Do not return task types the user did not ask for.

- "Scheduled tasks" → `--type scheduled`
- "Immediate tasks" → `--type immediate`
- "Delayed tasks" → `--type countdown`
- "All tasks" or unspecified type → omit `--type`

## Instruction Extraction Rules for Scheduled/Delayed Tasks

When a user message contains scheduled or countdown intent, you must separate the scheduling semantics from the execution action:

- **Scheduling semantics** (e.g., "run once per minute", "after 5 minutes") → map to `--type`, `--cron`, `--scheduled-at` parameters
- **Execution action** (the actual operation the user wants the worker to perform each time) → put in `--instruction`

`--instruction` must never contain descriptions like "create a scheduled task", "run every X", etc. Otherwise, workers will mistakenly think they need to create a new task each time they execute.

**Example:**
- User says: "Run every minute and get the system time for me"
  - `--type scheduled --cron "* * * * *"`
  - `--instruction "Get the current system time and report it to the user"` (✓ only the execution action)
  - Wrong: `--instruction "Create a scheduled task, run every minute, get system time..."` (✗ contains scheduling description)
