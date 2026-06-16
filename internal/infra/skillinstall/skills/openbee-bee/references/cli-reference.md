# openbee ctl CLI Complete Reference

`openbee ctl` is the command-line tool for operating the openbee system, outputting in JSON format. All subcommands use `-c config.yaml` to specify the config file (default: `config.yaml`).

## worker subcommand

```bash
openbee ctl worker list [--department <id|name>] [--no-recursive] [--name <name>] [--id <id>] [--page <n>] [--page-size <n>]
openbee ctl worker get <id>
openbee ctl worker status <id>
openbee ctl worker create --name <name> [--description <description>] [--constraints <constraints content>] [--work-dir <directory>] [--engine <engine>] [--department <id|name>] [--scopes <scopes>] [--engine-args <engine=args>]
openbee ctl worker update <id> [--name <name>] [--description <description>] [--constraints <constraints>] [--work-dir <directory>] [--engine <engine>] [--department <id|name>] [--scopes <scopes>] [--engine-args <engine=args>]
openbee ctl worker delete <id> [--delete-work-dir]
```

- `--department` accepts an ID or name; comma-separated for multiple departments
- `--no-recursive` (only for `worker list`): return only workers directly in the department, excluding child departments; default is recursive
- `--name` (only for `worker list`): filter by name (case-insensitive partial match)
- `--id` (only for `worker list`): filter by exact worker ID
- `--page` / `--page-size` (only for `worker list`): pagination; default page 1, default 50 per page, max 200
- `--scopes` (create/update): comma-separated permission scope list granted to this worker; pass empty string to clear all scopes
- `--engine-args` (create/update): extra CLI flags for a specific engine, in `engine=<flags>` format (repeatable); e.g. `--engine-args "claude=--model claude-sonnet-4-6 --effort high"`; for update, pass `engine=` (empty value) to clear args for that engine

### Worker Permission Scopes

Permission scopes control which read-only query tools a worker token is allowed to call. Bee tokens are never scope-restricted — only worker tokens are subject to scope enforcement.

Available scopes:

| Scope | Grants access to |
|---|---|
| `read:workers` | `list_workers`, `get_worker`, `get_worker_status` |
| `read:departments` | `list_departments`, `get_department` |
| `read:tasks` | `list_tasks` |
| `read:messages` | `list_messages`, `list_outbound_messages` |

If a worker token calls a tool without the required scope, the call returns: `permission denied: scope <scope> required`.

```bash
# Grant a worker access to workers and tasks
openbee ctl worker update <id> --scopes read:workers,read:tasks

# Grant all read scopes
openbee ctl worker update <id> --scopes read:workers,read:departments,read:tasks,read:messages

# Clear all scopes
openbee ctl worker update <id> --scopes ""
```

```bash
# Update a worker's working directory
openbee ctl worker update <id> --work-dir /path/to/new/dir
```

## department subcommand

```bash
openbee ctl department list
openbee ctl department get <id|name>
openbee ctl department create --name <name> [--parent <id|name>] [--sort-order <n>]
openbee ctl department update <id|name> [--name <name>] [--parent <id|name>] [--sort-order <n>]
openbee ctl department delete <id|name>
```

## task subcommand

```bash
openbee ctl task list [--task-id <id>] [--session-key <key>] [--message-id <id>] [--worker-id <id>] [--status <status>] [--type <type>] [--page <n>] [--page-size <n>] [--execution-limit <n>]
openbee ctl task create --message-id <id> --worker-id <id> --instruction <instruction> --type <immediate|countdown|scheduled> [--scheduled-at <unix milliseconds>] [--cron <cron expression>]
openbee ctl task cancel <id>
```

Each task returned by `task list` includes an `executions` array with the newest execution records for that task. The default is the latest 10 executions per task. Use `--execution-limit <n>` to request a different bounded count, or `--task-id <id> --execution-limit 0` to inspect the full execution history for one task. Task results are paginated (default 50 per page, max 100); the response contains `items`, `total`, `page`, `page_size`.

## constraint subcommand

```bash
openbee ctl constraint get --scope <global|session_key> [--key <key>]
openbee ctl constraint save --scope <global|session_key> --key <key> --value <value>
openbee ctl constraint delete --scope <global|session_key> --key <key>
```

## session subcommand

```bash
openbee ctl session list --session-key <key>
openbee ctl session clear --session-key <key> [--force]
openbee ctl session clear-worker --session-key <key> --worker-id <id> [--force]
```

## system subcommand

```bash
openbee ctl system overview
```

## message subcommand

```bash
openbee ctl message send --message-id <id> [--stdin] [--media-path <file path>]
openbee ctl message list [--session-key <key>] [--platform <platform>] [--status <status>] [--received-from <unix ms>] [--received-to <unix ms>] [--page <n>] [--page-size <n>]
openbee ctl message list-outbound [--session-key <key>] [--platform <platform>] [--status <status>] [--source-type <type>] [--source-id <id>] [--sent-from <unix ms>] [--sent-to <unix ms>] [--page <n>] [--page-size <n>]

# Note: --media-path supports only one file per call; sending multiple files requires multiple calls

# Scenario 1: Text-only notification
openbee ctl message send --message-id <id> --stdin << 'EOF'
Task has been dispatched to Maomao, please wait.
EOF

# Scenario 2: Send a screenshot (with description)
openbee ctl message send --message-id <id> --stdin --media-path /tmp/overview.png << 'EOF'
System status screenshot attached.
EOF

# Scenario 3: Send a file (e.g., logs, CSV report)
openbee ctl message send --message-id <id> --stdin --media-path /tmp/tasks.csv << 'EOF'
Here is the exported task list.
EOF

# Scenario 4: Send multiple files (multiple calls required)
openbee ctl message send --message-id <id> --stdin << 'EOF'
2 attachments in total, sending in order.
EOF
openbee ctl message send --message-id <id> --media-path /tmp/file1.png
openbee ctl message send --message-id <id> --media-path /tmp/file2.pdf

# Scenario 5: Query message history (single filter)
openbee ctl message list --session-key feishu:oc_xxx:ou_xxx --status received
openbee ctl message list --platform feishu --received-from 1700000000000

# Scenario 6: Query message history (multiple filters combined)
openbee ctl message list --platform feishu --status received --session-key feishu:oc_xxx:ou_xxx
openbee ctl message list --platform feishu --received-from 1700000000000 --received-to 1700086400000 --status bee_processed

# Scenario 7: Pagination (default 50 per page, max 100)
openbee ctl message list --platform feishu --page 2 --page-size 20
openbee ctl message list --session-key feishu:oc_xxx:ou_xxx --page 1 --page-size 100
```