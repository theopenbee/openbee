# Session Prefix Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `WorkerPersona` / `BuildWorkerSessionPrefix` / `BuildBeeSessionPrefix` / `writePrefixStep1` with a single role-driven `core.BuildSessionPrompt(SessionRequest)` entry point in `internal/ai/core/`, eliminating the leaked "prefix string" intermediate and consolidating role differences into one config table.

**Architecture:** New file `internal/ai/core/session.go` exposes `SessionRequest`, `WorkerIdentity`, and `BuildSessionPrompt`. A private `rolePrefixSpecs` map keyed by `ai.Role` carries each role's `skillName`, `step2Header`, and `personaTag`. Business callers (`dispatcher.go`, `feeder.go`) move to import both `internal/ai` (for `Role`) and `internal/ai/core` (for `BuildSessionPrompt`); the four old `ai` helpers are deleted.

**Tech Stack:** Go 1.x, standard library only. Existing test framework (`go test`).

---

## Implementation refinement vs spec

The spec described preserving the bee blank-line by having `assembleMessages` prepend `\n` to the first message. That introduces a stray leading `\n` on the bee **resume** path (where `BuildSessionPrompt` returns `Content` verbatim). Cleaner approach used in this plan: encode the trailing whitespace in each role's `step2Header` itself:

- `RoleWorker.step2Header = "## Step 2: Execute the task\n"` (single `\n` — matches old worker behavior)
- `RoleBee.step2Header    = "## Step 2: Handle the messages below\n\n"` (extra `\n` so prefix+content reproduces the old blank line)

`assembleMessages` then produces content with no leading whitespace. `BuildSessionPrompt` stays as a single `prefix + Content` join with no role branch, and the bee resume path returns clean `Content` with no leading newline. Design intent (no role branch in the joiner) is preserved.

---

## File Structure

**New:**
- `internal/ai/core/session.go` — public `SessionRequest`, `WorkerIdentity`, `BuildSessionPrompt`; private `sessionPrefixSpec`, `rolePrefixSpecs`, `buildSessionPrefix`, `(WorkerIdentity).persona()`.
- `internal/ai/core/session_test.go` — tests for `BuildSessionPrompt` covering worker (with/without persona), bee, and resume.

**Modified:**
- `internal/ai/ai.go` — delete Section 5 prompt helpers (`WorkerPersona`, `BuildWorkerSessionPrefix`, `BuildBeeSessionPrefix`, `writePrefixStep1`). Leave `EngineArgsMap` / `ParseEngineArgs` / `MergeEngineArgs` / `ParseEngineArgsJSON` / `splitCLIArgs` untouched.
- `internal/ai/ai_test.go` — delete `TestWorkerPersona_*` / `TestBuildWorkerSessionPrefix_*` / `TestBuildBeeSessionPrefix` (lines roughly 162-249).
- `internal/domain/task/dispatcher.go` — `executeFresh` (~lines 337-350) switches to `core.BuildSessionPrompt`. `resolveExecution` resume branch (~line 365) also routed through `core.BuildSessionPrompt(SessionRequest{Resume: true})`.
- `internal/domain/bee/feeder.go` — `drainSession` (~lines 203-207) switches to `core.BuildSessionPrompt`; `buildPrompt` (~lines 338-358) renamed/refactored to `assembleMessages(msgs)`.

---

## Task 1: Create `core/session.go` with types, impl, and tests (TDD)

**Files:**
- Create: `internal/ai/core/session.go`
- Create: `internal/ai/core/session_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/ai/core/session_test.go`:

