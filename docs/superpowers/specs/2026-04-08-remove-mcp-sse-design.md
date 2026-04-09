# Remove MCP SSE Streaming — Design Spec

**Date:** 2026-04-08  
**Decision:** B1 — Remove SSE streaming only; keep `/mcp/bee/call` endpoint unchanged.

---

## Background

The MCP server (`internal/ai/mcp`) currently serves two roles:

1. **SSE streaming interface** (`/mcp/bee/sse`, `/mcp/bee/messages`, `/mcp/worker/sse`, `/mcp/worker/messages`): Standard MCP protocol over Server-Sent Events, intended for AI clients such as Claude Desktop or Claude CLI.
2. **Direct call interface** (`/mcp/bee/call`): Simple HTTP POST endpoint used by `openbee ctl` commands.

The SSE interface has no external users. All communication with the openbee server now goes through `openbee ctl`, which calls `/mcp/bee/call`. The SSE layer is dead code and can be removed.

---

## Goal

Remove the SSE streaming layer and all associated code while leaving the `/mcp/bee/call` endpoint and `openbee ctl` functionality completely intact.

---

## Out of Scope

- Renaming the `mcp` package or the `/mcp/bee/call` URL path (deferred; low value).
- Changing the `ctlclient` package.
- Any changes to the auth middleware or JWT logic.

---

## Architecture After Change

```
openbee ctl <subcommand>
    └─> ctlclient.Client.Call(toolName, args)
            └─> POST /mcp/bee/call   (JSON: {name, arguments})
                    └─> mcp.MCPServer.HandleCall()
                            └─> beeCallTool(ctx, name, args)
```

The SSE session loop, JSON-RPC dispatch layer, and Worker MCP server are all gone.

---

## Changes

### 1. `internal/ai/mcp/tools.go`

**Delete:**
- `workerToolNames` map (4-entry allowlist)
- `workerToolSchemas()` function
- `WorkerToolSchemas()` exported function
- `workerCallTool()` method

These were exclusively used by the Worker SSE server.

### 2. `internal/ai/mcp/server.go`

**Delete from MCPServer struct:**
- `basePath string`
- `schemasFn func() []toolSchema`
- `mu sync.Mutex`
- `sessions map[string]chan rpcResponse`

**Delete functions/methods:**
- `rpcRequest`, `rpcResponse`, `rpcError` structs
- `errResponse()`, `okResponse()` helpers
- `NewWorkerServer()`
- `HandleSSE()`
- `HandleMessages()`
- `dispatch()`
- `handleToolCall()`

**Delete imports:** `net/url`, `sync`, `time`

**Update `NewBeeServer()`:** remove `basePath` assignment and `sessions` map initialization.

**Keep unchanged:** `HandleCall()`, `workerIDContext()`, `CallTool()`, all store fields, `ExecutionStopper` and `SessionClearer` interfaces.

### 3. `internal/api/router.go`

**Delete from `ServerParams` / `Server`:**
- `WorkerMCPServer *mcp.MCPServer` field

**Delete from `NewServer()` gzip exclusion list:**
- `"/mcp/.*/sse"`
- `"/mcp/.*/messages"`

**Delete from `registerMCPRoutes()`:**
- `beeGroup.GET("/sse", ...)` and `beeGroup.POST("/messages", ...)`
- Entire `workerGroup` block (3 lines)

**Keep unchanged:** `/mcp/bee/call` route and its auth middleware.

### 4. `internal/app/app.go`

**Delete:**
- `workerMCPSrv := mcp.NewWorkerServer(...)` call
- `WorkerMCPServer: workerMCPSrv` from `buildAPIServer` invocation
- `workerMCPSrv *mcp.MCPServer` parameter from `buildAPIServer()` signature

### 5. `internal/ai/mcp/tools_test.go`

**Delete:**
- `TestWorkerToolSchemasCount` test function

### 6. `internal/infra/config/config.go`

**Delete:**
- `MCPWorkerBasePath = "/mcp/worker"` constant

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Breaking external MCP clients | None | N/A | Confirmed zero external users |
| `openbee ctl` regression | Low | High | Verified: ctl calls `/mcp/bee/call`, which is untouched |
| Missed references to deleted symbols | Low | Low | `go build` catches all unused/missing references |

---

## Verification

1. `go build ./...` — must compile with no errors
2. `go test ./internal/ai/mcp/...` — all tests must pass
3. `go test ./...` — full test suite must pass
4. Manual smoke test: `openbee ctl worker list` must return correct output

---

## Net Change

- ~180 lines deleted
- ~20 lines modified
- 0 new lines added
