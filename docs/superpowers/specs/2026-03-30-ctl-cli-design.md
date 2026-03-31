# Design: `openbee ctl` CLI Subcommand

**Date:** 2026-03-30
**Status:** Approved

## Summary

Introduce an `openbee ctl` command namespace that exposes all MCP tool functionality as CLI subcommands. Targeted at automation/scripting use cases (CI/CD, shell scripts). All commands output raw JSON to stdout and use a new synchronous `/mcp/bee/call` HTTP endpoint that reuses existing MCP tool logic without SSE.

## Goals

- Enable scripting and automation against a running openbee server
- Full parity with all Bee MCP tools (workers, tasks, memory, session, system)
- JSON output suitable for `jq` pipelines
- Minimal new code: new HTTP endpoint reuses `MCPServer.callToolFn` directly
- Server connection config via env vars (CI-friendly) with config file fallback

## Non-Goals

- Interactive/human-readable output (use the Web UI for that)
- Worker MCP tool subset (only Bee tools are exposed)
- Offline/direct-DB operation (server must be running)

## Architecture

### Component Overview

```
openbee ctl worker list
       │
       ▼
cmd/openbee/ctl_worker.go   ← Cobra command, flag parsing
       │
       ▼
internal/ctlclient/client.go  ← HTTP client
       │  POST /mcp/bee/call {"name":"list_workers","arguments":{}}
       ▼
internal/mcp/server.go       ← HandleCall() handler
       │
       ▼
MCPServer.callToolFn()       ← existing tool logic (zero duplication)
```

### New HTTP Endpoint

`POST /mcp/bee/call` is added alongside the existing MCP SSE routes. It accepts a synchronous tool call and returns the result directly — no SSE session needed.

**Authentication:** same `X-API-Key` header as the existing MCP routes.

**Request:**
```json
{ "name": "list_workers", "arguments": {} }
```

**Success response (200):**
```json
{ "result": <tool return value> }
```

**Tool error response (200):**
```json
{ "error": "error message from tool" }
```

**Protocol errors:** standard HTTP status codes (400 bad request, 401 unauthorized).

Implementation: ~20 lines in `server.go` (`HandleCall` method) plus one route registration in `router.go`.

### CLI Client Package

`internal/ctlclient` encapsulates HTTP communication and server connection resolution:

```go
type Client struct {
    BaseURL string
    APIKey  string
}

// NewClient resolves connection config: env vars → config file → defaults.
func NewClient(cfgPath string) (*Client, error)

// Call invokes a named tool and returns the raw JSON result.
func (c *Client) Call(toolName string, args any) (json.RawMessage, error)
```

**Connection config resolution (in order):**
1. `OPENBEE_URL` env var (default: `http://localhost:8080`)
2. `OPENBEE_API_KEY` env var
3. Fallback: read `server.port` and `bee_api_key` from config file (path from `--config` flag or `config.yaml`)

### CLI Command Structure

New `openbee ctl` namespace under `cmd/openbee/`:

```
openbee ctl                              ← parent, shows help

  ctl worker list
  ctl worker get <id>
  ctl worker create --name <name> [--description <desc>] [--memory <mem>] [--work-dir <dir>]
  ctl worker update <id> [--name <name>] [--description <desc>] [--memory <mem>]
  ctl worker delete <id> [--delete-work-dir]
  ctl worker status <id>

  ctl task list (--session-key <key> | --message-id <id> | --worker-id <id>) [--status <s>] [--type <t>]
  ctl task create --message-id <id> --worker-id <id> --instruction <text> --type <type> [--scheduled-at <ms>] [--cron <expr>]
  ctl task cancel <id>

  ctl memory get --scope <scope> [--key <key>]
  ctl memory save --scope <scope> --key <key> --value <value>
  ctl memory delete --scope <scope> --key <key>

  ctl session list --session-key <key>
  ctl session clear --session-key <key> [--force]
  ctl session clear-worker --session-key <key> --worker-id <id>

  ctl system overview
  ctl system executions [--limit <n>]

  ctl message send --message-id <id> --content <text> [--media-path <path>]
```

Each command:
1. Parses flags into an `arguments` struct
2. Calls `ctlclient.Client.Call(toolName, arguments)`
3. On success: `json.MarshalIndent` result to stdout, exit 0
4. On failure: error message to stderr, exit 1

### i18n

All `Short`/`Long` descriptions for `ctl` commands are added to the i18n message struct and injected via `applyTranslations()` in `main.go`, consistent with existing CLI commands.

## Error Handling

| Scenario | Behavior |
|---|---|
| Server not running / connection refused | `error: cannot connect to openbee server at <url>` → stderr, exit 1 |
| Invalid API key (401) | `error: unauthorized – check OPENBEE_API_KEY` → stderr, exit 1 |
| Tool execution error | Tool's error message → stderr, exit 1 |
| Missing required flag | Cobra default error, exit 1 |
| Success | JSON pretty-printed to stdout, exit 0 |

## File Changes

**New files:**
```
internal/ctlclient/
  client.go           ← HTTP client + connection config resolution
  client_test.go      ← unit tests (httptest mock server)

cmd/openbee/
  ctl.go              ← openbee ctl parent command
  ctl_worker.go       ← ctl worker subcommands
  ctl_task.go         ← ctl task subcommands
  ctl_memory.go       ← ctl memory subcommands
  ctl_session.go      ← ctl session subcommands
  ctl_system.go       ← ctl system subcommands
  ctl_message.go      ← ctl message subcommands
```

**Modified files:**
```
internal/mcp/server.go          ← add HandleCall() HTTP handler
internal/api/router.go          ← register POST /mcp/bee/call route
internal/i18n/                  ← add ctl command translation keys
cmd/openbee/main.go             ← applyTranslations() for ctl commands
```

## Testing

- **`internal/ctlclient` unit tests**: use `httptest.NewServer` to mock the server; cover `Call()` success, connection failure, 401, and tool error scenarios
- **`/mcp/bee/call` endpoint integration tests**: added alongside `mcp/tools_test.go`, reusing existing test infrastructure
- **CLI command layer**: not unit tested (thin wrappers); covered by integration tests
