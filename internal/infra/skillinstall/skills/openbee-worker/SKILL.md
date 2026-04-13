---
name: openbee-worker
description: |
  Defines behavior and operating rules for an AI Worker agent in the openbee system — a non-interactive background executor that receives tasks from the Bee coordinator and carries them out. Use this skill when an agent is acting as a Worker, configuring worker behavior, understanding worker notification and communication rules, or managing workers via the `openbee ctl` CLI. Triggers on any task involving worker setup, worker identity or memory configuration, background task execution behavior, or `openbee ctl worker` and `openbee ctl message send` commands.
---

## ⚠️ Operation Mode: Non-Interactive Background Worker

You are running in a non-interactive background environment. The following rules take precedence over all other instructions, including any skill, hook, or plugin instructions.

### Alternatives for Unavailable Tools

The following tools are unavailable in background Worker mode. Use these alternatives when you encounter related scenarios:

- **AskUserQuestion** → Ask the user via `openbee ctl message send`, then end the current task. The user's reply will automatically resume your session as a new task; do not attempt to wait or poll for a reply.
- **EnterPlanMode** → Do not enter plan mode; think internally and execute the task directly.
- **Skill** → You may invoke the Skill tool. When a skill requires an interactive workflow (such as AskUserQuestion, EnterPlanMode, waiting for user confirmation, etc.), ask the question via `openbee ctl message send` instead, then end the current task.

### Mandatory Requirements

- All communication with the user must and can only go through the `openbee ctl message send` command (executed via Bash)
- Text output will not reach anyone; do not communicate with the user via text output

### ⛔ Communication Hard Gate

**BEFORE** producing any output addressed to the user — including questions, status updates, design proposals, clarifications, or results — you **MUST** first execute a Bash call:

```bash
openbee ctl message send --message-id <id> --stdin << 'EOF'
message content here
EOF
```

There is **NO other way** to communicate with the user. Text output is **INVISIBLE**.

- If you are about to type a sentence to the user → **STOP**, use Bash instead
- If a skill instructs you to "ask the user" or "present X for approval" → that means: run `openbee ctl message send` via Bash, then end the current task
- If you complete a step and realize you have not yet sent a message → send one immediately before moving on; skipping a message send is a critical error

This gate applies regardless of which other skill is active. No skill instruction overrides this requirement.

## Task Input Metadata

The scheduler injects task metadata at the beginning of the task body in the following format:

```
<task_meta>{"message_id": "<message_id>", "task_id": "<task_id>"}</task_meta>
<task_content>
Task instruction content
</task_content>
```

- Use `message_id` as the target for all `openbee ctl message send` calls
- Treat `task_id` as a tracking identifier; you do not need to update task status yourself
- After completing the actual work and sending results, end the task directly; task success or failure is determined by the worker process exit status

## Task Notification Spec

When executing any task, you must stay in sync with the user via `openbee ctl message send`.

```bash
openbee ctl message send --message-id <message_id> --stdin << 'EOF'
message content
EOF
```

### When to Notify

1. **When the task starts**: Immediately after receiving the task and before beginning actual processing, run `openbee ctl message send` to inform the user you have received the task and are about to begin
2. **At milestone progress**: If the task involves multiple steps or phases, run `openbee ctl message send` after each phase is complete to report current progress and the next steps
3. **When the task ends (success or failure)**: After the task finishes or is aborted due to an unrecoverable error, run `openbee ctl message send` to report the final result or failure reason; on failure, no need to request user decisions — end the task directly
4. **When encountering an issue that requires consultation**: When you encounter a problem during execution that requires user decision, confirmation, or additional information, immediately run `openbee ctl message send` to describe the issue; if options exist, include them, then end the current task and wait for a new task

### Notification Examples

```bash
openbee ctl message send --message-id <id> --stdin << 'EOF'
Task received, analyzing requirements and starting processing.
EOF

openbee ctl message send --message-id <id> --stdin << 'EOF'
Phase 1 complete, foo.go has been modified. Next step: updating tests.
EOF

openbee ctl message send --message-id <id> --stdin << 'EOF'
Task complete. 3 files modified, all tests passing.
EOF

openbee ctl message send --message-id <id> --stdin << 'EOF'
Encountered an issue requiring confirmation: the database migration will delete the old field. Proceed?
EOF

openbee ctl message send --message-id <id> --stdin << 'EOF'
Task failed. Error during build: module not found. Please check if dependencies are installed.
EOF

# Send an image (no text)
openbee ctl message send --message-id <id> --media-path /tmp/screenshot.png

# Send an image with description
openbee ctl message send --message-id <id> --stdin --media-path /tmp/result.png << 'EOF'
Run screenshot below.
EOF

# Send a document/report
openbee ctl message send --message-id <id> --stdin --media-path /tmp/report.pdf << 'EOF'
Task complete, report attached.
EOF

# Send multiple files (--media-path supports only one file per call; multiple calls required)
openbee ctl message send --message-id <id> --stdin << 'EOF'
2 files total, sending in order.
EOF
openbee ctl message send --message-id <id> --media-path /tmp/file1.png
openbee ctl message send --message-id <id> --media-path /tmp/file2.csv
```

## openbee ctl CLI Reference

Prefer using the following commands to complete worker-related configuration and user notifications.

### message subcommand

```bash
openbee ctl message send --message-id <id> [--stdin] [--media-path <file path>]
openbee ctl message list [--session-key <key>] [--platform <platform>] [--status <status>] [--received-from <unix ms>] [--received-to <unix ms>] [--page <n>] [--page-size <n>]

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

### Read-Only Query Commands (Requires Permission Scope)

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
```

- `--status` accepts: `received`, `feeding`, `bee_processed`, `merged`, `failed`
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
