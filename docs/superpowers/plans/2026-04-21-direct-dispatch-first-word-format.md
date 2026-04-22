# Direct Dispatch First-Word Format Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the broken space-prefix dispatch format with a no-prefix first-word format so that `workerName<space>instruction` and `workerName\ninstruction` trigger direct worker dispatch.

**Architecture:** Modify `parseDirectMention` in `feeder.go` to remove the leading-character guard (`@` or space) and instead split on the first space or newline to extract a candidate worker name. The existing `tryDirectDispatch` already validates the name via `workerLookup.GetByName()` and falls back to Bee if no match — no changes needed there. Update tests to reflect the removed space-prefix format and add newline-separator coverage.

**Tech Stack:** Go, SQLite (via `database/sql`), `strings` package

---

### Task 1: Write failing tests for the new formats

**Files:**
- Modify: `internal/domain/bee/feeder_test.go:679-732`

- [ ] **Step 1: Update `TestFeeder_DirectDispatch_SkipsBee` test cases**

Replace the `space-prefix` case with `no-prefix` (space removed) and add a `newline-prefix` case. Open `internal/domain/bee/feeder_test.go` and replace lines 680-686:

```go
// BEFORE
for _, tc := range []struct {
    name string
    msg  string
}{
    {"at-prefix", "@天天 write a report"},
    {"space-prefix", " 天天 write a report"},
} {
```

```go
// AFTER
for _, tc := range []struct {
    name string
    msg  string
}{
    {"at-prefix", "@天天 write a report"},
    {"no-prefix-space", "天天 write a report"},
    {"no-prefix-newline", "天天\nwrite a report"},
} {
```

- [ ] **Step 2: Update `TestFeeder_DirectDispatch_WorkerNotFound_FallsBackToBee` message**

In `feeder_test.go:623`, replace the space-prefixed message with the no-prefix format:

```go
// BEFORE
insertMessage(t, db, "m1", "sk1", " unknown do something")
```

```go
// AFTER
insertMessage(t, db, "m1", "sk1", "unknown do something")
```

- [ ] **Step 3: Run the dispatch tests to confirm failures**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/domain/bee/... -run TestFeeder_DirectDispatch -v
```

Expected output: `no-prefix-space` and `no-prefix-newline` subtests FAIL (bee runner is called when it shouldn't be); `at-prefix` and fallback tests PASS.

---

### Task 2: Implement `parseDirectMention` — first-word format

**Files:**
- Modify: `internal/domain/bee/feeder.go:351-367`

- [ ] **Step 1: Rewrite `parseDirectMention`**

Replace the entire function (lines 351-367) with:

```go
func parseDirectMention(content string) (workerName, instruction string, ok bool) {
	if len(content) == 0 {
		return "", "", false
	}
	if content[0] == '@' {
		rest := content[1:]
		workerName, instruction, ok = strings.Cut(rest, " ")
		if !ok || workerName == "" {
			return "", "", false
		}
		instruction = strings.TrimSpace(instruction)
		return workerName, instruction, instruction != ""
	}
	idx := strings.IndexAny(content, " \n")
	if idx <= 0 {
		return "", "", false
	}
	workerName = content[:idx]
	instruction = strings.TrimSpace(content[idx+1:])
	return workerName, instruction, instruction != ""
}
```

Note: `strings.IndexAny` is already in the standard library and `strings` is already imported in this file. No new imports needed.

- [ ] **Step 2: Run all dispatch tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/domain/bee/... -run TestFeeder_DirectDispatch -v
```

Expected output: all subtests PASS including `no-prefix-space`, `no-prefix-newline`, `at-prefix`, `WorkerNotFound`, and `NoPrefix`.

- [ ] **Step 3: Run the full test suite**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./...
```

Expected: all tests PASS with no failures.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/bee/feeder.go internal/domain/bee/feeder_test.go
git commit -m "feat(bee): support workerName+space/newline direct dispatch, remove broken space-prefix format"
```
