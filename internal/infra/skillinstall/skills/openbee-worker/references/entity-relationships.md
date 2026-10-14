# Core Concepts & Entity Relationships

Understanding the relationships between the core entities helps you interpret your task context and use query tools effectively.

## Entity Overview

| Entity | Description |
|--------|-------------|
| **Message** | An inbound message received from an external platform (e.g. Feishu, Telegram). Carries a `session_key` that identifies the conversation. Every Task you receive originates from a Message. |
| **Outbound Message** | A message sent from the system (Bee, Worker, or system) back to the user. When you call `openbee ctl message send`, the system creates an Outbound Message with `source_type = "worker"` and `source_id = your worker ID`. Replies sent with `--message-id` are linked to that Message via `inbound_msg_id`; messages sent with `--session-key` belong to the conversation only. |
| **Task** | The unit of work you receive and execute. Created by the Bee from a Message. Carries `instruction`, `type` (immediate / countdown / scheduled), `status`, the conversation it delivers to (`session_key`), the originating Message (`message_id`), and you (`worker_id`). |
| **Execution** | The runtime instance created when your Task is dispatched. Records your process lifecycle: PID, log path, start/end time, result, and final status (`pending` / `running` / `completed` / `failed`). A Task may produce one or more Executions (e.g. scheduled tasks run repeatedly); each Execution references its Task via `task_id`. |
| **Worker** | An AI agent (you) that executes Tasks. Has attributes including `name`, `description`, `status` (idle / working / error), `permission_scopes`, `work_dir`, and `constraints`. |
| **Department** | A hierarchical grouping of Workers (tree structure). A Worker can belong to multiple Departments. Used for organizational management. |
| **Session** | Per-agent conversation context, keyed by `(session_key, agent_id)`. You and the Bee each maintain independent Sessions. Your Session accumulates conversation history across multiple tasks in the same session, enabling multi-turn context. |

## Data Flow

```
External Platform
       │  (user sends a message)
       ▼
  Message  ── session_key identifies the conversation
       │
       │  Bee creates a Task and assigns it to you
       ▼
    Task  ── message_id ──► Message  (what triggered this work)
       │   ├── session_key ──► conversation to deliver results to
       │   └── worker_id ──► Worker (you)
       │
       │  Scheduler dispatches Task to you
       ▼
  Execution  (your runtime instance: PID, logs, result, status)
       │   └── Session (your conversation context for this session_key)
       │
       │  You complete the task and call message send
       ▼
Outbound Message  ── inbound_msg_id ──► Message (replies only)
       │  (source_type = "worker", source_id = your worker ID)
       ▼
External Platform
       │  (user receives your reply)
```

## Key Relationships at a Glance

- **Task → Message**: every Task you receive traces back to a user Message; the `session_key` in that message scopes the conversation
- **Task → Execution**: each Task has one or more Execution records; the newest ones are returned inline in the `executions` array of each task from `openbee ctl task list` (default: latest 10 per task; use `--task-id <id> --execution-limit 0` for the full history of one task)
- **Worker ↔ Department**: you may belong to multiple Departments; departments are used for grouping and filtering only
- **Outbound Message → Message**: every reply you send via `openbee ctl message send --message-id` is recorded as an Outbound Message linked to the original inbound Message; `--session-key` sends are recorded against the conversation only
- **Session**: your conversation context is isolated per `session_key`; each new session_key starts fresh with no prior conversation history; constraints are re-injected from your worker config at the start of every session

## Practical Implications for a Worker

- Immediate tasks carry `message_id` in task metadata — reply with `openbee ctl message send --message-id <message_id>`
- Scheduled and countdown tasks carry `session_key` instead — deliver with `openbee ctl message send --session-key <session_key>`; they are not bound to any earlier message
- If you have `read:messages` scope, use `openbee ctl message list` to look up the original message content or conversation history by `session_key`
- If you have `read:tasks` scope, use `openbee ctl task list --worker-id <your-id>` to see tasks assigned to you, along with each task's newest `executions` (default 10 per task); add `--task-id <id> --execution-limit 0` to read the full run history of a single task
- Your Session is automatically maintained across tasks in the same session; you do not need to manage it manually