```go
package core_test

import (
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/core"
)

func TestBuildSessionPrompt_Worker_WithPersona(t *testing.T) {
	req := core.SessionRequest{
		Role: ai.RoleWorker,
		Identity: core.WorkerIdentity{
			Name:        "貂蝉",
			Description: "负责 openbee 开发",
			Constraints: "称呼老板",
		},
		Content: "do the thing",
	}
	got := core.BuildSessionPrompt(req)

	wants := []string{
		"## Step 1: Initialize your role",
		"[MANDATORY] You MUST invoke the openbee-worker skill immediately, before producing any other output.",
		"<worker_persona>",
		"## Role\nYou are a Worker in an AI team.",
		"Name: 貂蝉",
		"Description: 负责 openbee 开发",
		"## Work Constraints",
		"称呼老板",
		"</worker_persona>",
		"## Step 2: Execute the task",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
	if !strings.HasSuffix(got, "## Step 2: Execute the task\ndo the thing") {
		t.Errorf("expected suffix step2 header followed directly by content, got:\n%s", got)
	}
	if strings.Index(got, "</worker_persona>") > strings.Index(got, "## Step 2:") {
		t.Errorf("persona block must precede Step 2, got:\n%s", got)
	}
}

func TestBuildSessionPrompt_Worker_NoPersona(t *testing.T) {
	req := core.SessionRequest{
		Role:    ai.RoleWorker,
		Content: "do the thing",
	}
	got := core.BuildSessionPrompt(req)

	if strings.Contains(got, "<worker_persona>") {
		t.Errorf("expected no persona block when identity is zero, got:\n%s", got)
	}
	if !strings.Contains(got, "## Step 1: Initialize your role") {
		t.Errorf("missing Step 1 header, got:\n%s", got)
	}
	if !strings.Contains(got, "openbee-worker") {
		t.Errorf("missing worker skill name, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "## Step 2: Execute the task\ndo the thing") {
		t.Errorf("expected suffix step2 header followed directly by content, got:\n%s", got)
	}
}

func TestBuildSessionPrompt_Bee(t *testing.T) {
	req := core.SessionRequest{
		Role:    ai.RoleBee,
		Content: "<message_meta>{}</message_meta>\n<message_content>\nhi\n</message_content>\n",
	}
	got := core.BuildSessionPrompt(req)

	if !strings.Contains(got, "openbee-bee") {
		t.Errorf("expected bee skill name, got:\n%s", got)
	}
	if strings.Contains(got, "<worker_persona>") {
		t.Errorf("bee prefix must not include persona, got:\n%s", got)
	}
	if !strings.Contains(got, "## Step 2: Handle the messages below") {
		t.Errorf("missing bee step 2 header, got:\n%s", got)
	}
	// Bee preserves a blank line between Step 2 header and first message
	// (i.e. "## Step 2: Handle the messages below\n\n<message_meta>...")
	if !strings.Contains(got, "## Step 2: Handle the messages below\n\n<message_meta>") {
		t.Errorf("expected blank line between bee step 2 header and first message, got:\n%s", got)
	}
}

func TestBuildSessionPrompt_Resume(t *testing.T) {
	content := "just the instruction"
	cases := []struct {
		name string
		req  core.SessionRequest
	}{
		{"worker_resume", core.SessionRequest{Role: ai.RoleWorker, Resume: true, Content: content, Identity: core.WorkerIdentity{Name: "x"}}},
		{"bee_resume", core.SessionRequest{Role: ai.RoleBee, Resume: true, Content: content}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := core.BuildSessionPrompt(tc.req)
			if got != content {
				t.Errorf("resume must return Content verbatim\nwant: %q\ngot:  %q", content, got)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail to compile**

Run: `go test ./internal/ai/core/... -run TestBuildSessionPrompt -v`
Expected: build failure — `undefined: core.SessionRequest`, `undefined: core.WorkerIdentity`, `undefined: core.BuildSessionPrompt`.

- [ ] **Step 3: Write the implementation**

Create `internal/ai/core/session.go`:

```go
package core

import (
	"fmt"
	"strings"

	"github.com/theopenbee/openbee/internal/ai"
)

// WorkerIdentity describes a worker's identity to embed in the <worker_persona>
// block. The zero value yields no persona block.
type WorkerIdentity struct {
	Name        string
	Description string
	Constraints string
}

// SessionRequest is the complete input required to build a session prompt.
// Identity is only consulted when Role is ai.RoleWorker.
type SessionRequest struct {
	Role     ai.Role
	Identity WorkerIdentity
	Resume   bool
	Content  string
}

// BuildSessionPrompt returns a full session prompt. When Resume is true it
// returns Content verbatim (no prefix). Otherwise it prepends a role-specific
// prefix (Step 1 + optional persona block + Step 2 header) to Content.
func BuildSessionPrompt(req SessionRequest) string {
	if req.Resume {
		return req.Content
	}
	return buildSessionPrefix(req.Role, req.Identity.persona()) + req.Content
}

type sessionPrefixSpec struct {
	skillName   string
	step2Header string
	personaTag  string // empty string means "this role does not support persona"
}

