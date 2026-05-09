# Skill Hint & Worker Persona via System Prompt — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the skill-invocation hint and the worker persona out of the user prompt and into the system prompt for engines that support it (claude, pi); for engines that don't (codex, kimi) keep current behaviour but encapsulate the prepend inside the adapter so callers stay engine-agnostic.

**Architecture:** Add a single `SystemPrompt` field to `ai.RunOptions`. Each adapter routes that string to the most authoritative channel it has (`--append-system-prompt` for claude/pi, prepend-to-user-prompt for codex/kimi). Callers (`task.dispatcher`, `bee.feeder`) build the system body once via a new `ai.BuildSystemPrompt` helper and pass it through; nothing about per-engine quirks leaks above the adapter line.

**Tech Stack:** Go, existing engine-adapter pattern in `internal/ai/*`, existing TDD style with `go test`.

---

## File Structure

| File | Role |
|---|---|
| `internal/ai/contracts.go` | Add `SystemPrompt` field to `RunOptions` |
| `internal/ai/prompt.go` | Add `BuildSystemPrompt(role, *model.Worker)` helper |
| `internal/ai/prompt_test.go` | Cover `BuildSystemPrompt` |
| `internal/ai/claude/invoker.go` | Honour `SystemPrompt` via `--append-system-prompt` |
| `internal/ai/claude/invoker_test.go` | Argv assertions |
| `internal/ai/pi/invoker.go` | Honour `SystemPrompt` via `--append-system-prompt` |
| `internal/ai/pi/invoker_test.go` | Argv assertions |
| `internal/ai/codex/invoker.go` | Prepend `SystemPrompt` to user prompt |
| `internal/ai/codex/invoker_test.go` | Stdin/argv prompt assertions |
| `internal/ai/kimi/invoker.go` | Prepend `SystemPrompt` to user prompt |
| `internal/ai/kimi/invoker_test.go` | Stdin prompt assertions |
| `internal/domain/worker/execution.go` | `ExecuteWorker`/`launchRuntime` gain `systemPrompt` arg |
| `internal/domain/worker/manager_test.go` | Update call site |
| `internal/domain/task/dispatcher.go` | `executeWithHint` builds & passes `SystemPrompt`; `ExecutionManager` interface gets the new arg |
| `internal/domain/task/dispatcher_test.go` | Mocks updated; assertions flipped |
| `internal/domain/bee/feeder.go` | `SystemPrompt` set on fresh runs; `buildPrompt(msgs, "")` drops the hint param |
| `internal/domain/bee/feeder_internal_test.go` | Drop `skillHint` arg from tests |
| `internal/domain/bee/feeder_test.go` | Assert SystemPrompt routed via `RunOptions` |

---

## Task 1: Add `SystemPrompt` field to `RunOptions`

**Files:**
- Modify: `internal/ai/contracts.go:30-36`

This task is a pure additive contract change with no behavioural impact (no adapter reads the field yet). It unblocks all later tasks.

- [ ] **Step 1: Add the field**

In `internal/ai/contracts.go`, change `RunOptions` to:

```go
// RunOptions controls session behaviour for an engine invocation.
type RunOptions struct {
	SessionID    string
	Resume       bool
	APIKey       string
	ExtraEnv     []string // additional KEY=VALUE env vars to inject
	ExtraArgs    []string // additional CLI args to pass to the engine
	SystemPrompt string   // session-level system instructions; routed by each adapter to its highest-priority channel (--append-system-prompt or user-prompt prefix). Leave empty to skip.
}
```

- [ ] **Step 2: Build the package**

Run: `go build ./internal/ai/...`
Expected: succeeds.

- [ ] **Step 3: Run the existing test suite**

Run: `go test ./internal/ai/...`
Expected: all pass — no behaviour changed, only a new field with default zero value.

- [ ] **Step 4: Commit**

```bash
git add internal/ai/contracts.go
git commit -m "ai: add SystemPrompt field to RunOptions"
```

---

## Task 2: Add `BuildSystemPrompt` helper

**Files:**
- Modify: `internal/ai/prompt.go`
- Modify: `internal/ai/prompt_test.go`

The helper centralises the rule: bee gets the hint only; worker gets `hint + <worker_persona>persona</worker_persona>`. Both `task.dispatcher` and `bee.feeder` will call this exactly once on fresh sessions.

- [ ] **Step 1: Write the failing tests**

Append to `internal/ai/prompt_test.go`:

```go
import "github.com/theopenbee/openbee/internal/infra/model"

func TestBuildSystemPrompt_Bee(t *testing.T) {
	got := BuildSystemPrompt(RoleBee, nil)
	if !strings.HasPrefix(got, SkillHintPrefix(RoleBee)) {
		t.Errorf("bee system prompt must start with skill hint, got: %q", got)
	}
	if strings.Contains(got, "<worker_persona>") {
		t.Errorf("bee system prompt must not include worker_persona, got: %q", got)
	}
}

func TestBuildSystemPrompt_Worker_WithPersona(t *testing.T) {
	w := &model.Worker{Name: "貂蝉", Description: "负责 openbee 开发", Constraints: "称呼用户老板"}
	got := BuildSystemPrompt(RoleWorker, w)
	if !strings.HasPrefix(got, SkillHintPrefix(RoleWorker)) {
		t.Errorf("worker system prompt must start with skill hint, got: %q", got)
	}
	if !strings.Contains(got, "<worker_persona>") || !strings.Contains(got, "</worker_persona>") {
		t.Errorf("worker system prompt must wrap persona in <worker_persona> tags, got: %q", got)
	}
	for _, want := range []string{"Name: 貂蝉", "Description: 负责 openbee 开发", "称呼用户老板"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in system prompt, got: %q", want, got)
		}
	}
}

func TestBuildSystemPrompt_Worker_NilWorker(t *testing.T) {
	got := BuildSystemPrompt(RoleWorker, nil)
	if got != SkillHintPrefix(RoleWorker) {
		t.Errorf("nil worker should yield only the skill hint, got: %q", got)
	}
}

func TestBuildSystemPrompt_UnknownRole(t *testing.T) {
	got := BuildSystemPrompt(Role("other"), nil)
	if got != "" {
		t.Errorf("unknown role must return empty string, got: %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ai/ -run TestBuildSystemPrompt`
Expected: compile error — `BuildSystemPrompt` undefined.

- [ ] **Step 3: Implement `BuildSystemPrompt`**

Append to `internal/ai/prompt.go`:

```go
import "github.com/theopenbee/openbee/internal/infra/model"

// BuildSystemPrompt returns the full session-level system instructions for
// the given role: the skill-invocation hint, plus (for workers with a
// resolved record) the persona block wrapped in <worker_persona> tags.
// Returns "" for unknown roles.
func BuildSystemPrompt(role Role, w *model.Worker) string {
	hint := SkillHintPrefix(role)
	if hint == "" {
		return ""
	}
	if role == RoleWorker && w != nil {
		persona := WorkerPersona(w.Name, w.Description, w.Constraints)
		return hint + "\n<worker_persona>\n" + persona + "</worker_persona>"
	}
	return hint
}
```

Note: the existing `import "fmt"` in `prompt.go` stays. Add `model` to the import block.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ai/ -run TestBuildSystemPrompt -v`
Expected: all four sub-tests PASS.

- [ ] **Step 5: Run the full ai package tests**

Run: `go test ./internal/ai/...`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/prompt.go internal/ai/prompt_test.go
git commit -m "ai: add BuildSystemPrompt helper"
```

---

## Task 3: Wire `SystemPrompt` into the claude adapter

**Files:**
- Modify: `internal/ai/claude/invoker.go:85-99`
- Modify: `internal/ai/claude/invoker_test.go`

Claude supports `--append-system-prompt <text>` natively.

- [ ] **Step 1: Write the failing test**

Append to `internal/ai/claude/invoker_test.go`. (`buildArgs` is not factored out in claude's invoker; the test inspects the spawned process's args by feeding `echo` and asserting the log content.) Use a different strategy: extract `buildArgs` first.

Add a new helper at the top of `internal/ai/claude/invoker.go` (between `streamContent` and `scanResultLog`):

```go
func buildArgs(opts ai.RunOptions) []string {
	args := []string{
		"--dangerously-skip-permissions",
		"--verbose",
		"--output-format", "stream-json",
	}
	if opts.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", opts.SystemPrompt)
	}
	if opts.SessionID != "" {
		if opts.Resume {
			args = append(args, "--resume", opts.SessionID)
		} else {
			args = append(args, "--session-id", opts.SessionID)
		}
	}
	args = append(args, opts.ExtraArgs...)
	args = append(args, "--print")
	return args
}
```

Then update `Run` to call it:

```go
args := buildArgs(opts)
```

(Replace the existing inline `args := []string{…}` … `args = append(args, "--print")` block.)

Now in `internal/ai/claude/invoker_test.go`, add:

```go
func TestBuildArgs_NoSystemPrompt(t *testing.T) {
	args := buildArgs(ai.RunOptions{SessionID: "s1"})
	for _, a := range args {
		if a == "--append-system-prompt" {
			t.Errorf("unexpected --append-system-prompt in args: %v", args)
		}
	}
	if !slices.Contains(args, "--session-id") || !slices.Contains(args, "s1") {
		t.Errorf("session args missing: %v", args)
	}
}

func TestBuildArgs_WithSystemPrompt(t *testing.T) {
	args := buildArgs(ai.RunOptions{SessionID: "s1", SystemPrompt: "be terse"})
	idx := slices.Index(args, "--append-system-prompt")
	if idx < 0 || idx == len(args)-1 {
		t.Fatalf("expected --append-system-prompt with value, got %v", args)
	}
	if args[idx+1] != "be terse" {
		t.Errorf("expected value %q, got %q", "be terse", args[idx+1])
	}
	// Must still come before --print so engine args (ExtraArgs) keep their relative order.
	printIdx := slices.Index(args, "--print")
	if printIdx < 0 || idx >= printIdx {
		t.Errorf("--append-system-prompt must precede --print, got %v", args)
	}
}

func TestBuildArgs_WithSystemPromptAndResume(t *testing.T) {
	args := buildArgs(ai.RunOptions{SessionID: "s1", Resume: true, SystemPrompt: "x"})
	if !slices.Contains(args, "--resume") {
		t.Errorf("expected --resume in args: %v", args)
	}
	if !slices.Contains(args, "--append-system-prompt") {
		t.Errorf("expected --append-system-prompt even on resume (caller controls; adapter is stateless): %v", args)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ai/claude/ -run TestBuildArgs -v`
