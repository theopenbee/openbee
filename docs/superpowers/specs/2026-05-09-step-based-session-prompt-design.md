# Step-Based Session Prompt Design

Date: 2026-05-09
Status: Approved (brainstorming)

## Background

When a Worker or Bee starts a fresh Claude session, the dispatcher / feeder
prepends a header to the user's instruction. The current header concatenates
three concerns into a flat block:

```
[MANDATORY] You MUST invoke the openbee-worker skill ...
<worker_persona>
  ## Role / ## Identity / ## Work Constraints
</worker_persona>
<task_meta>{...}</task_meta>
<task_content>
  实际任务
</task_content>
```

Observed problem: models sometimes skip the skill invocation and go straight to
the task body. The persona block sitting between the MANDATORY directive and
the task makes it easy to read the directive as background context rather than
a hard prerequisite.

## Goal

Restructure the header so that "load the skill" is unambiguously a prerequisite
step that precedes the task, while keeping the existing strong-language
deterrent ([MANDATORY], "before producing any other output").

Non-goal: changing skill content, persona content, or the dispatcher / feeder
control flow.

## Final Prompt Shape

### Worker (with persona)

```
Please complete the following two steps in order. Do not skip Step 1.

## Step 1: Initialize your role
[MANDATORY] You MUST invoke the openbee-worker skill immediately, before producing any other output. After the skill is loaded, internalize the persona below as your identity for the rest of this session:

<worker_persona>
## Role
You are a Worker in an AI team.

## Identity
Name: 貂蝉
Description: 负责 openbee 开发

## Work Constraints
...
</worker_persona>

## Step 2: Execute the task
<task_meta>{"message_id":"...","task_id":"..."}</task_meta>
<task_content>
实际任务
</task_content>
```

### Worker (no persona, `workerLookup == nil`)

Step 1 keeps the [MANDATORY] line but omits the `<worker_persona>` block.
Step 2 is unchanged.

### Bee

```
Please complete the following two steps in order. Do not skip Step 1.

## Step 1: Initialize your role
[MANDATORY] You MUST invoke the openbee-bee skill immediately, before producing any other output.

## Step 2: Handle the messages below
<原本的 messages 内容>
```

Bee has no persona, so Step 1 contains only the skill-load directive.

## Code Changes

### `internal/ai/prompt.go`

Introduce a single public builder that returns the full Step 1 + Step 2 header.
The caller appends the task body (instruction string for Worker, merged
messages for Bee) directly after the returned string.

```go
// BuildSessionPrefix returns the Step-1 + Step-2 header for a new session.
// The trailing "## Step 2: ...\n" line ends with a newline so the caller can
// append the task body directly without inserting extra separators.
//
//   role    — RoleWorker or RoleBee. Selects the skill name and the Step 2 title.
//   persona — Worker persona body produced by WorkerPersona(); pass "" for Bee
//             or when no worker record is available.
func BuildSessionPrefix(role Role, persona string) string
```

Behaviour:

- Step 2 title:
  - `RoleWorker` → `## Step 2: Execute the task`
  - `RoleBee`    → `## Step 2: Handle the messages below`
- `<worker_persona>` block is emitted only when `role == RoleWorker` and
  `persona != ""`.
- For unknown roles, return `""` (matches the previous `SkillHintPrefix`
  behaviour and keeps resume / unknown-role paths untouched).

Remove `SkillHintPrefix` from the public API. The MANDATORY sentence becomes an
unexported helper inside `prompt.go`. `WorkerPersona` is unchanged.

### `internal/domain/task/dispatcher.go` (around line 337)

Replace the current concatenation:

```go
hint := ai.SkillHintPrefix(ai.RoleWorker)
if d.workerLookup != nil {
    if worker == nil { return ..., fmt.Errorf("worker %q not found", task.WorkerID) }
    persona := ai.WorkerPersona(...)
    hint += "\n<worker_persona>\n" + persona + "</worker_persona>"
}
... hint + "\n" + instruction
```

with:

```go
persona := ""
if d.workerLookup != nil {
    if worker == nil {
        return model.WorkerExecution{}, fmt.Errorf("worker %q not found", task.WorkerID)
    }
    persona = ai.WorkerPersona(worker.Name, worker.Description, worker.Constraints)
}
prefix := ai.BuildSessionPrefix(ai.RoleWorker, persona)
... prefix + instruction
```

### `internal/domain/bee/feeder.go` (around line 203)

```go
prefix := ""
if !resume {
    prefix = ai.BuildSessionPrefix(ai.RoleBee, "")
}
prompt := buildPrompt(msgs, prefix)
```

`buildPrompt` already handles an empty prefix on resume; no change needed
there.

## Test Updates

### `internal/ai/prompt_test.go`

Replace the three `TestSkillHintPrefix_*` tests with new cases for
`BuildSessionPrefix`:

- `TestBuildSessionPrefix_WorkerWithPersona`
  - Asserts the output contains `## Step 1: Initialize your role`,
    `[MANDATORY]`, the literal skill name `openbee-worker`,
    `<worker_persona>`, the persona body, `</worker_persona>`, and ends with
    `## Step 2: Execute the task\n`.
- `TestBuildSessionPrefix_WorkerNoPersona`
  - persona == ""; output must NOT contain `<worker_persona>` but MUST contain
    `## Step 1` and `## Step 2: Execute the task\n`.
- `TestBuildSessionPrefix_Bee`
  - role == RoleBee; output contains `openbee-bee`, ends with
    `## Step 2: Handle the messages below\n`, no `<worker_persona>`.
- `TestBuildSessionPrefix_UnknownRole`
  - returns `""`.

`TestWorkerPersona_*` tests stay as-is (function unchanged).

### `internal/domain/task/dispatcher_test.go`

Lines 1191, 1221, 1257, 1298 currently use
`strings.HasPrefix(instruction, ai.SkillHintPrefix(ai.RoleWorker))`. Update
each call site to use the new prefix:

- "starts with prefix" assertions become
  `strings.HasPrefix(instruction, ai.BuildSessionPrefix(ai.RoleWorker, ""))` for
  the no-persona path, or check for the literal `## Step 1: Initialize your role`
  marker which is stable across both persona / no-persona variants.
- The `TestTaskDispatcher_NewSession_InjectsWorkerPersona` test should also
  assert that the `<worker_persona>` block sits between Step 1 and Step 2 (i.e.
  the persona appears before `## Step 2:`).

## Edge Cases

- **Resume sessions**: unchanged. Both dispatcher and feeder skip the prefix on
  resume; `BuildSessionPrefix` is not invoked in that path.
- **Worker lookup fails / `worker == nil` after lookup configured**: existing
  behaviour preserved — abort with the same error before building the prefix.
- **Empty persona fields** (name/description/constraints all empty): `WorkerPersona`
  still returns the `## Role` line, so the persona block is non-empty and gets
  emitted. No special-casing needed.
- **Unknown role**: `BuildSessionPrefix` returns `""`, matching the legacy
  `SkillHintPrefix` fallback so callers that previously checked for empty
  prefix continue to behave the same.

## Risks

- Header is ~4 lines longer per new session. Token cost is negligible (<100
  tokens) compared to the task body.
- Existing tests that grep for the literal `[MANDATORY] You MUST invoke...`
  line still pass — that sentence is preserved verbatim inside Step 1.