var rolePrefixSpecs = map[ai.Role]sessionPrefixSpec{
	ai.RoleWorker: {
		skillName:   "openbee-worker",
		step2Header: "## Step 2: Execute the task\n",
		personaTag:  "worker_persona",
	},
	ai.RoleBee: {
		// Trailing "\n\n" preserves the blank line that used to sit between
		// the bee step 2 header and the first <message_meta> entry.
		skillName:   "openbee-bee",
		step2Header: "## Step 2: Handle the messages below\n\n",
		personaTag:  "",
	},
}

func buildSessionPrefix(role ai.Role, persona string) string {
	spec := rolePrefixSpecs[role]
	var sb strings.Builder
	sb.WriteString("Please complete the following two steps in order. Do not skip Step 1.\n\n")
	sb.WriteString("## Step 1: Initialize your role\n")
	fmt.Fprintf(&sb, "[MANDATORY] You MUST invoke the %s skill immediately, before producing any other output.\n\n", spec.skillName)
	if persona != "" && spec.personaTag != "" {
		sb.WriteString("After the skill is loaded, internalize the persona below as your identity for the rest of this session:\n\n")
		fmt.Fprintf(&sb, "<%s>\n", spec.personaTag)
		sb.WriteString(persona)
		fmt.Fprintf(&sb, "</%s>\n\n", spec.personaTag)
	}
	sb.WriteString(spec.step2Header)
	return sb.String()
}

// persona converts a WorkerIdentity into the body that is wrapped inside the
// <worker_persona> block. Returns "" when the identity is the zero value.
func (id WorkerIdentity) persona() string {
	if id.Name == "" && id.Description == "" && id.Constraints == "" {
		return ""
	}
	s := "## Role\nYou are a Worker in an AI team.\n"
	if id.Name != "" || id.Description != "" {
		s += "\n## Identity\n"
	}
	if id.Name != "" {
		s += fmt.Sprintf("Name: %s\n", id.Name)
	}
	if id.Description != "" {
		s += fmt.Sprintf("Description: %s\n", id.Description)
	}
	if id.Constraints != "" {
		s += fmt.Sprintf("\n## Work Constraints\n%s\n", id.Constraints)
	}
	return s
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ai/core/... -run TestBuildSessionPrompt -v`
Expected: all four tests PASS.

- [ ] **Step 5: Run the entire core package test suite to catch regressions**

Run: `go test ./internal/ai/core/...`
Expected: PASS (no impact to existing core tests).

- [ ] **Step 6: Commit**

```bash
git add internal/ai/core/session.go internal/ai/core/session_test.go
git commit -m "feat(ai/core): add BuildSessionPrompt with role-driven prefix"
```

---

## Task 2: Migrate `dispatcher.go` to `core.BuildSessionPrompt`

**Files:**
- Modify: `internal/domain/task/dispatcher.go` (`executeFresh` ~lines 337-350; `resolveExecution` ~line 365)

- [ ] **Step 1: Add `core` import**

In `internal/domain/task/dispatcher.go`, locate the import block and add the `core` import next to the existing `ai` import:

```go
import (
	// ... existing imports ...
	"github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/core"
	// ... existing imports ...
)
```

- [ ] **Step 2: Rewrite `executeFresh` to use `core.BuildSessionPrompt`**

Replace the body of `executeFresh` between the function signature and the `return d.manager.ExecuteWorker(...)` line. Find this current block:

```go
persona := ""
if d.workerLookup != nil {
	if worker == nil {
		return model.WorkerExecution{}, fmt.Errorf("worker %q not found", task.WorkerID)
	}
	persona = ai.WorkerPersona(worker.Name, worker.Description, worker.Constraints)
}
prefix := ai.BuildWorkerSessionPrefix(persona)
sessionID := uuid.New().String()
d.upsertSessionContext(ctx, task, sessionID, engineName)
log.Info("executing worker", zap.String("workerID", task.WorkerID), zap.String("taskID", task.TaskID))
return d.manager.ExecuteWorker(ctx, task.WorkerID, prefix+instruction, sessionID, false)
```

Replace with:

```go
identity := core.WorkerIdentity{}
if d.workerLookup != nil {
	if worker == nil {
		return model.WorkerExecution{}, fmt.Errorf("worker %q not found", task.WorkerID)
	}
	identity = core.WorkerIdentity{
		Name:        worker.Name,
		Description: worker.Description,
		Constraints: worker.Constraints,
	}
}
prompt := core.BuildSessionPrompt(core.SessionRequest{
	Role:     ai.RoleWorker,
	Identity: identity,
	Content:  instruction,
})
sessionID := uuid.New().String()
d.upsertSessionContext(ctx, task, sessionID, engineName)
log.Info("executing worker", zap.String("workerID", task.WorkerID), zap.String("taskID", task.TaskID))
return d.manager.ExecuteWorker(ctx, task.WorkerID, prompt, sessionID, false)
```

- [ ] **Step 3: Route the resume path in `resolveExecution` through `core.BuildSessionPrompt`**

Locate this line in `resolveExecution`:

```go
exec, err := d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, sessionID, true)
```

Replace with:

```go
resumePrompt := core.BuildSessionPrompt(core.SessionRequest{
	Role:    ai.RoleWorker,
	Resume:  true,
	Content: instruction,
})
exec, err := d.manager.ExecuteWorker(ctx, task.WorkerID, resumePrompt, sessionID, true)
```

(Since `Resume: true` returns `Content` verbatim, the wire-level behavior is identical; this keeps all `ExecuteWorker` prompt assembly behind the single entry point.)

- [ ] **Step 4: Build the package to check for stale references**

Run: `go build ./internal/domain/task/...`
Expected: build succeeds. (At this stage `ai.WorkerPersona` and `ai.BuildWorkerSessionPrefix` are no longer referenced from this file — but they still exist in `ai.go`, so the project as a whole still compiles.)

- [ ] **Step 5: Run dispatcher tests**

Run: `go test ./internal/domain/task/... -run TestTaskDispatcher -v`
Expected: PASS. The existing assertions (`TestTaskDispatcher_NewSession_InjectsWorkerPersona`, `TestTaskDispatcher_NewSession_NilLookup_NoPersona`, and the resume test) check for the same prompt structure, which the new path produces.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/task/dispatcher.go
git commit -m "refactor(task): use core.BuildSessionPrompt for worker dispatch"
```

