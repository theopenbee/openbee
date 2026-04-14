# Core Concepts & Entity Relationships

Understanding the relationships between the core entities helps you make better routing, querying, and coordination decisions.

## Entity Overview

| Entity | Description |
|--------|-------------|
| **Message** | An inbound message received from an external platform (e.g. Feishu, Telegram). Carries a `session_key` that identifies the conversation. This is the starting point of every user interaction. |
| **Outbound Message** | A reply sent from the system (Bee, Worker, or system) back to the user on the originating platform. Linked to the originating Message via `inbound_msg_id`; `source_type` distinguishes whether it came from `bee`, `worker`, or `system`. |
| **Task** | A unit of work created by the Bee from a Message and assigned to a Worker. Carries `instruction`, `type` (immediate / countdown / scheduled), and `status`. Links back to the originating Message via `message_id` and to the assigned Worker via `worker_id`. |
| **Execution** | The runtime instance that is created when a Task is actually dispatched and run. Records the process lifecycle: PID, log path, start/end time, result, and final status (`pending` / `running` / `completed` / `failed`). One Task produces one Execution. |
| **Worker** | An AI agent that executes Tasks. Has attributes including `name`, `description`, `status` (idle / working / error), `permission_scopes`, `work_dir`, and `memory`. |
| **Department** | A hierarchical grouping of Workers (tree structure). A Worker can belong to multiple Departments. Used for organizational management and worker filtering. |
| **Session** | Per-agent conversation context, keyed by `(session_key, agent_id)`. Both the Bee and each Worker maintain their own independent Session so multi-turn conversations remain coherent. Clearing a session resets an agent's memory of prior exchanges. |

## Data Flow

```
External Platform
       │  (user sends a message)
       ▼
  Message  ── session_key ──► Session (Bee context, keyed by session_key)
       │
       │  Bee reads & creates
       ▼
    Task  ── worker_id ──► Worker ── department_id ──► Department
       │
       │  Scheduler dispatches
       ▼
  Execution  ── worker_id ──► Worker
       │                           └── Session (Worker context, keyed by session_key + worker_id)
       │  Worker completes
       ▼
Outbound Message  ── inbound_msg_id ──► Message
       │  (source_type = "worker", source_id = worker_id)
       ▼
External Platform
       │  (user receives reply)
```

## Key Relationships at a Glance

- **Message → Task**: one Message can produce one or more Tasks (e.g. the Bee creates tasks for multiple workers)
- **Task → Execution**: one Task produces exactly one Execution when dispatched
- **Task → Worker**: each Task is assigned to exactly one Worker
- **Worker ↔ Department**: many-to-many; a Worker can belong to multiple Departments
- **Outbound Message → Message**: each outbound reply references the inbound Message that triggered it
- **Session**: both Bee and Workers each have their own Session per `session_key`; clearing the session resets conversation history for all agents in that session

## Practical Implications for the Bee

- Use `session_key` from the incoming `<message_meta>` to scope memory, session, and task queries to the current conversation
- When querying task history for a conversation, filter by `--session-key` on `openbee ctl task list`
- When querying outbound message history, use `--source-type worker --source-id <id>` to isolate a specific worker's replies
- `openbee ctl execution list` lets you inspect runtime details (logs, timing, status) for any Worker's executions
- `openbee ctl system overview` aggregates worker load and task stats across all workers and departments
