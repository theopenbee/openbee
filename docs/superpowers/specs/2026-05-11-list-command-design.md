# `/list` Slash Command Design

- Date: 2026-05-11
- Owner: 小乔
- Status: Approved (brainstorming)

## Goal

Add a new slash command `/list [keyword]` that prints the bee's worker directory to the IM channel.

- `/list` (no argument) — print every worker with its status and description.
- `/list {keyword}` — case-insensitive substring search against `worker.description`, print matches.

## Non-Goals

- No name/engine/permission_scopes filtering. Keyword matches description only.
- No quoted multi-word keywords. `strings.Fields` splits on whitespace; an extra arg returns Usage.
- No description truncation. Worker descriptions are short by convention; truncation would mislead.
- No session-level filtering. `/list` is the global worker directory; `/status` already covers the session view.

## Architecture

Add `ListCommandHandler` alongside the existing four handlers under `internal/domain/command/`. Each handler owns one command and the dispatch is performed by `msgingest.ChainHandlers` in `internal/app/app.go`.

```
inbound msg → msgingest.Dispatch → ChainHandlers(engine, clear, stop, status, list) → ListCommandHandler.HandleCommand
```

### Why a new handler, not reuse `/status`

- `/status` is **session-scoped**: lists active bee agents and running tasks for the current `sessionKey + beeEngine`.
- `/list` is **bee-global**: lists every row in `bee_workers`, independent of session state.

Different scope, different audience, different data sources — splitting keeps each handler simple and matches the established one-handler-per-command pattern.

## Data Access

```go
// internal/domain/command/list.go

// WorkerLister returns the full worker directory.
type WorkerLister interface {
    List() ([]model.Worker, error)
}
```

- `*store.WorkerStore` already satisfies `List()`, so the constructor accepts it directly.
- Filtering and sorting happen in memory. The worker table holds at most tens of rows; SQL filtering on `description` would require a schema-aware change to `WorkerFilter` that no other caller needs.
- Sort by `Name` ascending (`sort.SliceStable`) — predictable and stable across calls.

## Command Flow

```go
const CmdList = "/list"

type ListCommandHandler struct {
    workers WorkerLister
    senders map[string]platform.PlatformSenderAdapter
}

func (h *ListCommandHandler) IsCommand(content string) bool {
    return isExactOrPrefixed(content, CmdList)
}

func (h *ListCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
    fields := strings.Fields(content)
    if len(fields) == 0 || fields[0] != CmdList {
        return false
    }
    if len(fields) > 2 {
        h.reply(ctx, replyTo, i18n.M.Runtime.ListCommand.Usage)
        return true
    }
    keyword := ""
    if len(fields) == 2 {
        keyword = fields[1]
    }

    workers, err := h.workers.List()
    if err != nil {
        log.Error("list workers for /list", zap.Error(err))
        h.reply(ctx, replyTo, i18n.M.Runtime.ListCommand.LookupFailed)
        return true
    }

    if keyword != "" {
        kw := strings.ToLower(keyword)
        filtered := workers[:0:0]
        for _, w := range workers {
            if strings.Contains(strings.ToLower(w.Description), kw) {
                filtered = append(filtered, w)
            }
        }
        workers = filtered
    }
    sort.SliceStable(workers, func(i, j int) bool { return workers[i].Name < workers[j].Name })

    h.reply(ctx, replyTo, h.format(keyword, workers))
    return true
}
```

### Output format

Single line per worker, mirroring `/status` style:

```
员工列表（共 3 个）：
  - 小乔   状态: 空闲   负责 openbee 开发
  - 张三   状态: 工作中  …
  - 李四   状态: 异常   …
```

Search form:

```
匹配 "openbee" 的员工（共 1 个）：
  - 小乔   状态: 空闲   负责 openbee 开发
```

Empty states:

- No workers in DB → `员工列表（共 0 个）：\n  (暂无员工)`
- Keyword has no matches → `匹配 "xxx" 的员工（共 0 个）：\n  (无匹配的员工)`

Usage:

- `/list a b ...` → `用法：/list [关键词]`

### Status label translation

`statusLabel(model.WorkerStatus)` maps `idle / working / error` to localized strings; unknown values fall through to the raw enum string (defensive).

## i18n

Add `ListCommand` to `RuntimeMessages` in `internal/infra/i18n/messages.go`:

```go
type RuntimeMessages struct {
    // ... existing fields
    ListCommand ListCommandMessages `yaml:"list_command"`
}

type ListCommandMessages struct {
    Usage         string `yaml:"usage"`
    LookupFailed  string `yaml:"lookup_failed"`
    HeaderAll     string `yaml:"header_all"`     // %d worker count
    HeaderSearch  string `yaml:"header_search"`  // %q keyword, %d count
    EmptyAll      string `yaml:"empty_all"`
    EmptySearch   string `yaml:"empty_search"`
    Line          string `yaml:"line"`           // %s name, %s status, %s description
    StatusIdle    string `yaml:"status_idle"`
    StatusWorking string `yaml:"status_working"`
    StatusError   string `yaml:"status_error"`
}
```

Populate both `zh.yaml` and `en.yaml` to match existing 4 commands.

## Wiring

In `internal/app/app.go`:

```go
listCmdHandler := command.NewListCommandHandler(s.workerStore, sendersByPlatform)
cmdChain := msgingest.ChainHandlers(
    engineCmdHandler,
    clearCmdHandler,
    stopCmdHandler,
    statusCmdHandler,
    listCmdHandler,
)
```

## Testing

New file `internal/domain/command/list_test.go`, fake `WorkerLister`:

| Case | Input | Expected |
|---|---|---|
| Command detection | `/list`, `/list xxx`, `/listfoo` | `IsCommand` true / true / false |
| Empty DB | `List()` returns `[]` | reply uses `EmptyAll` |
| Full list | 3 workers | sorted by name asc, header shows count |
| Keyword match | description contains keyword | only matching workers returned |
| Case-insensitive | `OPENBEE` matches description `openbee` | hit |
| No matches | unknown keyword | reply uses `EmptySearch` |
| Too many args | `/list a b` | reply uses `Usage` |
| Lookup error | `List()` returns error | reply uses `LookupFailed`, error logged |
| Status labels | one worker per status | each status uses its localized label |

## Files Changed

- `internal/domain/command/list.go` (new)
- `internal/domain/command/list_test.go` (new)
- `internal/infra/i18n/messages.go` (add `ListCommand` to `RuntimeMessages`)
- `internal/infra/i18n/locales/zh.yaml` (add `list_command` section)
- `internal/infra/i18n/locales/en.yaml` (add `list_command` section)
- `internal/app/app.go` (build and register handler in chain)
- `CHANGELOG.md` (one-line English entry under unreleased)

## Risks / Considerations

- Worker table assumed small (tens of rows). If it ever grows to thousands, in-memory filter is still fine for an IM-triggered admin command — IM reply size will be the real ceiling.
- Description is free-form; users may include newlines. The `Line` template puts description last so a single-line output remains valid even with multi-line description; we do not strip newlines for the MVP.