---

## Task 3: Migrate `feeder.go` and split `buildPrompt`

**Files:**
- Modify: `internal/domain/bee/feeder.go` (`drainSession` ~lines 203-207; `buildPrompt` ~lines 338-358)

- [ ] **Step 1: Add `core` import**

In `internal/domain/bee/feeder.go`, locate the existing `ai` import and add `core` alongside it:

```go
import (
	// ... existing imports ...
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/core"
	// ... existing imports ...
)
```

- [ ] **Step 2: Replace the prefix block in `drainSession`**

Find:

```go
prefix := ""
if !resume {
	prefix = ai.BuildBeeSessionPrefix()
}
prompt := buildPrompt(msgs, prefix)
```

Replace with:

```go
prompt := core.BuildSessionPrompt(core.SessionRequest{
	Role:    ai.RoleBee,
	Resume:  resume,
	Content: assembleMessages(msgs),
})
```

- [ ] **Step 3: Rename `buildPrompt` to `assembleMessages` and strip the prefix logic**

Replace the existing function:

```go
func buildPrompt(msgs []store.ClaimedMessage, prefix string) string {
	var sb strings.Builder
	sb.Grow(len(msgs) * 128)
	if prefix != "" {
		sb.WriteString(prefix)
		sb.WriteByte('\n')
	}
	for i, m := range msgs {
		if i > 0 {
			sb.WriteByte('\n')
		}
		meta := messageMeta{
			From:       m.Platform,
			SessionKey: m.SessionKey,
			MessageID:  m.ID,
		}
		b, _ := json.Marshal(meta)
		fmt.Fprintf(&sb, "<message_meta>%s</message_meta>\n<message_content>\n%s\n</message_content>\n", b, m.Content)
	}
	return sb.String()
}
```

With:

```go
// assembleMessages renders the bee message section in the
// <message_meta>/<message_content> envelope. Messages are separated by a
// single blank line. No leading or trailing whitespace.
func assembleMessages(msgs []store.ClaimedMessage) string {
	var sb strings.Builder
	sb.Grow(len(msgs) * 128)
	for i, m := range msgs {
		if i > 0 {
			sb.WriteByte('\n')
		}
		meta := messageMeta{
			From:       m.Platform,
			SessionKey: m.SessionKey,
			MessageID:  m.ID,
		}
		b, _ := json.Marshal(meta)
		fmt.Fprintf(&sb, "<message_meta>%s</message_meta>\n<message_content>\n%s\n</message_content>\n", b, m.Content)
	}
	return sb.String()
}
```

