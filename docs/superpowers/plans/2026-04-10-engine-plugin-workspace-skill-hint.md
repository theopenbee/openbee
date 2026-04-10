# Engine Plugin: Workspace Simplification & Skill Hint Prefix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove `.openbee.md` from codex/pi engine workspaces and inject a `use openbee-<role> skill.` prefix into the first message of every new session.

**Architecture:** Two coordinated changes: (1) `SetupWorkspace()` writes persona-only AGENTS.md and skips `.openbee.md`; (2) `buildPrompt()` (bee) and `resolveExecution()` (worker) prepend a skill hint string produced by a new `SkillHintPrefix()` helper when starting a fresh session. Claude engine is untouched.

**Tech Stack:** Go, standard library, existing `testing` package patterns.

---

## File Map

| File | Change |
|------|--------|
| `internal/ai/rules.go` | Add `WorkerPersona()` and `SkillHintPrefix()` |
| `internal/ai/workspace.go` | Remove `.openbee.md` write; use `WorkerPersona()` in AGENTS.md |
| `internal/ai/workspace_test.go` | Rewrite bee/worker assertions; remove `.openbee.md` checks |
| `internal/ai/codex/adapter_test.go` | Remove `.openbee.md` existence checks |
| `internal/ai/pi/adapter_test.go` | Remove `.openbee.md` existence checks |
| `internal/domain/bee/feeder.go` | `buildPrompt()` gains `skillHint string` param |
| `internal/domain/bee/feeder_internal_test.go` | Update `buildPrompt` call sites; add hint tests |
| `internal/domain/task/dispatcher.go` | `resolveExecution()` prepends hint on new/fresh sessions |
| `internal/domain/task/dispatcher_test.go` | Add tests asserting hint presence/absence |

---

## Task 1: Add `WorkerPersona` and `SkillHintPrefix` to `rules.go`

**Files:**
- Modify: `internal/ai/rules.go`

- [ ] **Step 1: Write failing tests for the two new functions**

Add a new test file `internal/ai/rules_test.go`:

```go
package ai

import (
	"strings"
	"testing"
)

func TestSkillHintPrefix_Bee(t *testing.T) {
	got := SkillHintPrefix(RoleBee)
	if got != "use openbee-bee skill." {
		t.Errorf("got %q, want %q", got, "use openbee-bee skill.")
	}
}

func TestSkillHintPrefix_Worker(t *testing.T) {
	got := SkillHintPrefix(RoleWorker)
	if got != "use openbee-worker skill." {
		t.Errorf("got %q, want %q", got, "use openbee-worker skill.")
	}
}

func TestSkillHintPrefix_Unknown(t *testing.T) {
	got := SkillHintPrefix(Role("other"))
	if got != "" {
		t.Errorf("expected empty string for unknown role, got %q", got)
	}
}

func TestWorkerPersona_Full(t *testing.T) {
	got := WorkerPersona("mybot", "does things", "remember X")
	if !strings.Contains(got, "You are a Worker in an AI team.") {
		t.Errorf("missing persona line, got: %q", got)
	}
	if !strings.Contains(got, "Name: mybot") {
		t.Errorf("missing name, got: %q", got)
	}
	if !strings.Contains(got, "Description: does things") {
		t.Errorf("missing description, got: %q", got)
	}
	if !strings.Contains(got, "## Memory Constraints") {
		t.Errorf("missing memory header, got: %q", got)
	}
	if !strings.Contains(got, "remember X") {
		t.Errorf("missing memory content, got: %q", got)
	}
	if strings.Contains(got, "openbee-worker") {
		t.Errorf("persona must NOT contain skill rule directive, got: %q", got)
	}
}

func TestWorkerPersona_Empty(t *testing.T) {
	got := WorkerPersona("", "", "")
	if got != "You are a Worker in an AI team.\n" {
		t.Errorf("got %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /path/to/openbee
go test ./internal/ai/... -run "TestSkillHintPrefix|TestWorkerPersona" -v
```

Expected: FAIL — `SkillHintPrefix` and `WorkerPersona` undefined.

- [ ] **Step 3: Add `WorkerPersona` and `SkillHintPrefix` to `rules.go`**

Append to `internal/ai/rules.go`:

