# openbee ctl CLI Reference

Prefer using the following commands to complete worker-related configuration and user notifications.

## message subcommand

```bash
openbee ctl message send (--message-id <id> | --session-key <key>) [--stdin] [--media-path <file path>]
openbee ctl message list [--session-key <key>] [--platform <platform>] [--status <status>] [--received-from <unix ms>] [--received-to <unix ms>] [--page <n>] [--page-size <n>]
openbee ctl message list-outbound [--session-key <key>] [--platform <platform>] [--status <status>] [--source-type <type>] [--source-id <id>] [--sent-from <unix ms>] [--sent-to <unix ms>] [--page <n>] [--page-size <n>]

# Note: --media-path supports only one file per call; sending multiple files requires multiple calls
# --stdin and --media-path can be used independently or together (text first, then media)
# --message-id replies to that message; --session-key sends a new message to the conversation
# Use the target your task_meta provides: message_id (immediate tasks) or session_key (scheduled / countdown tasks)
# Send tabular files as .xlsx, not .csv: DingTalk cannot preview .csv in-app (see "File Format for Attachments" in SKILL.md)

# Send plain text
openbee ctl message send --message-id <id> --stdin << 'EOF'
Done.
EOF

# Send an image file
openbee ctl message send --message-id <id> --media-path /tmp/chart.png

# Send text and file together
openbee ctl message send --message-id <id> --stdin --media-path /tmp/output.xlsx << 'EOF'
See attachment for details.
EOF

# Scheduled / countdown task: send a new message to the conversation
openbee ctl message send --session-key <session_key> --stdin << 'EOF'
Daily report attached.
EOF
```

## Read-Only Query Commands (Requires Permission Scope)

The following commands are available only if the administrator has granted the corresponding
permission scope to this worker. The worker token in `OPENBEE_API_KEY` is used automatically —
no additional configuration is needed.

If a command returns a "permission denied" error, the worker has not been granted the required scope.
Ask the administrator to run `openbee ctl worker update <id> --scopes <scope>`.

**Requires `read:workers` scope:**

```bash
openbee ctl worker list                        # List all workers
openbee ctl worker get <id>                    # Get worker details by ID
openbee ctl worker status <id>                 # Get worker current status (idle/working/error)
```

**Requires `read:tasks` scope:**

```bash
openbee ctl task list --worker-id <id>                          # List tasks assigned to a worker
openbee ctl task list --status pending                          # Filter tasks by status
openbee ctl task list --session-key <key>                       # Filter tasks by session key
openbee ctl task list --task-id <id>                            # Fetch a single task by ID
openbee ctl task list --worker-id <id> --page 2 --page-size 20  # Paginate results
openbee ctl task list --task-id <id> --execution-limit 0        # Full execution history for one task
```

Each task returned by `task list` includes an `executions` array with the newest execution records for that task. The default is the latest 10 executions per task. Use `--execution-limit <n>` to request a different bounded count, or `--task-id <id> --execution-limit 0` to inspect the full execution history for one task. Task results are paginated (default 50 per page, max 100); the response contains `items`, `total`, `page`, `page_size`.

**Requires `read:messages` scope:**

```bash
openbee ctl message list [--session-key <key>] [--platform <platform>] [--status <status>] [--received-from <unix ms>] [--received-to <unix ms>] [--page <n>] [--page-size <n>]
openbee ctl message list-outbound [--session-key <key>] [--platform <platform>] [--status <status>] [--source-type <type>] [--source-id <id>] [--sent-from <unix ms>] [--sent-to <unix ms>] [--page <n>] [--page-size <n>]
```

- `--status` accepts: `received`, `feeding`, `bee_processed`, `merged`, `failed`
- `--status` (for `list-outbound`): accepts `sent`, `failed`
- `--source-type` (for `list-outbound`): accepts `bee`, `worker`, `system`
- `--source-id` (for `list-outbound`): filter by the ID of the source worker or system
- Pagination: default 50 per page, max 100; returns `items`, `total`, `page`, `page_size`
- All filter flags can be combined freely in a single command

```bash
# Single filter
openbee ctl message list --session-key <platform>:<chat_id>:<user_id>
openbee ctl message list --status received

# Multiple filters combined
openbee ctl message list --platform <platform> --status received --session-key <platform>:<chat_id>:<user_id>
openbee ctl message list --platform <platform> --received-from 1700000000000 --received-to 1700086400000 --status bee_processed

# Pagination (default 50/page, max 100)
openbee ctl message list --platform <platform> --page 2 --page-size 20
openbee ctl message list --session-key <platform>:<chat_id>:<user_id> --page 1 --page-size 100

# list-outbound examples
openbee ctl message list-outbound --session-key <platform>:<chat_id>:<user_id>
openbee ctl message list-outbound --source-type worker --source-id <worker_id>
openbee ctl message list-outbound --status failed
openbee ctl message list-outbound --platform <platform> --sent-from 1700000000000 --sent-to 1700086400000
```

There is no standalone execution query command. Each task returned by `task list` includes an `executions` array with the newest execution records for that task — the latest 10 per task by default. Use `--execution-limit <n>` to request a different bounded count, or `--task-id <id> --execution-limit 0` to inspect the full execution history for one task.
