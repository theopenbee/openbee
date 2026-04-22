# Direct Worker Dispatch — First-Word Format

**Date:** 2026-04-21  
**Status:** Approved

---

## Problem

IM platforms (Lark, DingTalk, etc.) trim leading and trailing spaces from messages before delivering them. The existing space-prefix format (` workerName instruction`) was added as an alternative to `@mention`, but it never arrives with the leading space intact, making it completely unusable in practice.

## Goal

Support direct worker dispatch when a message starts with `workerName` followed by a space or newline, with no prefix character required.

New supported formats:
- `天天 写份报告` (worker name + space + instruction)
- `天天\n写份报告` (worker name + newline + instruction)

---

## Design

### Core Change: `parseDirectMention` in `feeder.go`

The function currently rejects any message whose first character is not `@` or ` `. The new logic:

1. **`@mention` path (unchanged):** If the message starts with `@`, strip it and split on the first space to extract `workerName` and `instruction`.
2. **First-word path (new):** Otherwise, find the first occurrence of `' '` or `'\n'` in the content. Everything before it is the candidate `workerName`; everything after (trimmed) is the `instruction`. If no separator is found, or if either part is empty, return no match.

```
parseDirectMention(content):
  if empty → no match
  if content[0] == '@':
    rest = content[1:]
    workerName, instruction = split(rest, first " ")
    if workerName empty → no match
    return workerName, TrimSpace(instruction), instruction != ""
  else:
    idx = IndexAny(content, " \n")
    if idx <= 0 → no match
    workerName = content[0:idx]
    instruction = TrimSpace(content[idx+1:])
    return workerName, instruction, instruction != ""
```

`tryDirectDispatch` is unchanged — it validates the extracted `workerName` via `workerLookup.GetByName()` and falls back to Bee if the worker is not found.

### Behavior Matrix

| Message | Before | After |
|---------|--------|-------|
| `@天天 写报告` | Dispatches to 天天 | Dispatches to 天天 (unchanged) |
| `天天 写报告` | Falls to Bee | Dispatches to 天天 ✅ |
| `天天\n写报告` | Falls to Bee | Dispatches to 天天 ✅ |
| ` 天天 写报告` (leading space) | Dispatches (unreachable via IM) | Falls to Bee (format removed) |
| `random message here` | Falls to Bee | Falls to Bee (lookup fails, fallback) |

### False Positive Risk

Every non-`@`-prefixed message now has its first word extracted and checked against the worker store. This is acceptable because:

- Worker names are proper nouns specific to the team (e.g., `天天`, `小乔`)
- The instruction must be non-empty for dispatch to trigger
- `GetByName` uses a SQLite `LOWER(name) = LOWER(?)` query — negligible latency
- If the first word matches a worker name accidentally, the user can rephrase or use a different opener

---

## Files to Change

| File | Change |
|------|--------|
| `internal/domain/bee/feeder.go` | Rewrite `parseDirectMention` (~10 lines) |
| `internal/domain/bee/feeder_test.go` | Update `space-prefix` test cases to use no-prefix format; add newline-separator cases |

---

## Out of Scope

- No changes to `tryDirectDispatch`, `GetByName`, or any other dispatch plumbing
- No changes to the `@mention` path
- No gateway-level changes
