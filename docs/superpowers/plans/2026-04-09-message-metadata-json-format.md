# Message Metadata JSON Format Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace YAML-style frontmatter metadata in bee and worker messages with XML-tagged JSON, and remove the now-unnecessary `stripFrontmatter` workaround in the pi engine.

**Architecture:** Direct in-place replacement across three Go files and two skill markdown files. No new packages. The two format-building functions (`buildPrompt`, `buildInstruction`) are updated to emit `<message_meta>`/`<message_content>` and `<task_meta>`/`<task_content>` blocks respectively. The pi invoker's `stripFrontmatter` function and its call site are deleted entirely.

**Tech Stack:** Go, standard library only (`encoding/json`, `fmt`, `strings`)

---

### Task 1: Update `buildInstruction` in dispatcher and fix its test

**Files:**
- Modify: `internal/domain/task/dispatcher.go:236-244`
- Modify: `internal/domain/task/dispatcher_test.go:188-227`

- [ ] **Step 1: Update the test to assert the new format**

Open `internal/domain/task/dispatcher_test.go`. Find `TestTaskDispatcher_InstructionInjection` (line 188). Replace the three assertions about the old YAML format:

```go
// REPLACE these three assertions:
if !strings.HasPrefix(instr, "---\n") {
    t.Errorf("instruction missing frontmatter prefix, got: %q", instr)
}
if !strings.Contains(instr, "task_id: task-abc") {
    t.Errorf("instruction missing task_id injection, got: %q", instr)
}
if !strings.Contains(instr, "message_id: msg-xyz") {
    t.Errorf("instruction missing message_id injection, got: %q", instr)
}
```

With:

```go
wantMeta := `<task_meta>{"message_id":"msg-xyz","task_id":"task-abc"}</task_meta>`
if !strings.HasPrefix(instr, wantMeta) {
    t.Errorf("instruction missing task_meta prefix, got: %q", instr)
}
if !strings.Contains(instr, "<task_content>") {
    t.Errorf("instruction missing task_content tag, got: %q", instr)
}
if !strings.Contains(instr, "do the thing") {
    t.Errorf("instruction missing original content, got: %q", instr)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/domain/task/... -run TestTaskDispatcher_InstructionInjection -v
```

Expected: FAIL — assertion about `<task_meta>` prefix fails.

- [ ] **Step 3: Update `buildInstruction` in dispatcher.go**

Open `internal/domain/task/dispatcher.go`. Replace the entire `buildInstruction` function (lines 236–244):

```go
// buildInstruction prepends task metadata to the instruction so workers
// can call mark_task_success and send_message via MCP.
func buildInstruction(t DispatchTask) string {
	if t.TaskID != "" {
		meta := fmt.Sprintf(`{"message_id":%q,"task_id":%q}`, t.MessageID, t.TaskID)
		return fmt.Sprintf("<task_meta>%s</task_meta>\n<task_content>\n%s\n</task_content>", meta, t.Instruction)
	}
	if t.MessageID != "" {
		meta := fmt.Sprintf(`{"message_id":%q}`, t.MessageID)
		return fmt.Sprintf("<task_meta>%s</task_meta>\n<task_content>\n%s\n</task_content>", meta, t.Instruction)
	}
	return t.Instruction
}
```