Expected: compile error — `buildArgs` undefined (we haven't extracted it yet).

- [ ] **Step 3: Extract `buildArgs` and wire `SystemPrompt`**

Apply the `buildArgs` extraction shown in Step 1 to `internal/ai/claude/invoker.go`. The relevant lines in `Run` change from the current inline block to just `args := buildArgs(opts)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ai/claude/ -run TestBuildArgs -v`
Expected: all three pass.

- [ ] **Step 5: Run the full claude package tests**

Run: `go test ./internal/ai/claude/...`
Expected: all pass — the existing `TestInvoker_Run_*` tests rely on observed argv behaviour through `echo`; argv ordering is preserved.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/claude/invoker.go internal/ai/claude/invoker_test.go
git commit -m "ai/claude: route SystemPrompt to --append-system-prompt"
```

---

## Task 4: Wire `SystemPrompt` into the pi adapter

**Files:**
- Modify: `internal/ai/pi/invoker.go:59-62`
- Modify: `internal/ai/pi/invoker_test.go`

Pi also supports `--append-system-prompt <text>`.

- [ ] **Step 1: Write the failing test**

Append to `internal/ai/pi/invoker_test.go`:

```go
import "slices"

func TestBuildArgs_NoSystemPrompt(t *testing.T) {
	args := buildArgs("hi", "/tmp/sess.jsonl", "", nil)
	for _, a := range args {
		if a == "--append-system-prompt" {
			t.Errorf("unexpected --append-system-prompt in args: %v", args)
		}
	}
}

func TestBuildArgs_WithSystemPrompt(t *testing.T) {
	args := buildArgs("hi", "/tmp/sess.jsonl", "be terse", nil)
	idx := slices.Index(args, "--append-system-prompt")
	if idx < 0 || idx == len(args)-1 {
		t.Fatalf("expected --append-system-prompt with value, got %v", args)
	}
	if args[idx+1] != "be terse" {
		t.Errorf("expected value %q, got %q", "be terse", args[idx+1])
	}
}
```

If `internal/ai/pi/invoker_test.go` already has tests that call `buildArgs(prompt, sessionPath, extraArgs)` with three args, the signature change will compile-fail those callers — update them to pass `""` for `systemPrompt`. Run `go test ./internal/ai/pi/... -count=1` after Step 3 to find any remaining breaks.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ai/pi/ -run TestBuildArgs_.*SystemPrompt -v`
Expected: compile error — `buildArgs` does not accept a `systemPrompt` parameter.

- [ ] **Step 3: Update `buildArgs` and `Run`**

In `internal/ai/pi/invoker.go`, change `buildArgs`:

```go
func buildArgs(prompt, sessionPath, systemPrompt string, extraArgs []string) []string {
	base := []string{"--mode", "json", "--session", sessionPath, "-p", prompt}
	if systemPrompt != "" {
		base = append(base, "--append-system-prompt", systemPrompt)
	}
	return append(base, extraArgs...)
}
```

In `Run`, change the call site (currently `args := buildArgs(prompt, sessionPath, opts.ExtraArgs)`):

```go
args := buildArgs(prompt, sessionPath, opts.SystemPrompt, opts.ExtraArgs)
```

- [ ] **Step 4: Update any other existing callers of `buildArgs`**

Run: `go test ./internal/ai/pi/... -count=1`
Expected: any pre-existing test that calls `buildArgs` with three args fails to compile. For each one, add `""` as the third positional argument (just before `extraArgs`). Re-run.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/ai/pi/...`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/pi/invoker.go internal/ai/pi/invoker_test.go
git commit -m "ai/pi: route SystemPrompt to --append-system-prompt"
```

---

## Task 5: Wire `SystemPrompt` into the codex adapter

**Files:**
- Modify: `internal/ai/codex/invoker.go:106-127`
- Modify: `internal/ai/codex/invoker_test.go`

Codex has no `--append-system-prompt`. Adapter prepends `SystemPrompt + "\n\n"` to the user prompt. Callers only set `SystemPrompt` on fresh sessions, so we can safely prepend whenever the field is non-empty.

- [ ] **Step 1: Write the failing test**

Append to `internal/ai/codex/invoker_test.go`:

```go
func TestApplySystemPrompt_Empty(t *testing.T) {
	got := applySystemPrompt("hello", "")
	if got != "hello" {
		t.Errorf("empty system prompt must not modify user prompt, got: %q", got)
	}
}

func TestApplySystemPrompt_Prepends(t *testing.T) {
	got := applySystemPrompt("hello", "be terse")
	want := "be terse\n\nhello"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ai/codex/ -run TestApplySystemPrompt -v`
Expected: compile error — `applySystemPrompt` undefined.

- [ ] **Step 3: Implement `applySystemPrompt` and use it in `Run`**

In `internal/ai/codex/invoker.go`, add (anywhere above `Run`):

```go
// applySystemPrompt prepends the system-level instructions onto the user
// prompt when the engine has no native system-prompt channel. Caller is
// responsible for only setting systemPrompt on fresh sessions.
func applySystemPrompt(userPrompt, systemPrompt string) string {
	if systemPrompt == "" {
		return userPrompt
	}
	return systemPrompt + "\n\n" + userPrompt
}
```

In `Run`, before computing args/stdin, transform the prompt:

```go
prompt = applySystemPrompt(prompt, opts.SystemPrompt)
threadID, resume := inv.resolveThread(opts.SessionID, opts.Resume)
args := buildArgs(threadID, resume, prompt, opts.ExtraArgs)
```

(The existing line `cmd.Stdin = strings.NewReader(prompt)` keeps the prepended content; `buildArgs` already places `prompt` in the right place when resuming.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ai/codex/...`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/codex/invoker.go internal/ai/codex/invoker_test.go
git commit -m "ai/codex: prepend SystemPrompt to user prompt"
```

---

## Task 6: Wire `SystemPrompt` into the kimi adapter

**Files:**
- Modify: `internal/ai/kimi/invoker.go:135-156`
- Modify: `internal/ai/kimi/invoker_test.go`

Kimi has no `--append-system-prompt`. Mirror the codex pattern: prepend the system body to stdin.

- [ ] **Step 1: Write the failing test**

Append to `internal/ai/kimi/invoker_test.go`:

```go
func TestApplySystemPrompt_Empty(t *testing.T) {
	got := applySystemPrompt("hello", "")
	if got != "hello" {
		t.Errorf("empty system prompt must not modify user prompt, got: %q", got)
	}
}

func TestApplySystemPrompt_Prepends(t *testing.T) {
	got := applySystemPrompt("hello", "be terse")
	want := "be terse\n\nhello"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ai/kimi/ -run TestApplySystemPrompt -v`
Expected: compile error — `applySystemPrompt` undefined.

- [ ] **Step 3: Implement and use `applySystemPrompt`**

In `internal/ai/kimi/invoker.go`, add:

```go
// applySystemPrompt prepends the system-level instructions onto the user
// prompt. Mirrors codex; kimi has no --append-system-prompt flag.
func applySystemPrompt(userPrompt, systemPrompt string) string {
	if systemPrompt == "" {
		return userPrompt
	}
	return systemPrompt + "\n\n" + userPrompt
}
```

In `Run`, modify the body (around line 145-147):

```go
cmd := exec.CommandContext(ctx, inv.binary, args...)
cmd.Dir = workDir
cmd.Stdin = strings.NewReader(applySystemPrompt(prompt, opts.SystemPrompt))
cmd.Stdout = logFile
cmd.Stderr = logFile
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ai/kimi/...`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/kimi/invoker.go internal/ai/kimi/invoker_test.go
git commit -m "ai/kimi: prepend SystemPrompt to user prompt"
```

---

## Task 7: Plumb `systemPrompt` through `worker.Manager`

**Files:**
- Modify: `internal/domain/worker/execution.go:18,40,49,76-82`
- Modify: `internal/domain/worker/manager_test.go:195`

The manager and the `task.ExecutionManager` interface both need a new `systemPrompt string` parameter on `ExecuteWorker`. We update the signature and propagate it into `RunOptions.SystemPrompt`. Callers that pass `""` get the same behaviour as today.

- [ ] **Step 1: Update `Manager.ExecuteWorker` and `launchRuntime`**

In `internal/domain/worker/execution.go`:

```go
// ExecuteWorker runs a worker. When resume is true, the AI engine will attempt
// to resume the session identified by sessionID; otherwise it starts a fresh session.
// sessionID must always be non-empty; callers are responsible for generating it.
// systemPrompt is the session-level system instructions (skill hint + persona);
// it is only meaningful for fresh sessions and should be "" on resume.
func (m *Manager) ExecuteWorker(ctx context.Context, workerID, triggerInput, sessionID string, resume bool, systemPrompt string) (model.WorkerExecution, error) {
	// ... unchanged through the if-checks ...
	if err := m.launchRuntime(ctx, exec, worker, engine, engineName, timeout, triggerInput, resume, systemPrompt); err != nil {
		// ...
	}
	return exec, nil
}

func (m *Manager) launchRuntime(ctx context.Context, exec model.WorkerExecution, worker model.Worker, engine ai.EngineAdapter, engineName string, timeout time.Duration, prompt string, resume bool, systemPrompt string) error {
	// ...
	runRes, err := engine.Run(execCtx, worker.WorkDir, prompt, ai.RunOptions{
		SessionID:    exec.SessionID,
		Resume:       resume,
		APIKey:       token,
		ExtraEnv:     extraEnv,
		ExtraArgs:    extraArgs,
		SystemPrompt: systemPrompt,
	}, logPath)
	// ...
}
```

- [ ] **Step 2: Update the manager test call site**

In `internal/domain/worker/manager_test.go:195`, change:

```go
exec, err := mgr.ExecuteWorker(context.Background(), w.ID, "test", "session-1", false)
```

to:

```go
exec, err := mgr.ExecuteWorker(context.Background(), w.ID, "test", "session-1", false, "")
```

- [ ] **Step 3: Run worker tests**

Run: `go test ./internal/domain/worker/...`
Expected: all pass.

- [ ] **Step 4: Sanity build the whole module**

Run: `go build ./...`
Expected: this will fail in `internal/domain/task/` because `mockExecManager.ExecuteWorker` no longer satisfies the (about-to-change) interface. That's the next task. Note the failure for crosscheck.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/worker/execution.go internal/domain/worker/manager_test.go
git commit -m "worker: thread systemPrompt through ExecuteWorker into RunOptions"
```

---

## Task 8: Update `task.ExecutionManager` interface and `dispatcher.executeWithHint`

**Files:**
- Modify: `internal/domain/task/dispatcher.go:33-34,336-348`
- Modify: `internal/domain/task/dispatcher_test.go:29-31,66,261,733,746,758,887,1191,1221,1257,1298`

The dispatcher stops prepending hint+persona to the user message and instead passes them via the new `systemPrompt` argument on the fresh-session path only.

- [ ] **Step 1: Update the `ExecutionManager` interface**

In `internal/domain/task/dispatcher.go`:

```go
type ExecutionManager interface {
	ExecuteWorker(ctx context.Context, workerID, input, sessionID string, resume bool, systemPrompt string) (model.WorkerExecution, error)
	CancelExecution(ctx context.Context, executionID string) error
}
```

- [ ] **Step 2: Rewrite `executeWithHint`**

Replace the body of `executeWithHint` (currently lines 336-349) with:

```go
func (d *TaskDispatcher) executeWithHint(ctx context.Context, task DispatchTask, instruction, engineName string, worker *model.Worker) (model.WorkerExecution, error) {
	if d.workerLookup != nil && worker == nil {
		return model.WorkerExecution{}, fmt.Errorf("worker %q not found", task.WorkerID)
	}
	systemPrompt := ai.BuildSystemPrompt(ai.RoleWorker, worker)
	sessionID := uuid.New().String()
	d.upsertSessionContext(ctx, task, sessionID, engineName)
	log.Info("executing worker", zap.String("workerID", task.WorkerID), zap.String("taskID", task.TaskID))
	return d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, sessionID, false, systemPrompt)
}
```

Note: `BuildSystemPrompt` returns `""` when `worker == nil`, so when `workerLookup` is not configured we still emit only the hint, consistent with current behaviour. Do NOT special-case the `workerLookup == nil` branch — that's why we made `BuildSystemPrompt` accept `nil`.

- [ ] **Step 3: Update the resume call in `resolveExecution`**

In `internal/domain/task/dispatcher.go` around line 364, change:

```go
exec, err := d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, sessionID, true)
```

to:

```go
exec, err := d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, sessionID, true, "")
```

- [ ] **Step 4: Update mock signatures in `dispatcher_test.go`**

Find every `func (m *…) ExecuteWorker(…)` and add the trailing `_ string` (or named) parameter. Locations and their replacement signatures:

| Line | New signature |
|---|---|
| 29 | `func (m *mockExecManager) ExecuteWorker(_ context.Context, _, instruction, sessionID string, resume bool, systemPrompt string) (model.WorkerExecution, error) {` |
| 66 | `func (m *quickCancelExecManager) ExecuteWorker(_ context.Context, _, _, _ string, _ bool, _ string) (model.WorkerExecution, error) {` |
| 261 | `func (m *orderedMockManager) ExecuteWorker(_ context.Context, _, _, sessionID string, resume bool, _ string) (model.WorkerExecution, error) {` |
| 733 | `func (m *blockingExecManager) ExecuteWorker(_ context.Context, _, _, _ string, _ bool, _ string) (model.WorkerExecution, error) {` |
| 746 | `func (m *alwaysFailExecManager) ExecuteWorker(_ context.Context, _, _, _ string, _ bool, _ string) (model.WorkerExecution, error) {` |
| 758 | `func (m *fallbackExecManager) ExecuteWorker(_ context.Context, _, _, _ string, resume bool, _ string) (model.WorkerExecution, error) {` |
| 887 | `func (m *cancelTrackingExecManager) ExecuteWorker(ctx context.Context, _, _, _ string, _ bool, _ string) (model.WorkerExecution, error) {` |

For `mockExecManager` only, also capture the new field so tests can assert on it. The struct and its `ExecuteWorker` (currently lines 22-37) become:

```go
type mockExecManager struct {
	mu                    sync.Mutex
	execResult            model.WorkerExecution
	resumedWithSessionID  string
	executedInstructions  []string
	executedSystemPrompts []string
}

func (m *mockExecManager) ExecuteWorker(_ context.Context, _, instruction, sessionID string, resume bool, systemPrompt string) (model.WorkerExecution, error) {
	m.mu.Lock()
	if resume {
		m.resumedWithSessionID = sessionID
	}
	m.executedInstructions = append(m.executedInstructions, instruction)
	m.executedSystemPrompts = append(m.executedSystemPrompts, systemPrompt)
	m.mu.Unlock()
	return m.execResult, nil
}
```

- [ ] **Step 5: Flip the assertions in fresh-vs-resume tests**

In `internal/domain/task/dispatcher_test.go`, the four hint-assertion sites need to flip. The hint is no longer in `instruction`; it lives in `systemPrompt`. Update:

**Around line 1191** (`TestTaskDispatcher_NewSession_HasSkillHint` or similar):

```go
mgr.mu.Lock()
instruction := mgr.executedInstructions[0]
systemPrompt := mgr.executedSystemPrompts[0]
mgr.mu.Unlock()
if strings.HasPrefix(instruction, ai.SkillHintPrefix(ai.RoleWorker)) {
	t.Errorf("instruction must NOT start with skill hint (it now lives in systemPrompt)\ngot: %q", instruction)
}
if !strings.HasPrefix(systemPrompt, ai.SkillHintPrefix(ai.RoleWorker)) {
	t.Errorf("new session systemPrompt must start with skill hint\ngot: %q", systemPrompt)
}
```

**Around line 1221** (`TestTaskDispatcher_ResumeSession_NoSkillHint`):

```go
mgr.mu.Lock()
instruction := mgr.executedInstructions[0]
systemPrompt := mgr.executedSystemPrompts[0]
mgr.mu.Unlock()
if strings.HasPrefix(instruction, ai.SkillHintPrefix(ai.RoleWorker)) {
	t.Errorf("resume session must NOT have skill hint in instruction\ngot: %q", instruction)
}
if systemPrompt != "" {
	t.Errorf("resume session must have empty systemPrompt\ngot: %q", systemPrompt)
}
```

**Around line 1257** (`TestTaskDispatcher_NewSession_InjectsWorkerPersona`):

```go
mgr.mu.Lock()
instr := mgr.executedInstructions[0]
sysPrompt := mgr.executedSystemPrompts[0]
mgr.mu.Unlock()

if strings.HasPrefix(instr, ai.SkillHintPrefix(ai.RoleWorker)) {
	t.Errorf("instruction must NOT carry the skill hint anymore, got: %q", instr)
}
if !strings.HasPrefix(sysPrompt, ai.SkillHintPrefix(ai.RoleWorker)) {
	t.Errorf("systemPrompt missing skill hint, got: %q", sysPrompt)
}
if !strings.Contains(sysPrompt, "<worker_persona>") {
	t.Errorf("systemPrompt missing <worker_persona> tag, got: %q", sysPrompt)
}
if !strings.Contains(sysPrompt, "Name: 毛毛") {
	t.Errorf("systemPrompt missing worker name, got: %q", sysPrompt)
}
if !strings.Contains(sysPrompt, "Description: 负责 openbee 开发") {
	t.Errorf("systemPrompt missing worker description, got: %q", sysPrompt)
}
if !strings.Contains(sysPrompt, "记住老板的偏好") {
	t.Errorf("systemPrompt missing worker constraints, got: %q", sysPrompt)
}
if !strings.Contains(sysPrompt, "</worker_persona>") {
	t.Errorf("systemPrompt missing </worker_persona> tag, got: %q", sysPrompt)
}
```

**Around line 1298** (`TestTaskDispatcher_NewSession_NilLookup_OnlySkillHint`):

With `workerLookup == nil`, the dispatcher passes `worker == nil` to `BuildSystemPrompt`. Per Task 2's contract that returns just `SkillHintPrefix(RoleWorker)` (no persona), preserving today's behaviour where the hint is injected but persona is omitted. The assertion therefore is "hint present, no persona":

```go
mgr.mu.Lock()
instr := mgr.executedInstructions[0]
sysPrompt := mgr.executedSystemPrompts[0]
mgr.mu.Unlock()

if strings.HasPrefix(instr, ai.SkillHintPrefix(ai.RoleWorker)) {
	t.Errorf("instruction must NOT carry the skill hint anymore, got: %q", instr)
}
if !strings.HasPrefix(sysPrompt, ai.SkillHintPrefix(ai.RoleWorker)) {
	t.Errorf("nil-lookup path must still inject the skill hint via systemPrompt, got: %q", sysPrompt)
}
if strings.Contains(sysPrompt, "<worker_persona>") {
	t.Errorf("nil-lookup path must not include <worker_persona>, got: %q", sysPrompt)
}
```

- [ ] **Step 6: Run the dispatcher tests**

Run: `go test ./internal/domain/task/... -count=1`
Expected: all pass.

- [ ] **Step 7: Sanity-build the whole module**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 8: Run the whole test suite**

Run: `go test ./...`
Expected: bee tests still pass (their behaviour did not change — they still use the legacy `buildPrompt(msgs, hint)` path, which Task 9 will rewrite). All other packages pass.

- [ ] **Step 9: Commit**

```bash
git add internal/domain/task/dispatcher.go internal/domain/task/dispatcher_test.go
git commit -m "task: pass skill hint and persona via SystemPrompt instead of user prompt"
```

---

## Task 9: Migrate the bee feeder to `SystemPrompt`

**Files:**
- Modify: `internal/domain/bee/feeder.go:203-237,338-358`
- Modify: `internal/domain/bee/feeder_internal_test.go`
- Modify: `internal/domain/bee/feeder_test.go` (only if it asserts on prompt content; otherwise no change needed)

The bee path is parallel to the worker path. Drop the `skillHint` argument from `buildPrompt` (no caller will need it after this task) and route the hint through `RunOptions.SystemPrompt`.

- [ ] **Step 1: Rewrite the relevant block in `feeder.go`**

Replace lines 203-207 (the current `hint := ""; if !resume { hint = ai.SkillHintPrefix(ai.RoleBee) }; prompt := buildPrompt(msgs, hint)`) with:

```go
systemPrompt := ""
if !resume {
	systemPrompt = ai.BuildSystemPrompt(ai.RoleBee, nil)
}
prompt := buildPrompt(msgs)
```

Replace the `runner.Run` call (currently around line 237):

```go
runRes, err := f.runner.Run(beeCtx, f.workDir, prompt, ai.RunOptions{
	SessionID:    sessionID,
	Resume:       resume,
	SystemPrompt: systemPrompt,
}, logPath)
```

Drop the `skillHint` parameter from `buildPrompt`. Replace the function (lines 338-358) with:

```go
func buildPrompt(msgs []store.ClaimedMessage) string {
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

- [ ] **Step 2: Update internal tests for `buildPrompt`**

Rewrite `internal/domain/bee/feeder_internal_test.go` so the now-deleted `WithHint` cases become `BuildSystemPrompt`-coverage instead. Replace the file content with:

```go
package bee

import (
	"strings"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

func TestBuildPrompt_Single(t *testing.T) {
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "feishu", SessionKey: "feishu:oc_abc:ou_xyz", Content: "hello world"},
	}
	got := buildPrompt(msgs)
	wantMeta := `<message_meta>{"from":"feishu","session_key":"feishu:oc_abc:ou_xyz","message_id":"msg-1"}</message_meta>`
	if !strings.HasPrefix(got, wantMeta) {
		t.Errorf("missing message_meta prefix\ngot:  %q", got)
	}
	if !strings.Contains(got, "<message_content>") || !strings.Contains(got, "</message_content>") {
		t.Errorf("missing message_content tags, got: %q", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Errorf("missing original content, got: %q", got)
	}
	if strings.Contains(got, "[MANDATORY]") {
		t.Errorf("buildPrompt must not embed the skill hint anymore, got: %q", got)
	}
}

func TestBuildPrompt_Multiple(t *testing.T) {
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

func TestBuildPrompt_NeverHasPlatformContext(t *testing.T) {
	platform.RegisterExtractor("testplatform2", func(_ string) string {
		return `{"testplatform2":{"sender":{"open_id":"ou_abc"}}}`
	})
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "testplatform2", SessionKey: "testplatform2:oc_xyz:ou_abc", Content: "hello"},
	}
	got := buildPrompt(msgs)
	if strings.Contains(got, `"platform_context"`) {
		t.Errorf("platform_context must never appear in bee message_meta, got: %q", got)
	}
}

func TestBeeSystemPrompt_StartsWithSkillHint(t *testing.T) {
	got := ai.BuildSystemPrompt(ai.RoleBee, nil)
	if !strings.HasPrefix(got, ai.SkillHintPrefix(ai.RoleBee)) {
		t.Errorf("bee system prompt must start with skill hint, got: %q", got)
	}
}
```

- [ ] **Step 3: Decide whether `feeder_test.go` needs changes**

Run: `go test ./internal/domain/bee/... -count=1`

If failures show up in `feeder_test.go` (the integration-style file), open the failing test, locate where it asserts on the prompt passed to the mock runner, and update it: the prompt no longer includes the hint, but `RunOptions.SystemPrompt` does on the fresh path. Use the same shape as the dispatcher test changes:

```go
if strings.Contains(actualPrompt, "[MANDATORY]") {
	t.Errorf("hint must not appear in prompt anymore, got: %q", actualPrompt)
}
if !strings.HasPrefix(actualSystemPrompt, ai.SkillHintPrefix(ai.RoleBee)) {
	t.Errorf("fresh bee run must carry hint in SystemPrompt, got: %q", actualSystemPrompt)
}
```

If `feeder_test.go` does not assert on prompt content, leave it alone.

- [ ] **Step 4: Run all tests**

Run: `go test ./... -count=1`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/bee/feeder.go internal/domain/bee/feeder_internal_test.go internal/domain/bee/feeder_test.go
git commit -m "bee: route skill hint via SystemPrompt; drop hint param from buildPrompt"
```

---

## Task 10: Final verification

**Files:** none modified.

This task is a sanity gate before declaring done.

- [ ] **Step 1: Full test run**

Run: `go test ./... -count=1`
Expected: green.

- [ ] **Step 2: Vet**

Run: `go vet ./...`
Expected: no warnings.

- [ ] **Step 3: Format check**

Run: `gofmt -l internal/ | tee /tmp/gofmt.out`
Expected: empty output (no files need reformatting). If any file is listed, run `gofmt -w` on it and amend the corresponding commit.

- [ ] **Step 4: Smoke check the spec coverage**

Open `docs/superpowers/specs/2026-05-08-skill-hint-system-prompt-design.md` and verify each section maps to a task:

| Spec section | Task |
|---|---|
| 1. Adapter contract: `SystemPrompt` field | Task 1 |
| 2. `BuildSystemPrompt` helper | Task 2 |
| 3. Caller changes (dispatcher / manager / feeder) | Tasks 7, 8, 9 |
| 4. Resume policy (only fresh sessions) | Tasks 8 (dispatcher) + 9 (feeder) |
| Per-engine (claude/pi/codex/kimi) | Tasks 3, 4, 5, 6 |
| Testing | Embedded in each task |

- [ ] **Step 5: Final commit (if `gofmt` produced changes)**

If Step 3 made changes, commit them as a follow-up:

```bash
git add -u
git commit -m "chore: gofmt fixups from systemprompt migration"
```

Otherwise nothing to do.