```go
// WorkerPersona returns the persona-only content for a worker's AGENTS.md.
// It contains identity (name, description, memory) but no rule directives.
// Distinct from WorkerRules, which is still used by the Claude engine.
func WorkerPersona(name, description, memory string) string {
	s := "You are a Worker in an AI team.\n"
	if name != "" {
		s += fmt.Sprintf("Name: %s\n", name)
	}
	if description != "" {
		s += fmt.Sprintf("Description: %s\n", description)
	}
	if memory != "" {
		s += fmt.Sprintf("\n## Memory Constraints\n%s\n", memory)
	}
	return s
}

// SkillHintPrefix returns the skill invocation hint prepended to the first
// message of a new session for codex/pi engines.
func SkillHintPrefix(role Role) string {
	switch role {
	case RoleBee:
		return "use openbee-bee skill."
	case RoleWorker:
		return "use openbee-worker skill."
	default:
		return ""
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/ai/... -run "TestSkillHintPrefix|TestWorkerPersona" -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/rules.go internal/ai/rules_test.go
git commit -m "feat(ai): add WorkerPersona and SkillHintPrefix helpers"
```

---

## Task 2: Simplify `SetupWorkspace` — persona-only AGENTS.md, no `.openbee.md`

**Files:**
- Modify: `internal/ai/workspace.go`
- Modify: `internal/ai/workspace_test.go`
- Modify: `internal/ai/codex/adapter_test.go`
- Modify: `internal/ai/pi/adapter_test.go`

- [ ] **Step 1: Update `workspace_test.go` to reflect new behaviour**

Replace the contents of `internal/ai/workspace_test.go`:

```go
package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupWorkspace_Bee(t *testing.T) {
	dir := t.TempDir()
	if err := SetupWorkspace(dir, RoleBee, WorkspaceOptions{}); err != nil {
		t.Fatalf("SetupWorkspace: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "You are") {
		t.Errorf("AGENTS.md missing bee persona, got: %q", content)
	}
	if strings.Contains(content, LoadInstruction) {
		t.Errorf("AGENTS.md must NOT contain LoadInstruction for codex/pi engines, got: %q", content)
	}

	if _, err := os.Stat(filepath.Join(dir, SystemRulesFile)); !os.IsNotExist(err) {
		t.Errorf("%s must NOT be created for codex/pi engines", SystemRulesFile)
	}
}

func TestSetupWorkspace_Worker(t *testing.T) {
	dir := t.TempDir()
	opts := WorkspaceOptions{
		Name:        "my-worker",
		Description: "does things",
		Memory:      "remember X",
	}
	if err := SetupWorkspace(dir, RoleWorker, opts); err != nil {
		t.Fatalf("SetupWorkspace: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	content := string(data)
	if strings.Contains(content, LoadInstruction) {
		t.Errorf("AGENTS.md must NOT contain LoadInstruction, got: %q", content)
	}
	if !strings.Contains(content, "my-worker") {
		t.Errorf("AGENTS.md missing name, got: %q", content)
	}
	if !strings.Contains(content, "does things") {
		t.Errorf("AGENTS.md missing description, got: %q", content)
	}
	if !strings.Contains(content, "remember X") {
		t.Errorf("AGENTS.md missing memory, got: %q", content)
	}
	if strings.Contains(content, "openbee-worker") {
		t.Errorf("AGENTS.md must NOT contain skill rule directive, got: %q", content)
	}

	if _, err := os.Stat(filepath.Join(dir, SystemRulesFile)); !os.IsNotExist(err) {
		t.Errorf("%s must NOT be created for codex/pi engines", SystemRulesFile)
	}
}

func TestSetupWorkspace_UnknownRole(t *testing.T) {
	dir := t.TempDir()
	err := SetupWorkspace(dir, Role("unknown"), WorkspaceOptions{})
	if err == nil {
		t.Error("expected error for unknown role, got nil")
	}
}

func TestSetupWorkspace_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := SetupWorkspace(dir, RoleBee, WorkspaceOptions{}); err != nil {
		t.Fatalf("first SetupWorkspace: %v", err)
	}
	agentsmd := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(agentsmd, []byte("custom content"), 0o644); err != nil {
		t.Fatalf("write custom content: %v", err)
	}
	if err := SetupWorkspace(dir, RoleBee, WorkspaceOptions{}); err != nil {
		t.Fatalf("second SetupWorkspace: %v", err)
	}
	data, _ := os.ReadFile(agentsmd)
	if string(data) != "custom content" {
		t.Errorf("SetupWorkspace overwrote existing AGENTS.md")
	}
}
```

- [ ] **Step 2: Run to verify tests fail**

```bash
go test ./internal/ai/... -run "TestSetupWorkspace" -v
```

Expected: FAIL — `.openbee.md` is still being created and LoadInstruction is still in AGENTS.md.

- [ ] **Step 3: Update `workspace.go` implementation**

