# Step-Based Session Prompt Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure the worker/bee new-session prompt header into explicit Step 1 (skill load + persona) and Step 2 (task) blocks to reduce skill-load skipping.

**Architecture:** Replace the public `ai.SkillHintPrefix` API with a single `ai.BuildSessionPrefix(role, persona)` function that emits the full Step-1 + Step-2 header. Both `task.TaskDispatcher.executeWithHint` and `bee.Feeder.processBeeGroup` switch to the new builder; the persona block is concatenated inside Step 1 instead of after the MANDATORY line.

**Tech Stack:** Go 1.22+, standard `testing` package, existing internal packages `internal/ai`, `internal/domain/task`, `internal/domain/bee`.

**Spec:** `docs/superpowers/specs/2026-05-09-step-based-session-prompt-design.md`

---

## File Structure

### Modified files
- `internal/ai/prompt.go` — add `BuildSessionPrefix`; remove `SkillHintPrefix` in the final task.
- `internal/ai/prompt_test.go` — replace 3 `TestSkillHintPrefix_*` cases with 4 `TestBuildSessionPrefix_*` cases.
- `internal/domain/task/dispatcher.go` (lines 337-348) — switch `executeWithHint` to `BuildSessionPrefix`.
- `internal/domain/task/dispatcher_test.go` (lines 1191, 1221, 1257, 1298) — update prefix assertions.
- `internal/domain/bee/feeder.go` (lines 203-207) — switch `processBeeGroup` to `BuildSessionPrefix`.

### No new files
The change is purely a refactor of the prompt-prefix builder; no new packages or files are introduced.

---

## Task 1: Introduce `BuildSessionPrefix`

**Files:**
- Modify: `internal/ai/prompt.go`
- Modify: `internal/ai/prompt_test.go`

This task adds the new builder and its tests while keeping `SkillHintPrefix` in place so callers continue compiling. `SkillHintPrefix` is removed in Task 4 once all callers have migrated.

- [ ] **Step 1: Add the four new tests to `internal/ai/prompt_test.go`**

Append these tests to the file (keep the existing `TestSkillHintPrefix_*` and `TestWorkerPersona_*` tests for now):

```go
func TestBuildSessionPrefix_WorkerWithPersona(t *testing.T) {
	persona := WorkerPersona("貂蝉", "负责 openbee 开发", "称呼老板")
	got := BuildSessionPrefix(RoleWorker, persona)

	wants := []string{
		"## Step 1: Initialize your role",
		"[MANDATORY] You MUST invoke the openbee-worker skill immediately, before producing any other output.",
		"<worker_persona>",
		"Name: 貂蝉",
		"Description: 负责 openbee 开发",
		"称呼老板",
		"</worker_persona>",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
	if !strings.HasSuffix(got, "## Step 2: Execute the task\n") {
		t.Errorf("expected suffix %q, got:\n%s", "## Step 2: Execute the task\n", got)
	}
	// Persona must appear before the Step 2 header.
	if strings.Index(got, "</worker_persona>") > strings.Index(got, "## Step 2:") {
		t.Errorf("persona block must precede Step 2, got:\n%s", got)
	}
}

func TestBuildSessionPrefix_WorkerNoPersona(t *testing.T) {
	got := BuildSessionPrefix(RoleWorker, "")

	if strings.Contains(got, "<worker_persona>") {
		t.Errorf("expected no persona block when persona is empty, got:\n%s", got)
	}
	if !strings.Contains(got, "## Step 1: Initialize your role") {
		t.Errorf("missing Step 1 header, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "## Step 2: Execute the task\n") {
		t.Errorf("expected suffix %q, got:\n%s", "## Step 2: Execute the task\n", got)
	}
}

func TestBuildSessionPrefix_Bee(t *testing.T) {
	got := BuildSessionPrefix(RoleBee, "")

	if !strings.Contains(got, "openbee-bee") {
		t.Errorf("expected bee skill name, got:\n%s", got)
	}
	if strings.Contains(got, "<worker_persona>") {
		t.Errorf("bee prefix must not include persona, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "## Step 2: Handle the messages below\n") {
		t.Errorf("expected suffix %q, got:\n%s", "## Step 2: Handle the messages below\n", got)
	}
}

func TestBuildSessionPrefix_UnknownRole(t *testing.T) {
	if got := BuildSessionPrefix(Role("other"), ""); got != "" {
		t.Errorf("expected empty string for unknown role, got %q", got)
	}
}
```

