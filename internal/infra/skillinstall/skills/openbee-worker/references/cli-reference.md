# openbee ctl CLI Reference

Prefer using the following commands to complete worker-related configuration and user notifications.

## message subcommand

```bash
openbee ctl message send --message-id <id> [--stdin] [--media-path <file path>]
openbee ctl message list [--session-key <key>] [--platform <platform>] [--status <status>] [--received-from <unix ms>] [--received-to <unix ms>] [--page <n>] [--page-size <n>]
openbee ctl message list-outbound [--session-key <key>] [--platform <platform>] [--status <status>] [--source-type <type>] [--source-id <id>] [--sent-from <unix ms>] [--sent-to <unix ms>] [--page <n>] [--page-size <n>]

# Note: --media-path supports only one file per call; sending multiple files requires multiple calls
# --stdin and --media-path can be used independently or together (text first, then media)

# Send plain text
openbee ctl message send --message-id <id> --stdin << 'EOF'
Done.
EOF

# Send an image file
openbee ctl message send --message-id <id> --media-path /tmp/chart.png

# Send text and file together
openbee ctl message send --message-id <id> --stdin --media-path /tmp/output.csv << 'EOF'
See attachment for details.
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
openbee ctl worker list --department <id>      # Filter by department ID or name
openbee ctl worker get <id>                    # Get worker details by ID
openbee ctl worker status <id>                 # Get worker current status (idle/working/error)
```

**Requires `read:departments` scope:**

```bash
openbee ctl department list                    # List all departments (tree structure)
openbee ctl department get <id|name>           # Get department details by ID or name
```

**Requires `read:tasks` scope:**

```bash
openbee ctl task list --worker-id <id>         # List tasks assigned to a worker
openbee ctl task list --status pending         # Filter tasks by status
openbee ctl task list --session-key <key>      # Filter tasks by session key
```

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
openbee ctl message list --session-key feishu:oc_xxx:ou_xxx
openbee ctl message list --status received

# Multiple filters combined
openbee ctl message list --platform feishu --status received --session-key feishu:oc_xxx:ou_xxx
openbee ctl message list --platform feishu --received-from 1700000000000 --received-to 1700086400000 --status bee_processed

# Pagination (default 50/page, max 100)
openbee ctl message list --platform feishu --page 2 --page-size 20
openbee ctl message list --session-key feishu:oc_xxx:ou_xxx --page 1 --page-size 100

# list-outbound examples
openbee ctl message list-outbound --session-key feishu:oc_xxx:ou_xxx
openbee ctl message list-outbound --source-type worker --source-id <worker_id>
openbee ctl message list-outbound --status failed
openbee ctl message list-outbound --platform feishu --sent-from 1700000000000 --sent-to 1700086400000
```

**Requires `read:executions` scope:**

```bash
openbee ctl execution list [--worker-id <id>] [--session-id <id>] [--status <status>] [--started-from <unix ms>] [--started-to <unix ms>] [--completed-from <unix ms>] [--completed-to <unix ms>] [--page <n>] [--page-size <n>]
```

- `--status` accepts: `pending`, `running`, `completed`, `failed`
- Pagination: default 50 per page, max 100; returns `items`, `total`, `page`, `page_size`
- All filter flags can be combined freely in a single command

```bash
# Single filter
openbee ctl execution list --worker-id abc123
openbee ctl execution list --status running

# Multiple filters combined
openbee ctl execution list --worker-id abc123 --status completed
openbee ctl execution list --session-id sess_xxx --status failed --started-from 1700000000000
openbee ctl execution list --worker-id abc123 --started-from 1700000000000 --started-to 1700086400000

# Pagination (default 50/page, max 100)
openbee ctl execution list --status completed --page 2 --page-size 20
openbee ctl execution list --worker-id abc123 --page 1 --page-size 100
```