Replace `internal/ai/workspace.go`:

```go
package ai

import (
	"fmt"
	"os"
	"path/filepath"
)

// SetupWorkspace initialises the AI engine workspace in workDir by writing the
// AGENTS.md persona file. No system rules file (.openbee.md) is written;
// rule injection is handled via the skill hint prefix on new sessions.
// This is shared by all CLI engines that use the AGENTS.md convention (e.g. Codex, Pi).
func SetupWorkspace(workDir string, role Role, opts WorkspaceOptions) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("mkdir workdir: %w", err)
	}
	switch role {
	case RoleBee:
		return createAgentsMD(workDir, BeePersona+"\n")
	case RoleWorker:
		return createAgentsMD(workDir, WorkerPersona(opts.Name, opts.Description, opts.Memory))
	default:
		return fmt.Errorf("unknown role: %q", role)
	}
}

func createAgentsMD(workDir, content string) error {
	if err := CreateFileOnce(filepath.Join(workDir, "AGENTS.md"), content); err != nil {
		return fmt.Errorf("create AGENTS.md: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/ai/... -run "TestSetupWorkspace" -v
```

Expected: PASS.

- [ ] **Step 5: Update `codex/adapter_test.go` — remove `.openbee.md` checks**

Replace the `.openbee.md` existence assertions in `internal/ai/codex/adapter_test.go`:

```go
package codex

import (
	"os"
	"path/filepath"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestAdapter_SetupWorkspace_Bee(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter("codex", "http://localhost:8080")
	if err := a.SetupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
		t.Fatalf("SetupWorkspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Errorf("AGENTS.md not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".openbee.md")); !os.IsNotExist(err) {
		t.Errorf(".openbee.md must NOT be created by codex engine")
	}
}

func TestAdapter_SetupWorkspace_Worker(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter("codex", "http://localhost:8080")
	opts := ai.WorkspaceOptions{Name: "w1", Description: "desc", Memory: "mem"}
	if err := a.SetupWorkspace(dir, ai.RoleWorker, opts); err != nil {
		t.Fatalf("SetupWorkspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Errorf("AGENTS.md not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".openbee.md")); !os.IsNotExist(err) {
		t.Errorf(".openbee.md must NOT be created by codex engine")
	}
}

func TestAdapter_SetupWorkspace_UnknownRole(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter("codex", "http://localhost:8080")
	err := a.SetupWorkspace(dir, ai.Role("unknown"), ai.WorkspaceOptions{})
	if err == nil {
		t.Error("expected error for unknown role, got nil")
	}
}
```

- [ ] **Step 6: Update `pi/adapter_test.go` — remove `.openbee.md` checks**

Replace `internal/ai/pi/adapter_test.go`:

```go
package pi

import (
	"os"
	"path/filepath"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestAdapter_SetupWorkspace_Bee(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter("pi", "http://localhost:8080", nil)
	if err := a.SetupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
		t.Fatalf("SetupWorkspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Errorf("AGENTS.md not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".openbee.md")); !os.IsNotExist(err) {
		t.Errorf(".openbee.md must NOT be created by pi engine")
	}
}

func TestAdapter_SetupWorkspace_Worker(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter("pi", "http://localhost:8080", nil)
	opts := ai.WorkspaceOptions{Name: "w1", Description: "desc", Memory: "mem"}
	if err := a.SetupWorkspace(dir, ai.RoleWorker, opts); err != nil {
		t.Fatalf("SetupWorkspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Errorf("AGENTS.md not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".openbee.md")); !os.IsNotExist(err) {
		t.Errorf(".openbee.md must NOT be created by pi engine")
	}
}

func TestAdapter_SetupWorkspace_UnknownRole(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter("pi", "http://localhost:8080", nil)
	err := a.SetupWorkspace(dir, ai.Role("unknown"), ai.WorkspaceOptions{})
	if err == nil {
		t.Error("expected error for unknown role, got nil")
	}
}

func TestAdapter_SetupWorkspace_Idempotent(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter("pi", "http://localhost:8080", nil)
	if err := a.SetupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := a.SetupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
		t.Fatalf("second call: %v", err)
	}
}
```

- [ ] **Step 7: Run all ai package tests**

```bash
go test ./internal/ai/... -v
```

Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/ai/workspace.go internal/ai/workspace_test.go \
        internal/ai/codex/adapter_test.go internal/ai/pi/adapter_test.go