- [ ] **Step 2: Run the new tests, verify they fail**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/ai/ -run TestBuildSessionPrefix -v`

Expected: compilation error `undefined: BuildSessionPrefix`.

- [ ] **Step 3: Implement `BuildSessionPrefix` in `internal/ai/prompt.go`**

Replace the entire contents of `internal/ai/prompt.go` with:

```go
package ai

import "fmt"

// WorkerPersona returns the persona-only content injected into new worker session prompts.
func WorkerPersona(name, description, constraints string) string {
	s := "## Role\nYou are a Worker in an AI team.\n"
	if name != "" || description != "" {
		s += "\n## Identity\n"
	}
	if name != "" {
		s += fmt.Sprintf("Name: %s\n", name)
	}
	if description != "" {
		s += fmt.Sprintf("Description: %s\n", description)
	}
	if constraints != "" {
		s += fmt.Sprintf("\n## Work Constraints\n%s\n", constraints)
	}
	return s
}

// SkillHintPrefix returns the skill invocation hint prepended to the first
// message of a new session.
//
// Deprecated: callers should migrate to BuildSessionPrefix, which wraps the
// hint and persona in an explicit Step 1 / Step 2 structure. This function
// will be removed once all internal callers have been migrated.
func SkillHintPrefix(role Role) string {
	switch role {
	case RoleBee:
		return "[MANDATORY] You MUST invoke the openbee-bee skill immediately. This is your FIRST and ONLY action before doing anything else. Do NOT skip this step. Do NOT produce any text output before invoking the skill."
	case RoleWorker:
		return "[MANDATORY] You MUST invoke the openbee-worker skill immediately. This is your FIRST and ONLY action before doing anything else. Do NOT skip this step. Do NOT produce any text output before invoking the skill."
	default:
		return ""
	}
}

// BuildSessionPrefix returns the Step-1 + Step-2 header for a new session.
// The trailing "## Step 2: ...\n" line ends with a newline so the caller can
// append the task body directly without inserting a separator.
//
//	role    — RoleWorker or RoleBee. Selects the skill name and Step 2 title.
//	persona — Worker persona body produced by WorkerPersona(). Pass "" for Bee
//	          or when no worker record is available; the <worker_persona> block
//	          is omitted in that case.
//
// For unknown roles the function returns "", matching the legacy SkillHintPrefix
// behaviour so callers that previously checked for empty prefix keep working.
func BuildSessionPrefix(role Role, persona string) string {
	var skillName, step2Title string
	switch role {
	case RoleWorker:
		skillName = "openbee-worker"
		step2Title = "Execute the task"
	case RoleBee:
		skillName = "openbee-bee"
		step2Title = "Handle the messages below"
	default:
		return ""
	}

	s := "Please complete the following two steps in order. Do not skip Step 1.\n\n"
	s += "## Step 1: Initialize your role\n"
	s += fmt.Sprintf("[MANDATORY] You MUST invoke the %s skill immediately, before producing any other output.", skillName)

	if role == RoleWorker && persona != "" {
		s += " After the skill is loaded, internalize the persona below as your identity for the rest of this session:\n\n"
		s += "<worker_persona>\n" + persona + "</worker_persona>\n\n"
	} else {
		s += "\n\n"
	}

	s += fmt.Sprintf("## Step 2: %s\n", step2Title)
	return s
}
```

- [ ] **Step 4: Run all `internal/ai` tests, verify they pass**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/ai/ -v`