Add `"fmt"` to the import block if not already present (it is — no change needed).

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/domain/task/... -run TestTaskDispatcher_InstructionInjection -v
```

Expected: PASS.

- [ ] **Step 5: Run full dispatcher test suite**

```bash
go test ./internal/domain/task/... -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/task/dispatcher.go internal/domain/task/dispatcher_test.go
git commit -m "feat(task): replace YAML frontmatter with XML-tagged JSON in buildInstruction"
```

---

### Task 2: Update `buildPrompt` in feeder

**Files:**
- Modify: `internal/domain/bee/feeder.go:303-313`

No existing unit test for `buildPrompt` — add one.

- [ ] **Step 1: Add a unit test for `buildPrompt`**

Open `internal/domain/bee/feeder_test.go`. At the bottom of the file, add:

```go
func TestBuildPrompt(t *testing.T) {
    msgs := []store.ClaimedMessage{
        {ID: "msg-1", Platform: "feishu", SessionKey: "feishu:oc_abc:ou_xyz", Content: "hello world"},
    }
    got := buildPrompt(msgs)
    wantMeta := `<message_meta>{"from":"feishu","session_key":"feishu:oc_abc:ou_xyz","message_id":"msg-1"}</message_meta>`
    if !strings.HasPrefix(got, wantMeta) {
        t.Errorf("missing message_meta prefix\ngot:  %q", got)
    }
    if !strings.Contains(got, "<message_content>") {
        t.Errorf("missing message_content tag, got: %q", got)
    }
    if !strings.Contains(got, "hello world") {
        t.Errorf("missing original content, got: %q", got)
    }
}

func TestBuildPrompt_MultipleMessages(t *testing.T) {
    msgs := []store.ClaimedMessage{
        {ID: "msg-1", Platform: "feishu", SessionKey: "sk1", Content: "first"},
        {ID: "msg-2", Platform: "feishu", SessionKey: "sk1", Content: "second"},
    }
    got := buildPrompt(msgs)
    if !strings.Contains(got, "msg-1") || !strings.Contains(got, "msg-2") {
        t.Errorf("missing message IDs, got: %q", got)
    }
    if strings.Count(got, "<message_meta>") != 2 {
        t.Errorf("expected 2 message_meta blocks, got: %q", got)
    }
}
```

Make sure `"strings"` is already in the import block of the test file (it is — no change needed).

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/domain/bee/... -run TestBuildPrompt -v
```

Expected: FAIL — `<message_meta>` prefix not found.

- [ ] **Step 3: Update `buildPrompt` in feeder.go**

Open `internal/domain/bee/feeder.go`. Replace the `buildPrompt` function (lines 303–313):

```go
func buildPrompt(msgs []store.ClaimedMessage) string {
	var sb strings.Builder
	for i, m := range msgs {
		if i > 0 {
			sb.WriteByte('\n')
		}
		meta := fmt.Sprintf(`{"from":%q,"session_key":%q,"message_id":%q}`, m.Platform, m.SessionKey, m.ID)
		fmt.Fprintf(&sb, "<message_meta>%s</message_meta>\n<message_content>\n%s\n</message_content>\n",
			meta, m.Content)
	}
	return sb.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/domain/bee/... -run TestBuildPrompt -v
```

Expected: PASS.

- [ ] **Step 5: Run full feeder test suite**

```bash
go test ./internal/domain/bee/... -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/bee/feeder.go internal/domain/bee/feeder_test.go
git commit -m "feat(bee): replace YAML frontmatter with XML-tagged JSON in buildPrompt"
```

---

### Task 3: Delete `stripFrontmatter` from pi invoker

**Files:**
- Modify: `internal/ai/pi/invoker.go:49-67`
- Modify: `internal/ai/pi/invoker_test.go` (remove `TestStripFrontmatter`)

- [ ] **Step 1: Delete `TestStripFrontmatter` from invoker_test.go**

Open `internal/ai/pi/invoker_test.go`. Remove the entire `TestStripFrontmatter` function and its test cases (the function that calls `stripFrontmatter(tc.input)`). Keep all other tests intact.

Also remove the `"strings"` import if it's only used by `TestStripFrontmatter` — check if `strings` is used elsewhere in the file first.

- [ ] **Step 2: Run tests to verify the remaining pi tests still pass**

```bash
go test ./internal/ai/pi/... -v
```

Expected: PASS (only `TestStripFrontmatter` is gone, others remain green). If there's a compile error about `strings` being unused, remove it from the import.

- [ ] **Step 3: Delete `stripFrontmatter` and update `buildArgs` in invoker.go**

