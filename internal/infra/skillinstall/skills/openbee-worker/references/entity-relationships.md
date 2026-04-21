# Core Concepts & Entity Relationships

Understanding the relationships between the core entities helps you interpret your task context and use query tools effectively.

## Entity Overview

| Entity | Description |
|--------|-------------|
| **Message** | An inbound message received from an external platform (e.g. Feishu, Telegram). Carries a `session_key` that identifies the conversation. Every Task you receive originates from a Message. |
| **Outbound Message** | A reply sent from the system (Bee, Worker, or system) back to the user. When you call `openbee ctl message send`, the system creates an Outbound Message linked to the originating Message via `inbound_msg_id`, with `source_type = "worker"` and `source_id = your worker ID`. |
| **Task** | The unit of work you receive and execute. Created by the Bee from a Message. Carries `instruction`, `type` (immediate / countdown / scheduled), and `status`. Each Task you execute is linked to the originating Message (`message_id`) and to you (`worker_id`). |
| **Execution** | The runtime instance created when your Task is dispatched. Records your process lifecycle: PID, log path, start/end time, result, and final status (`pending` / `running` / `completed` / `failed`). One Task produces one Execution. |
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
       │   └── worker_id ──► Worker (you)
       │
       │  Scheduler dispatches Task to you
       ▼
  Execution  (your runtime instance: PID, logs, result, status)
       │   └── Session (your conversation context for this session_key)
       │
       │  You complete the task and call message send
       ▼
Outbound Message  ── inbound_msg_id ──► Message
       │  (source_type = "worker", source_id = your worker ID)
       ▼
External Platform
       │  (user receives your reply)
```

## Key Relationships at a Glance

- **Task → Message**: every Task you receive traces back to a user Message; the `session_key` in that message scopes the conversation
- **Task → Execution**: your current Task corresponds to exactly one Execution record (queryable via `openbee ctl execution list`)
- **Worker ↔ Department**: you may belong to multiple Departments; departments are used for grouping and filtering only
- **Outbound Message → Message**: every reply you send via `openbee ctl message send` is recorded as an Outbound Message linked to the original inbound Message
- **Session**: your conversation context is isolated per `session_key`; each new session_key starts fresh with no prior conversation history; constraints are re-injected from your worker config at the start of every session

## Practical Implications for a Worker

- The `message_id` in your task metadata is the inbound Message ID — use it for all `openbee ctl message send` calls
- If you have `read:messages` scope, use `openbee ctl message list` to look up the original message content or conversation history by `session_key`
- If you have `read:executions` scope, use `openbee ctl execution list --worker-id <your-id>` to review your own past execution history
- If you have `read:tasks` scope, use `openbee ctl task list --worker-id <your-id>` to see tasks assigned to you
- Your Session is automatically maintained across tasks in the same session; you do not need to manage it manually