Expected: all `TestBuildSessionPrefix_*`, `TestSkillHintPrefix_*`, and `TestWorkerPersona_*` tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/prompt.go internal/ai/prompt_test.go
git commit -m "feat(ai): add BuildSessionPrefix for step-based session prompts"
```

---

## Task 2: Migrate `TaskDispatcher` to `BuildSessionPrefix`

**Files:**
- Modify: `internal/domain/task/dispatcher.go:333-349` (function `executeWithHint`)
- Modify: `internal/domain/task/dispatcher_test.go:1191`, `:1221`, `:1257`, `:1298`

- [ ] **Step 1: Update the four assertions in `internal/domain/task/dispatcher_test.go`**

Replace each occurrence of `ai.SkillHintPrefix(ai.RoleWorker)` with the new step marker. Concretely:

At line 1191 (inside `TestTaskDispatcher_NewSession_HasSkillHint` or equivalent — confirm by reading neighboring lines):

```go
if !strings.Contains(instruction, "## Step 1: Initialize your role") {
    t.Errorf("new session must start with Step 1 header\ngot: %q", instruction)
}
```

At line 1221 (resume test, negative assertion):

```go
if strings.Contains(instruction, "## Step 1: Initialize your role") {
    t.Errorf("resume session must NOT have Step 1 header\ngot: %q", instruction)
}
```

At line 1257 (`TestTaskDispatcher_NewSession_InjectsWorkerPersona`):

```go
if !strings.Contains(instr, "## Step 1: Initialize your role") {
    t.Errorf("instruction missing Step 1 header, got: %q", instr)
}
if strings.Index(instr, "<worker_persona>") > strings.Index(instr, "## Step 2:") {
    t.Errorf("persona block must appear before Step 2, got: %q", instr)
}
```

(Add the second assertion right after the first; keep the surrounding `<worker_persona>` / `Name: 毛毛` / `</worker_persona>` checks unchanged.)

At line 1298 (`TestTaskDispatcher_NewSession_NilLookup_OnlySkillHint`):

```go
if !strings.Contains(instr, "## Step 1: Initialize your role") {
    t.Errorf("instruction missing Step 1 header, got: %q", instr)
}
```

- [ ] **Step 2: Run dispatcher tests, verify they fail**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/domain/task/ -run TestTaskDispatcher_NewSession -v`

Expected: tests FAIL because the dispatcher is still emitting the old flat header without `## Step 1`.

- [ ] **Step 3: Update `executeWithHint` in `internal/domain/task/dispatcher.go`**

Replace lines 336-348 (the body of `executeWithHint` from the function declaration through the `return d.manager.ExecuteWorker(...)` line) with:

```go
func (d *TaskDispatcher) executeWithHint(ctx context.Context, task DispatchTask, instruction, engineName string, worker *model.Worker) (model.WorkerExecution, error) {
	persona := ""
	if d.workerLookup != nil {
		if worker == nil {
			return model.WorkerExecution{}, fmt.Errorf("worker %q not found", task.WorkerID)
		}
		persona = ai.WorkerPersona(worker.Name, worker.Description, worker.Constraints)
	}
	prefix := ai.BuildSessionPrefix(ai.RoleWorker, persona)
	sessionID := uuid.New().String()
	d.upsertSessionContext(ctx, task, sessionID, engineName)
	log.Info("executing worker", zap.String("workerID", task.WorkerID), zap.String("taskID", task.TaskID))
	return d.manager.ExecuteWorker(ctx, task.WorkerID, prefix+instruction, sessionID, false)
}
```

Note: the new prefix already ends with `\n`, so concatenate with `prefix+instruction` (no extra `\n`).

- [ ] **Step 4: Run dispatcher tests, verify they pass**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/domain/task/ -v`

Expected: all dispatcher tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/task/dispatcher.go internal/domain/task/dispatcher_test.go
git commit -m "refactor(task): use BuildSessionPrefix in TaskDispatcher"
```