git commit -m "feat(ai): remove .openbee.md from codex/pi workspace setup"
```

---

## Task 3: Add skill hint to bee's `buildPrompt`

**Files:**
- Modify: `internal/domain/bee/feeder.go`
- Modify: `internal/domain/bee/feeder_internal_test.go`

- [ ] **Step 1: Update tests for `buildPrompt`**

Replace `internal/domain/bee/feeder_internal_test.go`:

```go
package bee

import (
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/store"
)

func TestBuildPrompt_NoHint(t *testing.T) {
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "feishu", SessionKey: "feishu:oc_abc:ou_xyz", Content: "hello world"},
	}
	got := buildPrompt(msgs, "")
	wantMeta := `<message_meta>{"from":"feishu","session_key":"feishu:oc_abc:ou_xyz","message_id":"msg-1"}</message_meta>`
	if !strings.HasPrefix(got, wantMeta) {
		t.Errorf("missing message_meta prefix\ngot:  %q", got)
	}
	if !strings.Contains(got, "<message_content>") {
		t.Errorf("missing message_content tag, got: %q", got)
	}
	if !strings.Contains(got, "</message_content>") {
		t.Errorf("missing closing message_content tag, got: %q", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Errorf("missing original content, got: %q", got)
	}
}

func TestBuildPrompt_WithHint(t *testing.T) {
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "feishu", SessionKey: "sk1", Content: "hi"},
	}
	got := buildPrompt(msgs, "use openbee-bee skill.")
	if !strings.HasPrefix(got, "use openbee-bee skill.\n") {
		t.Errorf("skill hint must be first line\ngot: %q", got)
	}
	if !strings.Contains(got, "<message_meta>") {
		t.Errorf("missing message_meta, got: %q", got)
	}
	if !strings.Contains(got, "hi") {
		t.Errorf("missing message content, got: %q", got)
	}
}

func TestBuildPrompt_MultipleMessages(t *testing.T) {
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "feishu", SessionKey: "sk1", Content: "first"},
		{ID: "msg-2", Platform: "feishu", SessionKey: "sk1", Content: "second"},
	}
	got := buildPrompt(msgs, "")
	if !strings.Contains(got, "msg-1") || !strings.Contains(got, "msg-2") {
		t.Errorf("missing message IDs, got: %q", got)
	}
	if strings.Count(got, "<message_meta>") != 2 {
		t.Errorf("expected 2 message_meta blocks, got: %q", got)
	}
}

func TestBuildPrompt_MultipleMessages_WithHint(t *testing.T) {
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "feishu", SessionKey: "sk1", Content: "first"},
		{ID: "msg-2", Platform: "feishu", SessionKey: "sk1", Content: "second"},
	}
	got := buildPrompt(msgs, "use openbee-bee skill.")
	if !strings.HasPrefix(got, "use openbee-bee skill.\n") {
		t.Errorf("skill hint must be first line\ngot: %q", got)
	}
	if strings.Count(got, "<message_meta>") != 2 {
		t.Errorf("expected 2 message_meta blocks, got: %q", got)
	}
}
```

- [ ] **Step 2: Run to verify tests fail**

```bash
go test ./internal/domain/bee/... -run "TestBuildPrompt" -v
```

Expected: FAIL — `buildPrompt` does not accept a second argument.

- [ ] **Step 3: Update `buildPrompt` in `feeder.go`**

Find `buildPrompt` in `internal/domain/bee/feeder.go` (currently at line 313) and replace it:

```go
func buildPrompt(msgs []store.ClaimedMessage, skillHint string) string {
	var sb strings.Builder
	sb.Grow(len(msgs) * 128)
	if skillHint != "" {
		sb.WriteString(skillHint)
		sb.WriteByte('\n')
	}
	for i, m := range msgs {
		if i > 0 {
			sb.WriteByte('\n')
		}
		b, _ := json.Marshal(messageMeta{From: m.Platform, SessionKey: m.SessionKey, MessageID: m.ID})
		fmt.Fprintf(&sb, "<message_meta>%s</message_meta>\n<message_content>\n%s\n</message_content>\n", b, m.Content)
	}
	return sb.String()
}
```

Also update the call site in `processBeeGroup` (currently at line 175):

```go
// replace:
//   prompt := buildPrompt(msgs)
// with:
hint := ""
if !resume {
    hint = ai.SkillHintPrefix(ai.RoleBee)
}
prompt := buildPrompt(msgs, hint)
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/domain/bee/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/bee/feeder.go internal/domain/bee/feeder_internal_test.go
git commit -m "feat(bee): prepend skill hint on new session in buildPrompt"
```

---

## Task 4: Inject skill hint in worker's `resolveExecution`

**Files:**
- Modify: `internal/domain/task/dispatcher.go`
- Modify: `internal/domain/task/dispatcher_test.go`

- [ ] **Step 1: Write failing tests for skill hint behaviour**

Add to the end of `internal/domain/task/dispatcher_test.go`:

```go
func TestTaskDispatcher_NewSession_HasSkillHint(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "sess-1"},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}
	// No prior session context — new session
	d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	task := immediateTask("sk-1", "worker-1", "do the thing")
	in <- task

	if !waitForExecCount(mgr, 1, 3*time.Second) {
		t.Fatal("timeout waiting for execution")
	}
	mgr.mu.Lock()
	instruction := mgr.executedInstructions[0]
	mgr.mu.Unlock()
	if !strings.HasPrefix(instruction, "use openbee-worker skill.\n") {
		t.Errorf("new session must start with skill hint\ngot: %q", instruction)
	}
}