Open `internal/ai/pi/invoker.go`. Delete lines 49–63 (the `stripFrontmatter` function). Then update `buildArgs`:

```go
func buildArgs(prompt, sessionPath string) []string {
	return []string{"--mode", "json", "--session", sessionPath, "-p", prompt}
}
```

Also remove `"strings"` from the import block in `invoker.go` if it was only used by `stripFrontmatter`.

- [ ] **Step 4: Verify the package compiles and all pi tests pass**

```bash
go test ./internal/ai/pi/... -v
```

Expected: all tests PASS, no compile errors.

- [ ] **Step 5: Run full project build check**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/pi/invoker.go internal/ai/pi/invoker_test.go
git commit -m "refactor(pi): remove stripFrontmatter — no longer needed with XML-tagged prompt format"
```

---

### Task 4: Update openbee-worker skill documentation

**Files:**
- Modify: `internal/infra/skillinstall/skills/openbee-worker/SKILL.md`

- [ ] **Step 1: Update the "Task Input Metadata" section**

Open `internal/infra/skillinstall/skills/openbee-worker/SKILL.md`. Find the "Task Input Metadata" section (around line 40). Replace it entirely:

```markdown
## Task Input Metadata

The scheduler injects task metadata at the beginning of the task body in the following format:

```
<task_meta>{"message_id": "<message_id>", "task_id": "<task_id>"}</task_meta>
<task_content>
任务指令内容
</task_content>
```

- Use `message_id` as the target for all `openbee ctl message send` calls
- Treat `task_id` as a tracking identifier; you do not need to update task status yourself
- After completing the actual work and sending results, end the task directly; task success or failure is determined by the worker process exit status
```

- [ ] **Step 2: Commit**

```bash
git add internal/infra/skillinstall/skills/openbee-worker/SKILL.md
git commit -m "docs(skill): update openbee-worker task metadata format to XML-tagged JSON"
```

---

### Task 5: Update openbee-bee skill documentation

**Files:**
- Modify: `internal/infra/skillinstall/skills/openbee-bee/SKILL.md`

- [ ] **Step 1: Add an "Incoming Message Format" section**

Open `internal/infra/skillinstall/skills/openbee-bee/SKILL.md`. After the `## ⚠️ Operation Mode` block (after line 22, before `## Task Delegation Flow`), insert a new section:

```markdown
## Incoming Message Format

Each user message you receive is wrapped in XML tags that carry routing metadata:

```
<message_meta>{"from":"feishu","session_key":"feishu:oc_xxx:ou_xxx","message_id":"91982a9b-xxxx"}</message_meta>
<message_content>
用户的实际消息内容
</message_content>
```

- `from` — the platform the message came from (e.g. `feishu`, `telegram`, `local`)
- `session_key` — the session identifier; use this when calling `openbee ctl session list --session-key` or `openbee ctl memory get --scope`
- `message_id` — use this when calling `openbee ctl message send --message-id` to reply to the user
- The actual user text is inside `<message_content>` — this is what you analyze for task delegation
```

- [ ] **Step 2: Commit**

```bash
git add internal/infra/skillinstall/skills/openbee-bee/SKILL.md
git commit -m "docs(skill): document incoming message XML-tagged JSON format for bee"
```

---

### Task 6: Final verification

- [ ] **Step 1: Run the full test suite**

```bash
go test ./... 2>&1 | tail -20
```

Expected: all packages PASS, no failures.

- [ ] **Step 2: Verify build is clean**

```bash
go build ./...
```

Expected: exits with code 0, no output.

- [ ] **Step 3: Confirm no remaining references to the old YAML format in production code**

```bash
grep -rn '"\-\-\-\\n"' internal/ --include="*.go"
grep -rn 'from:.*session_key\|task_id:.*message_id' internal/ --include="*.go"
grep -rn 'stripFrontmatter' internal/ --include="*.go"
```

Expected: no matches in any of the three commands.