---

## Task 3: Migrate `bee.Feeder` to `BuildSessionPrefix`

**Files:**
- Modify: `internal/domain/bee/feeder.go:203-207`

The bee feeder has no dedicated unit test for the prefix shape, so this task is a direct migration. Existing integration / behavioural tests will continue to exercise the change.

- [ ] **Step 1: Update `processBeeGroup` in `internal/domain/bee/feeder.go`**

Replace lines 203-207:

```go
hint := ""
if !resume {
    hint = ai.SkillHintPrefix(ai.RoleBee)
}
prompt := buildPrompt(msgs, hint)
```

with:

```go
prefix := ""
if !resume {
    prefix = ai.BuildSessionPrefix(ai.RoleBee, "")
}
prompt := buildPrompt(msgs, prefix)
```

- [ ] **Step 2: Run bee tests, verify the package still compiles and tests pass**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/domain/bee/ -v`

Expected: all tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/bee/feeder.go
git commit -m "refactor(bee): use BuildSessionPrefix in Feeder.processBeeGroup"
```

---

## Task 4: Remove the deprecated `SkillHintPrefix`

**Files:**
- Modify: `internal/ai/prompt.go`
- Modify: `internal/ai/prompt_test.go`

After Tasks 2 and 3, no production code references `SkillHintPrefix`. This task removes the deprecated function and its three tests so the public surface is clean.

- [ ] **Step 1: Sanity-check there are no remaining callers**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && grep -rn "SkillHintPrefix" --include="*.go" .`

Expected: matches only in `internal/ai/prompt.go` (definition) and `internal/ai/prompt_test.go` (the three tests we will delete in Step 3). If any other file matches, stop and migrate that caller before proceeding.

- [ ] **Step 2: Remove the function from `internal/ai/prompt.go`**

Delete the `SkillHintPrefix` function and its leading doc comment (the block from `// SkillHintPrefix returns ...` through the closing `}` after the `default: return ""` case). After the edit, `prompt.go` should contain only `WorkerPersona` and `BuildSessionPrefix`.

- [ ] **Step 3: Remove the three deprecated tests from `internal/ai/prompt_test.go`**

Delete `TestSkillHintPrefix_Bee`, `TestSkillHintPrefix_Worker`, and `TestSkillHintPrefix_Unknown` (lines 8-29 in the original file). Keep all `TestBuildSessionPrefix_*` and `TestWorkerPersona_*` tests. Verify `import "strings"` is still needed (it is, by the new tests).

- [ ] **Step 4: Run all tests in the affected packages, verify they pass**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/ai/ ./internal/domain/task/ ./internal/domain/bee/ -v`

Expected: every test passes; no `undefined: SkillHintPrefix` compile error anywhere.

- [ ] **Step 5: Run the full build to ensure nothing else referenced `SkillHintPrefix`**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./...`

Expected: clean build, no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/prompt.go internal/ai/prompt_test.go
git commit -m "refactor(ai): remove deprecated SkillHintPrefix"
```

---

## Task 5: End-to-end verification

**Files:** none

- [ ] **Step 1: Run the full test suite**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./...`

Expected: all packages pass.

- [ ] **Step 2: Manually inspect a generated prompt (optional but recommended)**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/ai/ -run TestBuildSessionPrefix_WorkerWithPersona -v`

Visually confirm the rendered prefix matches the spec's "Worker (with persona)" example.

- [ ] **Step 3: Confirm the four commits land in order**

Run: `git log --oneline -5`

Expected (top to bottom):
1. `refactor(ai): remove deprecated SkillHintPrefix`
2. `refactor(bee): use BuildSessionPrefix in Feeder.processBeeGroup`
3. `refactor(task): use BuildSessionPrefix in TaskDispatcher`
4. `feat(ai): add BuildSessionPrefix for step-based session prompts`
5. `docs(spec): add step-based session prompt design`