- [ ] **Step 4: Build the package to check for stale references**

Run: `go build ./internal/domain/bee/...`
Expected: build succeeds. `ai.BuildBeeSessionPrefix` is no longer referenced from feeder.

- [ ] **Step 5: Run bee tests**

Run: `go test ./internal/domain/bee/... -v`
Expected: PASS. If any existing test asserts on the exact prompt string and expects a `\n` after a non-empty prefix, the joined output (`prefix(ends with \n\n) + messagesText(no leading whitespace)`) is byte-equivalent to the old `prefix + \n + messagesText`. Resume case: `Content` returned verbatim, identical to old behavior (prefix was empty).

- [ ] **Step 6: Commit**

```bash
git add internal/domain/bee/feeder.go
git commit -m "refactor(bee): use core.BuildSessionPrompt, split buildPrompt"
```

---

## Task 4: Delete old API and tests in `internal/ai/`

**Files:**
- Modify: `internal/ai/ai.go` (Section 5, ~lines 241-286)
- Modify: `internal/ai/ai_test.go` (~lines 162-249)

- [ ] **Step 1: Delete the four prompt helpers from `ai.go`**

In `internal/ai/ai.go`, find Section 5's comment header:

```go
// =========================================================
// Section 5: Helper utilities (from prompt.go + engine_args.go)
// =========================================================
```

Delete the block starting at `// WorkerPersona returns the persona-only content injected into new worker session prompts.` up to (but not including) the line `type EngineArgsMap map[string][]string`. That removes:

- `WorkerPersona`
- `BuildWorkerSessionPrefix`
- `BuildBeeSessionPrefix`
- `writePrefixStep1`

Update the Section 5 comment header to reflect remaining scope:

```go
// =========================================================
// Section 5: Engine argument helpers (from engine_args.go)
// =========================================================
```

Verify no leftover imports are unused: if `unicode` is still needed by `splitCLIArgs` keep it. `slices` is used by `MergeEngineArgs`. `encoding/json` is used by `ParseEngineArgsJSON`. `fmt` and `strings` are used elsewhere in this file. So no import block changes expected; run `go vet` after the edit to confirm.

- [ ] **Step 2: Delete the obsolete tests in `ai_test.go`**

Open `internal/ai/ai_test.go`. Delete the following test functions in full (each is delimited by `func TestX(t *testing.T) {` to its closing `}`):

- `TestWorkerPersona_Full`
- `TestWorkerPersona_Empty`
- `TestBuildWorkerSessionPrefix_WithPersona`
- `TestBuildWorkerSessionPrefix_NoPersona`
- `TestBuildBeeSessionPrefix`

After deletion, check that the `strings` import is still used by remaining tests in the file (it almost certainly is). If not, remove it.

- [ ] **Step 3: Build the entire module**

Run: `go build ./...`
Expected: build succeeds with no `undefined: ai.WorkerPersona` / `ai.BuildWorkerSessionPrefix` / `ai.BuildBeeSessionPrefix` errors. If any error appears, locate the offending file and update its call site (only `dispatcher.go` and `feeder.go` referenced these symbols per the pre-flight grep; this step is the safety net).

- [ ] **Step 4: Run `go vet`**

Run: `go vet ./...`
Expected: clean. Any "imported and not used" error here flags a leftover import to remove.

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`
Expected: PASS across the board.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/ai.go internal/ai/ai_test.go
git commit -m "refactor(ai): remove obsolete prompt helpers superseded by core"
```

---

## Final verification

- [ ] **Step 1: Confirm grep is clean**

Run: `git grep -n 'WorkerPersona\|BuildWorkerSessionPrefix\|BuildBeeSessionPrefix\|writePrefixStep1' -- '*.go'`
Expected: no matches in `.go` files (only the docs/specs/plans under `docs/superpowers/` may still reference these for historical context).

- [ ] **Step 2: Confirm the only public surface added is in `core`**

Run: `git grep -n 'BuildSessionPrompt\|SessionRequest\|core.WorkerIdentity' -- '*.go'`
Expected: definitions in `internal/ai/core/session.go`, tests in `internal/ai/core/session_test.go`, and call sites in `internal/domain/task/dispatcher.go` and `internal/domain/bee/feeder.go`.

- [ ] **Step 3: Run the full test suite once more**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Push the branch (if applicable)**

```bash
git push
```
