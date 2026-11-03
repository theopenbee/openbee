# openbee API Reference

The `execute` tool of the `openbee` MCP server runs a JavaScript snippet against the openbee API and returns what the snippet returns. The snippet is the body of an async function: `await` the calls and `return` the value you want; objects come back as JSON. `console.log` output comes back too. Write plain JavaScript — no TypeScript syntax, imports, network, file system or timers. Every `openbee.*` call returns a Promise that rejects with an `Error` when the call fails; catch it with `try`/`catch` when a script should go on. The tool description lists every call with its TypeScript signature. A script stops after 50 seconds.

Talk to the user with the `send_message` tool, not with `execute`.

Each `js` block below is the `code` of one `execute` call. Combine calls and trim results in one script instead of making many tool calls.

## Workers

```js
return await openbee.list_workers({ name: "mao", page: 1, page_size: 50 });
```

```js
return await openbee.get_worker({ worker_id: "<id>" });
```

```js
return await openbee.get_worker_status({ worker_id: "<id>" });
```

```js
return await openbee.create_worker({
  name: "<name>",
  description: "<description>",
  constraints: "<constraints>",
  work_dir: "<directory>",
  engine: "<engine>",
  permission_scopes: "read:workers,read:tasks",
  engine_args: { claude: "--model claude-sonnet-4-6 --effort high" },
});
```

```js
return await openbee.update_worker({ worker_id: "<id>", name: "<name>", engine_args: { claude: "" } });
```

```js
return await openbee.delete_worker({ worker_id: "<id>", delete_work_dir: false });
```

- `list_workers`: `name` matches part of the name, case-insensitive; `id` matches exactly; default 50 per page, max 200
- `update_worker` changes only the fields you pass
- `engine` is one of the enabled engines; leave it out for the server default
- `engine_args` holds extra CLI flags per engine. On update, an empty string removes that engine's flags and `{}` clears all of them
- `work_dir` is the worker's working directory; on update, pass a new path to change it
- `delete_work_dir: true` also deletes the working directory from disk

### Worker Permission Scopes

Permission scopes control which read-only calls a worker may make. The Bee is never scope-restricted. Workers can only send messages and make the read-only calls below; every other call is refused for them.

| Scope | Grants access to |
|---|---|
| `read:workers` | `list_workers`, `get_worker`, `get_worker_status` |
| `read:tasks` | `list_tasks` |
| `read:messages` | `list_messages`, `list_outbound_messages` |

A worker without the scope gets `permission denied: scope <scope> required`.

`permission_scopes` is a comma-separated list and replaces all scopes; `""` clears them:

```js
return await openbee.update_worker({ worker_id: "<id>", permission_scopes: "read:workers,read:tasks,read:messages" });
```

## Tasks

```js
return await openbee.create_task({ message_id: "<id>", worker_id: "<worker id>", instruction: "<instruction>", type: "immediate" });
```

```js
return await openbee.create_task({ message_id: "<id>", worker_id: "<worker id>", instruction: "<instruction>", type: "countdown", scheduled_at: Date.now() + 5 * 60 * 1000 });
```

```js
return await openbee.create_task({ message_id: "<id>", worker_id: "<worker id>", instruction: "<instruction>", type: "scheduled", cron_expr: "0 9 * * *" });
```

```js
return await openbee.list_tasks({ session_key: "<session_key>", type: "scheduled", status: "pending" });
```

```js
return await openbee.cancel_task({ task_id: "<id>" });
```

- `type` is `immediate`, `countdown` or `scheduled`. `countdown` needs `scheduled_at` (Unix ms, at least 5 seconds ahead); `scheduled` needs `cron_expr` (5-field cron). An invalid `cron_expr` returns `status: "cancelled"` with a `reason`
- `list_tasks` needs at least one of `task_id`, `message_id`, `session_key` or `worker_id`; `task_id` cannot be combined with the others, and `message_id` excludes `session_key`
- Each task carries an `executions` array with its newest execution records: 10 by default, `execution_limit` sets another count, and `execution_limit: 0` with `task_id` returns the full history of one task
- Task results are paginated (default 50 per page, max 100) as `{ items, total, page, page_size }`. Task fields are capitalized: `ID`, `Instruction`, `Status`, `Type`, `WorkerID`, `SessionKey`, `CronExpr`, `NextRunAt`

## Constraints

```js
return await openbee.get_constraint({ scope: "global" });
```

```js
return await openbee.get_constraint({ scope: "<session_key>", key: "<key>" });
```

```js
return await openbee.save_constraint({ scope: "<global|session_key>", key: "<key>", value: "<value>" });
```

```js
return await openbee.delete_constraint({ scope: "<global|session_key>", key: "<key>" });
```

## Sessions

```js
return await openbee.list_session_contexts({ session_key: "<session_key>" });
```

```js
return await openbee.clear_session({ session_key: "<session_key>", force: false });
```

```js
return await openbee.clear_worker_session({ session_key: "<session_key>", worker_id: "<id>", force: false });
```

## System

```js
return await openbee.get_system_overview();
```

## Messages

Send with the `send_message` tool. `message_id` replies to that message; `session_key` sends a new message to the conversation without replying to any message. `media_path` takes one local file per call; with `content` in the same call, the text goes first. Each line is the arguments of one call:

```json
{ "message_id": "<id>", "content": "Task has been dispatched to Maomao, please wait." }
{ "message_id": "<id>", "content": "System status screenshot attached.", "media_path": "/tmp/overview.png" }
{ "message_id": "<id>", "content": "Here is the exported task list.", "media_path": "/tmp/tasks.xlsx" }
```

Several files take several calls:

```json
{ "message_id": "<id>", "content": "2 attachments in total, sending in order." }
{ "message_id": "<id>", "media_path": "/tmp/file1.png" }
{ "message_id": "<id>", "media_path": "/tmp/file2.pdf" }
```

Query message history with `execute`:

```js
return await openbee.list_messages({ session_key: "<platform>:<chat_id>:<user_id>", status: "received" });
```

```js
return await openbee.list_messages({ platform: "<platform>", received_at_from: 1700000000000, received_at_to: 1700086400000, status: "bee_processed" });
```

```js
return await openbee.list_outbound_messages({ source_type: "worker", source_id: "<worker id>", status: "failed" });
```

- `list_messages` status: `received`, `feeding`, `bee_processed`, `merged`, `failed`; time filters `received_at_from` / `received_at_to`
- `list_outbound_messages` status: `sent`, `failed`; `source_type`: `bee`, `worker`, `system`; time filters `sent_at_from` / `sent_at_to`
- Times are Unix milliseconds; pagination defaults to 50 per page, max 100; all filters combine freely
