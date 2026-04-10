# Message Metadata JSON Format Design

**Date:** 2026-04-09
**Branch:** feat/engine-plugin
**Status:** Approved

## Background

On the `feat/engine-plugin` branch, when building prompts for bee or worker agents, a YAML-style frontmatter block is prepended to carry metadata:

```
---
from: feishu
session_key: feishu:oc_26e3020591e2ac027ff3b5e421ff6913:ou_db3b61b4c59a5d4e3d5333260229aafd
message_id: 91982a9b-c7f0-4558-b72e-0051e37ba780
---
```

This format has a known problem: the pi CLI misinterprets the leading `---` as an unknown CLI option, requiring a `stripFrontmatter` workaround. The goal is to replace YAML frontmatter with a more robust XML-tagged JSON format.

## New Format

### Bee Messages (`buildPrompt`)

```
<message_meta>{"from":"feishu","session_key":"feishu:oc_xxx:ou_xxx","message_id":"91982a9b-xxxx"}</message_meta>
<message_content>用户的实际消息内容</message_content>
```

Multiple messages in a single bee prompt are each wrapped individually and separated by a blank line.

### Worker Tasks (`buildInstruction`)

Full task (with both message_id and task_id):
```
<task_meta>{"message_id":"91982a9b-xxxx","task_id":"13c7c95f-xxxx"}</task_meta>
<task_content>任务指令内容</task_content>
```

Message-only (no task_id):
```
<task_meta>{"message_id":"91982a9b-xxxx"}</task_meta>
<task_content>任务指令内容</task_content>
```

No metadata (neither message_id nor task_id): pass instruction as-is, unchanged.

## Architecture

### Approach

Direct in-place replacement (Option 1). No new packages or abstractions. The two format-building functions are small and self-contained; extracting a shared package would add indirection without proportional benefit.

### Changed Files

| File | Change |
|------|--------|
| `internal/domain/bee/feeder.go` | Rewrite `buildPrompt()` to emit `<message_meta>` + `<message_content>` |
| `internal/domain/task/dispatcher.go` | Rewrite `buildInstruction()` to emit `<task_meta>` + `<task_content>` |
| `internal/ai/pi/invoker.go` | Delete `stripFrontmatter()` function; `buildArgs()` passes prompt directly |
| `internal/infra/skillinstall/skills/openbee-worker/SKILL.md` | Update "Task Input Metadata" section to describe new format |
| `internal/infra/skillinstall/skills/openbee-bee/SKILL.md` | Update message format description |

### Out of Scope

- Database schema — no change
- Backward compatibility shims — direct cutover, no dual-format support
- Other engine adapters (`claude`, `codex`) — no change needed; they receive the full prompt including tags
- `internal/ai/pi/invoker_test.go` — strip the `stripFrontmatter` unit tests

## Data Flow

```
Platform message received
        ↓
feeder.buildPrompt()
        ↓
<message_meta>{...}</message_meta>
<message_content>...</message_content>
        ↓
bee agent (reads message_meta for session_key, message_id)
        ↓
dispatcher.buildInstruction()
        ↓
<task_meta>{...}</task_meta>
<task_content>...</task_content>
        ↓
worker agent (reads task_meta for message_id, task_id)
```

## Skill Documentation Changes

### openbee-worker SKILL.md

Replace the "Task Input Metadata" section:

**Before:**
```yaml
---
task_id: <task_id>
message_id: <message_id>
---
```

**After:**
```
<task_meta>{"message_id": "<message_id>", "task_id": "<task_id>"}</task_meta>
<task_content>
任务指令内容
</task_content>
```

### openbee-bee SKILL.md

Update the incoming message format description to reflect `<message_meta>` + `<message_content>` tags carrying `from`, `session_key`, and `message_id`.

## Testing

- Update or remove unit tests for `stripFrontmatter` in `internal/ai/pi/invoker_test.go`
- Update unit tests for `buildPrompt` in `internal/domain/bee/feeder_test.go` (if they exist) to assert new format
- Update unit tests for `buildInstruction` in `internal/domain/task/dispatcher_test.go` to assert new format
