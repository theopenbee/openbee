# openbee API Reference (Worker)

The `execute` tool of the `openbee` MCP server runs a JavaScript snippet against the openbee API and returns what the snippet returns. The snippet is the body of an async function: `await` the calls and `return` the value you want. `console.log` output comes back too. Write plain JavaScript — no TypeScript syntax, imports, network, file system or timers. Every `openbee.*` call returns a Promise that rejects with an `Error` when the call fails. The tool description lists every call with its TypeScript signature.

Talk to the user with the `send_message` tool, not with `execute`.

## Read-Only Queries (Require Permission Scopes)

These calls work only if the administrator has granted this worker the matching permission scope. Your token is attached automatically.

If a call fails with `permission denied: scope <scope> required`, this worker lacks that scope. Tell the user which scope is missing; the Bee or the openbee web console can grant it.

**`read:workers`**

```js
return await openbee.list_workers({ name: "mao" });
```

```js
return await openbee.get_worker({ worker_id: "<id>" });
```

```js
return await openbee.get_worker_status({ worker_id: "<id>" });
```

**`read:tasks`**

```js
return await openbee.list_tasks({ worker_id: "<your-id>", status: "pending" });
```

```js
return await openbee.list_tasks({ task_id: "<id>", execution_limit: 0 });
```

`list_tasks` needs at least one of `task_id`, `message_id`, `session_key` or `worker_id`. Each task carries an `executions` array with its newest execution records: 10 by default, `execution_limit` sets another count, and `execution_limit: 0` with `task_id` returns the full history of that task. Results are paginated (default 50 per page, max 100) as `{ items, total, page, page_size }`.

**`read:messages`**

```js
return await openbee.list_messages({ session_key: "<platform>:<chat_id>:<user_id>", status: "received" });
```

```js
const since = Date.now() - 24 * 60 * 60 * 1000;
const { items } = await openbee.list_outbound_messages({ source_type: "worker", source_id: "<your-id>", sent_at_from: since });
return items.map((m) => ({ sent_at: m.sent_at, status: m.status, content: m.content.slice(0, 80) }));
```

- `list_messages` status: `received`, `feeding`, `bee_processed`, `merged`, `failed`; time filters `received_at_from` / `received_at_to`
- `list_outbound_messages` status: `sent`, `failed`; `source_type`: `bee`, `worker`, `system`; time filters `sent_at_from` / `sent_at_to`
- Times are Unix milliseconds; pagination defaults to 50 per page, max 100
- All filters combine freely

Combine calls and trim results in one script instead of making many tool calls:

```js
const { items } = await openbee.list_tasks({ worker_id: "<your-id>", status: "failed", page_size: 20 });
return items.map((t) => ({ id: t.ID, instruction: t.Instruction, error: t.executions[0]?.result }));
```

There is no standalone execution query. Use the `executions` array of each task from `list_tasks`.
