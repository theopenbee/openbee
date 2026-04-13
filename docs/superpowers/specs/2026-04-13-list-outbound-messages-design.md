# Design: List Outbound Messages

**Date:** 2026-04-13  
**Status:** Approved  

## Background

`openbee ctl message list` queries `bee_platform_messages` (inbound messages received from platforms). The `bee_outbound_messages` table stores messages sent out to platforms (replies from bee/worker/system), but there is no general-purpose query API for it — only `ListBySessionKey` with cursor-based pagination exists.

## Goal

Add `openbee ctl message list-outbound` CLI command and a corresponding `list_outbound_messages` MCP Tool to query outbound messages with flexible filtering and pagination.

## Scope

- **In scope:** Store layer (new `ListFiltered` method), MCP Tool, CLI command
- **Out of scope:** HTTP API (not needed at this time)

## Design

### 1. Store Layer

**File:** `internal/infra/store/outbound_message_store.go`

Add filter struct, slim list struct, and `ListFiltered` method:

```go
type OutboundMessageFilter struct {
    SessionKey string
    Platform   string
    Status     string // "sent" | "failed"
    SourceType string // "bee" | "worker" | "system"
    SourceID   string
    SentAtFrom int64  // inclusive lower bound (Unix ms); 0 = no lower bound
    SentAtTo   int64  // inclusive upper bound (Unix ms); 0 = no upper bound
}

type ListedOutboundMessage struct {
    ID           string `json:"id"`
    SessionKey   string `json:"session_key"`
    Platform     string `json:"platform"`
    Content      string `json:"content"`
    Status       string `json:"status"`
    SourceType   string `json:"source_type"`
    SourceID     string `json:"source_id"`
    InboundMsgID string `json:"inbound_msg_id"`
    Error        string `json:"error"`
    SentAt       int64  `json:"sent_at"`
}

func (s *OutboundMessageStore) ListFiltered(ctx context.Context, filter OutboundMessageFilter, limit, offset int) ([]ListedOutboundMessage, int, error)
```

The method builds a dynamic WHERE clause from the filter fields and returns both the page of results and the total count.

### 2. Tool Name Constant

**File:** `internal/infra/utils/toolnames.go`

```go
ListOutboundMessages = "list_outbound_messages"
```

### 3. MCP Tool

**File:** `internal/mcp/tools.go`

New method `toolListOutboundMessages(ctx, args)` registered under the name `list_outbound_messages`. Parameters mirror the CLI flags. Returns paginated JSON response with `items`, `total`, `page`, `page_size`.

### 4. CLI Command

**File:** `cmd/openbee/ctl_message.go`

New subcommand `list-outbound` under the `message` command group, calling `ctlRun(utils.ListOutboundMessages, args)`.

Flags:

| Flag | Type | Description |
|------|------|-------------|
| `--session-key` | string | Filter by session key |
| `--platform` | string | Filter by platform (feishu, local) |
| `--status` | string | Filter by status (sent, failed) |
| `--source-type` | string | Filter by source type (bee, worker, system) |
| `--source-id` | string | Filter by source ID |
| `--sent-from` | int64 | sent_at >= value (Unix ms) |
| `--sent-to` | int64 | sent_at <= value (Unix ms) |
| `--page` | int | Page number (default: 1) |
| `--page-size` | int | Page size (default: 50, max: 100) |

### Usage Examples

```bash
# List all outbound messages
openbee ctl message list-outbound

# List outbound messages for a specific session
openbee ctl message list-outbound --session-key feishu_xxx

# List messages sent by a specific worker
openbee ctl message list-outbound --source-type worker --source-id <worker_id>

# List failed outbound messages
openbee ctl message list-outbound --status failed

# List messages sent in a time range
openbee ctl message list-outbound --sent-from 1713000000000 --sent-to 1713086400000
```

## Data Flow

```
CLI (ctl message list-outbound)
  → ctlRun(utils.ListOutboundMessages, args)
    → MCP Server: toolListOutboundMessages()
      → OutboundMessageStore.ListFiltered()
        → bee_outbound_messages table
```

## Files to Modify

1. `internal/infra/store/outbound_message_store.go` — add filter struct, slim struct, `ListFiltered` method
2. `internal/infra/utils/toolnames.go` — add `ListOutboundMessages` constant
3. `internal/mcp/tools.go` — add `toolListOutboundMessages` method and register tool
4. `cmd/openbee/ctl_message.go` — add `list-outbound` subcommand