func TestTaskDispatcher_ResumeSession_NoSkillHint(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "sess-1"},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}
	ss := newMockSessionStore()
	// Pre-populate session context so this is a resume.
	// Engine name must match the dispatcher's WithEngine option so
	// GetSessionContextForEngine returns the stored session ID.
	_ = ss.UpsertSessionContext(context.Background(), "sk-1", "worker-1", "existing-sess", "testengine")
	d, in, _ := newTaskDispatcher(mgr, eq, ss, task.WithEngine("testengine"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	task := immediateTask("sk-1", "worker-1", "do the thing")
	in <- task

	if !waitForExecCount(mgr, 1, 3*time.Second) {
		t.Fatal("timeout waiting for execution")
	}
	mgr.mu.Lock()
	instruction := mgr.executedInstructions[0]
	mgr.mu.Unlock()
	if strings.HasPrefix(instruction, "use openbee-worker skill.") {
		t.Errorf("resume session must NOT have skill hint\ngot: %q", instruction)
	}
}
```

- [ ] **Step 2: Run to verify tests fail**

```bash
go test ./internal/domain/task/... -run "TestTaskDispatcher_NewSession_HasSkillHint|TestTaskDispatcher_ResumeSession_NoSkillHint" -v
```

Expected: FAIL — no skill hint currently added.

- [ ] **Step 3: Update `resolveExecution` in `dispatcher.go`**

Find `resolveExecution` (currently at line 291) and replace:

```go
func (d *TaskDispatcher) resolveExecution(ctx context.Context, task DispatchTask, instruction string) (model.WorkerExecution, error) {
	hint := ai.SkillHintPrefix(ai.RoleWorker)

	if task.TaskType != model.TaskTypeImmediate {
		log.Info("executing worker", zap.String("workerID", task.WorkerID), zap.String("taskID", task.TaskID))
		return d.manager.ExecuteWorker(ctx, task.WorkerID, hint+"\n"+instruction, "")
	}
	sessionID, err := d.sessionStore.GetSessionContextForEngine(ctx, task.SessionKey, task.WorkerID, d.engineName)
	if err != nil {
		log.Error("get session context", zap.Error(err))
	}
	if sessionID == "" {
		log.Info("executing worker", zap.String("workerID", task.WorkerID), zap.String("taskID", task.TaskID))
		return d.manager.ExecuteWorker(ctx, task.WorkerID, hint+"\n"+instruction, "")
	}
	log.Info("resuming session", zap.String("sessionID", sessionID), zap.String("taskID", task.TaskID))
	exec, err := d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, sessionID)
	if err == nil {
		return exec, nil
	}
	log.Error("resume error, falling back to fresh", zap.Error(err))
	if clearErr := d.sessionStore.ClearSessionContexts(ctx, task.SessionKey); clearErr != nil {
		log.Error("clear stale session contexts", zap.String("sessionKey", task.SessionKey), zap.Error(clearErr))
	}
	return d.manager.ExecuteWorker(ctx, task.WorkerID, hint+"\n"+instruction, "")
}
```

Also add the import for `ai` package at the top of `dispatcher.go` imports:

```go
import (
	// ... existing imports ...
	ai "github.com/theopenbee/openbee/internal/ai"
)
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/domain/task/... -v
```

Expected: all PASS.

- [ ] **Step 5: Run all affected tests together**

```bash
go test ./internal/ai/... ./internal/domain/bee/... ./internal/domain/task/... -v 2>&1 | tail -20
```

Expected: all packages PASS with no failures.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/task/dispatcher.go internal/domain/task/dispatcher_test.go
git commit -m "feat(task): prepend skill hint on new/fresh worker sessions"
```
