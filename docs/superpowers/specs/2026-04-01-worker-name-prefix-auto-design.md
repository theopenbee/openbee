# Worker Name Prefix Auto-Injection Design

**Date:** 2026-04-01
**Branch:** feat/ctl-cli
**Status:** Approved

## Background

`openbee-worker/skill.md` currently requires every worker to manually prefix its outbound messages with the worker's name:

```bash
openbee ctl message send --message-id <id> --content "毛毛: 任务完成"
```

This requirement exists so that users can identify which worker sent a given message. However, the dynamic API key feature (JWT tokens, implemented in `feat/ctl-cli`) encodes `worker_id` in every token. The MCP auth middleware already extracts this value and stores it in the request context under `CtxKeyWorkerID`. The server therefore knows the caller's identity on every `send_message` call — making the manual prefix redundant.

## Goal

Move the name-prefix responsibility from the worker agent to the server:

1. `toolSendMessage` automatically prepends `"<worker_name>: "` to the content when the caller is a worker.
2. `openbee-worker/skill.md` drops the name-prefix requirement; workers send plain content.

## Decisions

| Question | Decision |
|---|---|
| Who adds the prefix? | Server (`toolSendMessage`) |
| Does Bee get a prefix? | No — only worker tokens carry a `worker_id`; Bee token's `worker_id` is empty |
| Transition strategy | One-shot: update server and skill.md together |
| Fallback when worker is deleted | Use `worker_id` as the prefix |

## Design

### 1. Thread caller identity through the call stack (`internal/mcp/server.go`)

Change the `callToolFn` field type from:

```go
callToolFn func(name string, args json.RawMessage) (any, error)
```

to:

```go
callToolFn func(ctx context.Context, name string, args json.RawMessage) (any, error)
```

In `HandleCall`, extract the worker ID from the gin context and inject it into a `context.Context` before calling `callToolFn`:

```go
func (s *MCPServer) HandleCall(c *gin.Context) {
    // ...bind req...
    ctx := context.WithValue(context.Background(), CtxKeyWorkerID, c.GetString(CtxKeyWorkerID))
    result, err := s.callToolFn(ctx, req.Name, req.Arguments)
    // ...
}
```

For the SSE path, thread the context through the full call chain: `HandleMessages` extracts the worker ID from the gin context and passes it as a `context.Context` to `dispatch`, which passes it to `handleToolCall`, which passes it to `callToolFn`. This means `dispatch` and `handleToolCall` also receive a `ctx context.Context` parameter.

### 2. Auto-prepend prefix in `toolSendMessage` (`internal/mcp/tools.go`)

Update `beeCallTool`, `workerCallTool`, and `toolSendMessage` to accept `ctx context.Context` as the first parameter.

In `toolSendMessage`, after unmarshalling params, prepend the worker name when the caller is a worker:

```go
if workerID, _ := ctx.Value(CtxKeyWorkerID).(string); workerID != "" && params.Content != "" {
    name := workerID // fallback: use worker_id if worker record is not found
    if w, err := s.workerStore.GetByID(workerID); err == nil {
        name = w.Name
    }
    params.Content = name + ": " + params.Content
}
```

Bee tokens have an empty `worker_id`, so the prefix block is skipped entirely for Bee.

### 3. Update `openbee-worker/skill.md`

- Remove the rule that message content must be prefixed with the worker's name.
- Remove the `"姓名: 消息内容"` format requirement from the notification spec.
- Update all examples to show plain content (no name prefix):

```bash
openbee ctl message send --message-id <id> --content "已收到任务，正在处理。"
openbee ctl message send --message-id <id> --content "第一阶段完成，已修改 foo.go。下一步开始更新测试。"
openbee ctl message send --message-id <id> --content "任务完成。已修改 3 个文件，所有测试通过。"
```

## Files Changed

| File | Change |
|---|---|
| `internal/mcp/server.go` | `callToolFn` signature; `HandleCall` and `handleToolCall` inject caller `worker_id` into context |
| `internal/mcp/tools.go` | `beeCallTool`, `workerCallTool`, `toolSendMessage` accept `context.Context`; prefix logic added |
| `openbee-worker/skill.md` (local skill file) | Remove name-prefix requirement and update examples |

## Out of Scope

- Modifying how the Bee coordinator formats its own messages.
- Adding prefix to media-only messages (no `content` field); those are forwarded unchanged.
- Retroactively modifying stored execution logs or historical messages.
