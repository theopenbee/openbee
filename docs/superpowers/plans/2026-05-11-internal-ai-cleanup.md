# internal/ai 清理与重构实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 `internal/ai` 目录在代码复用、质量、效率三个维度上发现的 18 个问题，消除四个 provider（claude/codex/kimi/pi）的骨架重复，并修复若干性能与抽象隐患。

**Architecture:** 分 7 个阶段渐进式重构。先做无 API 变化的清理（Phase 0），再抽取共享 helper（Phase 1-2），然后引入嵌入式 `BaseAdapter` 收敛 4 个 provider（Phase 3-4），最后做表驱动 provider.go 重构（Phase 5）与一系列效率修复（Phase 6）。每个阶段都是独立提交，可中断恢复。

**Tech Stack:** Go 1.22+（已用 generics），`go test ./...` 作为回归门。

---

## 三类发现 → 阶段映射

- 一.1 Adapter 重复 → Phase 4
- 一.2 Invoker 构造重复 → Phase 3
- 一.3 Run 子进程脚手架重复 → Phase 2
- 一.4 Token Collector 重复 → Phase 1
- 一.5 env-or-default session dir 三份相同 → Phase 1
- 一.6 ExtractResultFromLog 重复 → Phase 4（与 Adapter 一起做）
- 二.1 Claude 日志双扫 → Phase 6
- 二.2 Kimi 双 unmarshal → Phase 6
- 二.3 Pi stripThinkingSignature 三重 marshal → Phase 6
- 二.4 Codex pipe 整生命周期开销 → Phase 6
- 二.5 BuildBaseEnv 每次重建 → Phase 6
- 二.6 sessionfile TOCTOU → Phase 6
- 三.1 RunResult.ExtractResult 泄漏 logPath → Phase 4（顺便修）
- 三.2 SystemRulesFile/ImportLine claude 专属 → Phase 0
- 三.3 ConfigureProvider 表驱动 → Phase 5
- 三.4 contracts.go DrainUsageMap 不该公开 → Phase 1（随着 collector 抽取移走）
- 三.4 dynamic.CollectTokenUsage 接口洁癖 → 跳过（接口改动牵连太广，标注 TODO 即可，**不在本计划范围**）
- 三.4 process.CmdProcess.mu → Phase 0
- 三.4 allEngines 与 registry 重复 → 跳过（registry 不保证顺序，allEngines 是 source of truth，**不修**）
- 三.5 stringly-typed env keys → Phase 5
- 三.5 codex/kimi event-type strings → Phase 0
- 三.6 嵌套条件（engine_args splitCLIArgs、provider.go switch、codex parseCodexFile）→ Phase 5（provider）+ 跳过其余（可读性已可接受）
- 三.6 无用注释 → Phase 0

**总计：实修 16 项，跳过 2 项。**

---

## 通用约定

- 每个 Task 完成后运行 `go build ./... && go test ./internal/ai/...` 必须通过。
- 每个 Task 完成后立即 commit。
- 测试不删：现有测试当回归门用；如果重构破坏接口需要同步改测试，把改测试当作 Task 的一部分。
- 所有"删除"操作都要 `git grep` 确认无其他引用。

---

## Phase 0 — 无 API 变化的纯清理

目标：删冗余注释、移动 Claude 专属常量、加事件类型常量。可在不影响任何 API 的前提下完成。

### Task 0.1：删除 process.go 与 contracts.go 中的多余注释

**Files:**
- Modify: `internal/ai/process.go`
- Modify: `internal/ai/contracts.go`
- Modify: `internal/ai/dynamic.go`

- [ ] **Step 1：删除 `process.go` 中重复的 clip 注释**

`internal/ai/process.go:46-47`、`:74` 两处 "Clip to length so concurrent append calls in Run() cannot share the backing array" 是相同警告，且解释 WHAT。保留 `BuildRunEnv` 上方 33-36 行的核心解释（"last value wins" 是真 WHY），其他删除。

具体删除：
- 第 45-47 行（`AppendExtraEnv` 注释的后半段冗余）→ 保留 "appends non-empty entries"，删除 "preventing concurrent..."
- 第 57-59 行（`BuildBaseEnv` 注释末句 "OPENBEE_URL is inherited..."）→ 这条解释了**不存在的代码**，删除
- 第 74 行（重复的 clip 注释）→ 删除

- [ ] **Step 2：删除 `contracts.go` 中的 WHAT 注释**

删除以下注释（行号以当前 contracts.go 为准）：
- 第 24 行 `// Add future fields here without changing the Prepare method signature.` —— 自描述
- 第 75-77 行 `// RunResult is the handle returned from EngineAdapter.Run...` 留首句，删 "bound to the engine that handled this Run, so it remains correct even if the active engine later changes."（实现细节）
- 第 84-85 行 `// NewRunResult wraps the (process, output, error) tuple returned by an engine invoker into a RunResult, attaching the engine's result extractor on success.` 简化为 `// NewRunResult builds a RunResult, propagating err unchanged.`

- [ ] **Step 3：删除 `dynamic.go` 实现 gossip 注释**

`internal/ai/dynamic.go:24-26`：`// Most engines have a no-op Prepare; the only meaningful work (Claude's legacy file cleanup) is a single os.Remove, so a sequential loop is sufficient.` —— 删除。保留首句 `// Prepare initialises every engine adapter for the given workDir.`

- [ ] **Step 4：删除三个 invoker 的 "OPENBEE_URL is inherited" 复制注释**

`internal/ai/claude/invoker.go:20-21`、`kimi/invoker.go:21-22`、`pi/invoker.go:22` 的 "OPENBEE_URL is inherited from the server process environment" 全删（解释的是 BuildBaseEnv 的行为，不属于这里）。

- [ ] **Step 5：删除 `var _ ai.EngineAdapter = (*xxxAdapter)(nil)` 三份冗余**

删除：
- `internal/ai/codex/adapter.go:46`
- `internal/ai/kimi/adapter.go:41`
- `internal/ai/pi/adapter.go:42`

理由：`ai.Register` 的 `Factory` 返回值类型是 `ai.EngineAdapter`，编译期已强制实现。

- [ ] **Step 6：构建 & 测试**

```bash
go build ./...
go test ./internal/ai/...
```

预期：全绿。

- [ ] **Step 7：Commit**

```bash
git add internal/ai/process.go internal/ai/contracts.go internal/ai/dynamic.go \
        internal/ai/claude/invoker.go internal/ai/kimi/invoker.go internal/ai/pi/invoker.go \
        internal/ai/codex/adapter.go internal/ai/kimi/adapter.go internal/ai/pi/adapter.go
git commit -m "refactor(ai): remove redundant comments and interface assertions"
```

---

### Task 0.2：把 Claude 专属常量挪进 claude 包

`SystemRulesFile` / `ImportLine` 只被 `claude/adapter.go` 和 `claude/adapter_test.go` 用到，不该在共享 contracts.go。

**Files:**
- Modify: `internal/ai/contracts.go`（删除两个常量）
- Modify: `internal/ai/claude/adapter.go`（新增本地常量，更新引用）
- Modify: `internal/ai/claude/adapter_test.go`（更新引用）

- [ ] **Step 1：在 `claude/adapter.go` 顶部新增常量**

在 `package claude` 下、`func init()` 之前插入：

```go
const (
	// systemRulesFile is the legacy rules file Claude's Prepare cleans up.
	systemRulesFile = ".openbee.md"
	// importLine is the legacy reference line removed from CLAUDE.md.
	importLine = "@" + systemRulesFile
)
```

- [ ] **Step 2：替换 `claude/adapter.go` 中的引用**

```bash
# 把 ai.SystemRulesFile 换成 systemRulesFile
# 把 ai.ImportLine 换成 importLine
```

用 Edit 工具替换（共 3 处：第 34、36、61 行）。

- [ ] **Step 3：替换 `claude/adapter_test.go` 中的引用**

测试里的 `ai.SystemRulesFile` 改成 `systemRulesFile`，`ai.ImportLine` 改成 `importLine`（共 5 处：第 39、56、67、108、109、114 行）。注意它们是同包测试可以直接用小写常量。

- [ ] **Step 4：从 `contracts.go` 删除常量**

删除 `internal/ai/contracts.go` 第 8-13 行（`SystemRulesFile`、`ImportLine` 块）。

- [ ] **Step 5：构建 & 测试**

```bash
go build ./...
go test ./internal/ai/claude/...
go test ./internal/ai/...
```

- [ ] **Step 6：Commit**

```bash
git add internal/ai/contracts.go internal/ai/claude/adapter.go internal/ai/claude/adapter_test.go
git commit -m "refactor(ai): move SystemRulesFile/ImportLine into claude package"
```

---

### Task 0.3：为 codex/kimi 事件类型字符串补常量

pi 包已有 `eventTypeAgentEnd` 等常量（`pi/invoker.go:52-57`），codex/kimi 没有。补齐让风格一致，且去 stringly-typed。

**Files:**
- Modify: `internal/ai/codex/invoker.go`
- Modify: `internal/ai/codex/token_usage.go`
- Modify: `internal/ai/kimi/invoker.go`

- [ ] **Step 1：codex/invoker.go 新增常量并替换**

在 `var log = ...` 之后插入：

```go
const (
	codexEventThreadStarted = "thread.started"
	codexEventItemCompleted = "item.completed"
	codexItemAgentMessage   = "agent_message"
)
```

把 `"thread.started"`（第 71 行）、`"item.completed"`（第 95 行）、`"agent_message"`（第 96 行）替换为对应常量。

- [ ] **Step 2：codex/token_usage.go 新增事件常量**

在 `type codexJSONLLine struct {` 上方插入：

```go
const (
	codexLineTurnContext = "turn_context"
	codexLineEventMsg    = "event_msg"
	codexPayloadTokens   = "token_count"
)
```

把 `parseCodexFile`（`codex/token_usage.go:131-137`）的 `"turn_context"` / `"event_msg"` / `"token_count"` 替换为常量。

- [ ] **Step 3：kimi/invoker.go 新增常量**

```go
const (
	kimiRoleAssistant = "assistant"
	kimiToolShell     = "Shell"
	kimiContentText   = "text"
	kimiEmptyPrefix   = "(Empty response:"
)
```

替换 `kimi/invoker.go:91, 95, 111, 121` 的对应字面量。

- [ ] **Step 4：构建 & 测试**

```bash
go build ./...
go test ./internal/ai/...
```

- [ ] **Step 5：Commit**

```bash
git add internal/ai/codex/invoker.go internal/ai/codex/token_usage.go internal/ai/kimi/invoker.go
git commit -m "refactor(ai): replace stringly-typed event identifiers with constants"
```

---

### Task 0.4：删除 `process.CmdProcess.mu`

读 `process_unix.go` 后确认 `Stop()` 只读 `p.cmd.Process.Pid`，不修改字段；`p.cmd` 在 `NewCmdProcess` 之后只读。互斥锁纯属噪声。

**Files:**
- Modify: `internal/ai/process.go`
- Modify: `internal/ai/process_unix.go`
- Modify: `internal/ai/process_windows.go`

- [ ] **Step 1：先验证 windows 版本**

```bash
cat internal/ai/process_windows.go
```

确认 `Stop()` 也不写 `p.cmd`。

- [ ] **Step 2：删除 mu 字段**

`internal/ai/process.go:14-17`：

改前：
```go
type CmdProcess struct {
	cmd *exec.Cmd
	mu  sync.Mutex
}
```

改后：
```go
type CmdProcess struct {
	cmd *exec.Cmd
}
```

同时删除 `import "sync"`。

- [ ] **Step 3：删除 `PID()` 内的 Lock/Unlock**

`internal/ai/process.go:24-31`：

改后：
```go
func (p *CmdProcess) PID() int {
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}
```

- [ ] **Step 4：删除 process_unix.go 与 process_windows.go 中 Stop() 的 Lock/Unlock**

- [ ] **Step 5：构建 & 测试**

```bash
go build ./...
go test ./internal/ai/... -race
```

`-race` 必加：确认没有数据竞争。

- [ ] **Step 6：Commit**

```bash
git add internal/ai/process.go internal/ai/process_unix.go internal/ai/process_windows.go
git commit -m "refactor(ai): drop unused mutex on CmdProcess"
```

---

## Phase 1 — Token Collector 抽取（一.4 + 一.5 + 三.4-DrainUsageMap）

目标：把 4 个 `parseXxxFile` 的"打开 → 扫描 → 解析 → 聚合 → drain" 五步模式抽成一个泛型 helper。同时把 `DrainUsageMap` 内化（不再公开），把 `EngineSessionsDir(envVar, defaultFn)` 抽出。

### Task 1.1：新增 `config.EngineSessionsDir` helper

**Files:**
- Modify: `internal/infra/config/config.go`

- [ ] **Step 1：在 `config.go` 中新增**

在 `DefaultKimiSessionsDir` 之后添加：

```go
// EngineSessionsDir returns the directory from envVar, or the fallback when unset.
func EngineSessionsDir(envVar string, fallback func() string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return fallback()
}
```

- [ ] **Step 2：构建**

```bash
go build ./...
```

- [ ] **Step 3：Commit**

```bash
git add internal/infra/config/config.go
git commit -m "refactor(config): add EngineSessionsDir env-or-fallback helper"
```

---

### Task 1.2：新增 `ai.AggregateUsage` 泛型 helper

**Files:**
- Create: `internal/ai/usage.go`
- Create: `internal/ai/usage_test.go`

- [ ] **Step 1：写测试 `internal/ai/usage_test.go`**

```go
package ai

import (
	"path/filepath"
	"os"
	"testing"
)

func TestAggregateUsage_AddsAcrossLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	body := `{"model":"m1","in":10,"out":2}
{"model":"m1","in":5,"out":1}
{"model":"m2","in":7,"out":3}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	type line struct {
		Model string `json:"model"`
		In    int64  `json:"in"`
		Out   int64  `json:"out"`
	}
	usages, err := AggregateUsage[line](path, func(l line, agg map[string]*TokenUsage) {
		if l.Model == "" {
			return
		}
		u := agg[l.Model]
		if u == nil {
			u = &TokenUsage{Model: l.Model}
			agg[l.Model] = u
		}
		u.InputTokens += l.In
		u.OutputTokens += l.Out
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 2 {
		t.Fatalf("want 2 models, got %d", len(usages))
	}
	byModel := map[string]TokenUsage{}
	for _, u := range usages {
		byModel[u.Model] = u
	}
	if byModel["m1"].InputTokens != 15 || byModel["m1"].OutputTokens != 3 {
		t.Errorf("m1 wrong: %+v", byModel["m1"])
	}
	if byModel["m2"].InputTokens != 7 || byModel["m2"].OutputTokens != 3 {
		t.Errorf("m2 wrong: %+v", byModel["m2"])
	}
}

func TestAggregateUsage_MissingFile(t *testing.T) {
	_, err := AggregateUsage[struct{}]("/nonexistent/path", func(struct{}, map[string]*TokenUsage) {})
	if err == nil {
		t.Fatal("want error for missing file")
	}
}

func TestAggregateUsage_SkipsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	body := `not-json
{"model":"m1","in":3,"out":1}
also-not-json
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	type line struct {
		Model string `json:"model"`
		In    int64  `json:"in"`
		Out   int64  `json:"out"`
	}
	usages, err := AggregateUsage[line](path, func(l line, agg map[string]*TokenUsage) {
		if l.Model == "" {
			return
		}
		u := agg[l.Model]
		if u == nil {
			u = &TokenUsage{Model: l.Model}
			agg[l.Model] = u
		}
		u.InputTokens += l.In
		u.OutputTokens += l.Out
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 1 || usages[0].InputTokens != 3 {
		t.Errorf("got %+v", usages)
	}
}
```

- [ ] **Step 2：跑测试看失败**

```bash
go test ./internal/ai/ -run TestAggregateUsage -v
```

预期：编译错（`AggregateUsage` 未定义）。

- [ ] **Step 3：实现 `internal/ai/usage.go`**

```go
package ai

import (
	"encoding/json"

	"github.com/theopenbee/openbee/internal/utils/sessionfile"
)

// AggregateUsage scans a JSONL file at path, unmarshals each line as T,
// and lets fold accumulate per-model TokenUsage into agg. Lines that fail
// to unmarshal are silently skipped (matches existing per-engine behavior).
// The result slice is in undefined order; callers may sort if needed.
func AggregateUsage[T any](path string, fold func(line T, agg map[string]*TokenUsage)) ([]TokenUsage, error) {
	agg := map[string]*TokenUsage{}
	err := sessionfile.ScanJSONLFile(path, func(data []byte) {
		var line T
		if json.Unmarshal(data, &line) != nil {
			return
		}
		fold(line, agg)
	})
	if err != nil {
		return nil, err
	}
	out := make([]TokenUsage, 0, len(agg))
	for _, u := range agg {
		out = append(out, *u)
	}
	return out, nil
}
```

- [ ] **Step 4：跑测试看通过**

```bash
go test ./internal/ai/ -run TestAggregateUsage -v
```

预期：PASS。

- [ ] **Step 5：Commit**

```bash
git add internal/ai/usage.go internal/ai/usage_test.go
git commit -m "feat(ai): add AggregateUsage generic helper for token-usage parsing"
```

---

### Task 1.3：迁移 claude collector 到 `AggregateUsage`

**Files:**
- Modify: `internal/ai/claude/token_usage.go`

- [ ] **Step 1：替换 `claudeBaseDirs` 用 `EngineSessionsDir`？**

Claude 略特殊（baseDirs 多个、用 `SplitAndTrim`），保持原样。这条只在 codex/pi 上做。

- [ ] **Step 2：把 `parseClaudeFile` 改为调用 `AggregateUsage`**

改前（`claude/token_usage.go:71-99`）：

```go
func parseClaudeFile(path string) ([]ai.TokenUsage, error) {
	agg := map[string]*ai.TokenUsage{}
	err := sessionfile.ScanJSONLFile(path, func(data []byte) {
		var line claudeJSONLLine
		if err := json.Unmarshal(data, &line); err != nil {
			return
		}
		m := line.Message.Model
		if m == "" || m == syntheticModel || line.Message.Usage == nil {
			return
		}
		if line.Message.Speed == "fast" {
			m += "-fast"
		}
		u, ok := agg[m]
		if !ok {
			u = &ai.TokenUsage{Model: m}
			agg[m] = u
		}
		u.InputTokens += line.Message.Usage.InputTokens
		u.OutputTokens += line.Message.Usage.OutputTokens
		u.CacheCreationTokens += line.Message.Usage.CacheCreationInputTokens
		u.CacheReadTokens += line.Message.Usage.CacheReadInputTokens
	})
	if err != nil {
		return nil, fmt.Errorf("scan claude session file: %w", err)
	}
	return ai.DrainUsageMap(agg), nil
}
```

改后：

```go
func parseClaudeFile(path string) ([]ai.TokenUsage, error) {
	usages, err := ai.AggregateUsage[claudeJSONLLine](path, func(line claudeJSONLLine, agg map[string]*ai.TokenUsage) {
		m := line.Message.Model
		if m == "" || m == syntheticModel || line.Message.Usage == nil {
			return
		}
		if line.Message.Speed == "fast" {
			m += "-fast"
		}
		u := agg[m]
		if u == nil {
			u = &ai.TokenUsage{Model: m}
			agg[m] = u
		}
		u.InputTokens += line.Message.Usage.InputTokens
		u.OutputTokens += line.Message.Usage.OutputTokens
		u.CacheCreationTokens += line.Message.Usage.CacheCreationInputTokens
		u.CacheReadTokens += line.Message.Usage.CacheReadInputTokens
	})
	if err != nil {
		return nil, fmt.Errorf("scan claude session file: %w", err)
	}
	return usages, nil
}
```

删除 import `"encoding/json"` 和 `"github.com/theopenbee/openbee/internal/utils/sessionfile"`（如果不再用）。

- [ ] **Step 3：测试**

```bash
go test ./internal/ai/claude/...
```

预期：PASS（行为等价）。

- [ ] **Step 4：Commit**

```bash
git add internal/ai/claude/token_usage.go
git commit -m "refactor(ai/claude): use ai.AggregateUsage helper"
```

---

### Task 1.4：迁移 kimi collector

Kimi 特殊：它只取**最后一条** StatusUpdate 的快照，不是累加。`AggregateUsage` 是累加模型，所以这里**不**用 `AggregateUsage`，但用 `EngineSessionsDir`。

实际看 `kimi/token_usage.go:21-28`，NewCollector 没用 env var，只用 `config.DefaultKimiSessionsDir()`。**跳过 env 改造**，但本 Task 仍做：

**Files:**
- Modify: `internal/ai/kimi/token_usage.go`

- [ ] **Step 1：把 `parseKimiFile` 改为返回结构清晰，保留语义**

实际上 `parseKimiFile` 的"取最后一条"逻辑用 `AggregateUsage` 模拟成本反而高。**这条不改，跳过 Task 1.4。**

- [ ] **Skip：标注**

注释一下：`kimi` 是"取最后一条快照"，与累加语义不同，不进入泛型。

- [ ] **Step 2：Commit (空 commit 不必要)** —— 跳过。

---

### Task 1.5：迁移 pi collector

**Files:**
- Modify: `internal/ai/pi/token_usage.go`

- [ ] **Step 1：把 `NewCollector` 用 `EngineSessionsDir`**

改前：

```go
func NewCollector() *Collector {
	dir := os.Getenv("PI_AGENT_DIR")
	if dir == "" {
		dir = config.DefaultPiSessionsDir()
	}
	return NewCollectorAt(dir)
}
```

改后：

```go
func NewCollector() *Collector {
	return NewCollectorAt(config.EngineSessionsDir("PI_AGENT_DIR", config.DefaultPiSessionsDir))
}
```

- [ ] **Step 2：把 `parsePiFile` 改为 `AggregateUsage`**

类似 claude 的改法。改后：

```go
func parsePiFile(path string) ([]ai.TokenUsage, error) {
	usages, err := ai.AggregateUsage[piJSONLLine](path, func(line piJSONLLine, agg map[string]*ai.TokenUsage) {
		if line.Type != "message" || line.Message.Role != "assistant" || line.Message.Usage == nil {
			return
		}
		m := line.Message.Model
		u := agg[m]
		if u == nil {
			u = &ai.TokenUsage{Model: m}
			agg[m] = u
		}
		u.InputTokens += line.Message.Usage.Input
		u.OutputTokens += line.Message.Usage.Output
		u.CacheCreationTokens += line.Message.Usage.CacheWrite
		u.CacheReadTokens += line.Message.Usage.CacheRead
	})
	if err != nil {
		return nil, fmt.Errorf("scan pi session file: %w", err)
	}
	return usages, nil
}
```

清理无用 import。

- [ ] **Step 3：测试**

```bash
go test ./internal/ai/pi/...
```

- [ ] **Step 4：Commit**

```bash
git add internal/ai/pi/token_usage.go
git commit -m "refactor(ai/pi): use EngineSessionsDir and AggregateUsage helpers"
```

---

### Task 1.6：迁移 codex collector

Codex 的 `parseCodexFile` 内部有跨行状态（`currentModel`、`prevByModel`），`AggregateUsage` 的 `fold` 签名是 `(T, agg)`，状态需要别处持有。最简方案：在 fold 闭包外定义 `currentModel` / `prevByModel`，闭包捕获。

**Files:**
- Modify: `internal/ai/codex/token_usage.go`

- [ ] **Step 1：替换 NewCollector 的 CODEX_HOME 处理**

改前：

```go
func NewCollector() *Collector {
	codexBase := os.Getenv("CODEX_HOME")
	if codexBase == "" {
		home, _ := os.UserHomeDir()
		codexBase = filepath.Join(home, ".codex")
	}
	return NewCollectorAt(config.DefaultCodexSessionsDir(), codexBase)
}
```

改后：

```go
func defaultCodexBase() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

func NewCollector() *Collector {
	return NewCollectorAt(
		config.DefaultCodexSessionsDir(),
		config.EngineSessionsDir("CODEX_HOME", defaultCodexBase),
	)
}
```

- [ ] **Step 2：把 `parseCodexFile` 改为 `AggregateUsage`**

改后（保留所有原有语义）：

```go
func parseCodexFile(path string) ([]ai.TokenUsage, error) {
	prevByModel := map[string]*codexTokenUsage{}
	currentModel := ""
	return ai.AggregateUsage[codexJSONLLine](path, func(line codexJSONLLine, agg map[string]*ai.TokenUsage) {
		switch line.Type {
		case codexLineTurnContext:
			if line.Payload.Model != "" {
				currentModel = line.Payload.Model
			}
		case codexLineEventMsg:
			if line.Payload.Type != "" && line.Payload.Type != codexPayloadTokens {
				return
			}
			info := line.tokenInfo()
			if info == nil {
				return
			}
			m := codexResolveModel(info, currentModel)
			if m == "" {
				return
			}
			u := agg[m]
			if u == nil {
				u = &ai.TokenUsage{Model: m}
				agg[m] = u
			}
			if prevByModel[m] == nil {
				prevByModel[m] = &codexTokenUsage{}
			}
			prev := prevByModel[m]
			if info.LastTokenUsage != nil {
				addCodexUsage(u, *info.LastTokenUsage)
				if info.TotalTokenUsage != nil {
					// Codex emits both fields together when a turn is replayed/resumed;
					// the cumulative total is authoritative, so reset prev instead of
					// double-counting by advancing it.
					*prev = *info.TotalTokenUsage
				} else {
					prev.advance(*info.LastTokenUsage)
				}
			} else if info.TotalTokenUsage != nil {
				addCodexUsage(u, prev.deltaAndSet(*info.TotalTokenUsage))
			}
		}
	})
}
```

外层错误 wrap 丢失，补回：

```go
func parseCodexFile(path string) ([]ai.TokenUsage, error) {
	prevByModel := map[string]*codexTokenUsage{}
	currentModel := ""
	usages, err := ai.AggregateUsage[codexJSONLLine](path, func(line codexJSONLLine, agg map[string]*ai.TokenUsage) {
		// ... 同上 ...
	})
	if err != nil {
		return nil, fmt.Errorf("scan codex session file: %w", err)
	}
	return usages, nil
}
```

- [ ] **Step 3：清理 import**

如果 `encoding/json`、`sessionfile` 已无引用，删除。

- [ ] **Step 4：测试**

```bash
go test ./internal/ai/codex/...
```

- [ ] **Step 5：Commit**

```bash
git add internal/ai/codex/token_usage.go
git commit -m "refactor(ai/codex): use EngineSessionsDir and AggregateUsage helpers"
```

---

### Task 1.7：移除 `ai.DrainUsageMap`

如果所有调用方都已迁移到 `AggregateUsage`，`DrainUsageMap` 不再有外部调用者。

**Files:**
- Modify: `internal/ai/contracts.go`

- [ ] **Step 1：确认无外部引用**

```bash
git grep -n DrainUsageMap
```

预期：仅剩 `contracts.go` 中的定义。

- [ ] **Step 2：删除 `DrainUsageMap` 与其上方注释**

删除 `contracts.go:125-132`。

- [ ] **Step 3：测试**

```bash
go build ./...
go test ./internal/ai/...
```

- [ ] **Step 4：Commit**

```bash
git add internal/ai/contracts.go
git commit -m "refactor(ai): remove now-unused DrainUsageMap"
```

---

## Phase 2 — Run 子进程脚手架 helper（一.3）

目标：抽取 claude/kimi 几乎相同的 "open log → exec.CommandContext → set fields → BuildRunEnv → ConfigureCmd → Start → 启动 wait goroutine" 序列。codex（pipe + 早期 extractSessionID）和 pi（scanner + stripping）有显著定制，本阶段**不强行收编**它们。

### Task 2.1：新增 `ai.SpawnSubprocess` helper

**Files:**
- Create: `internal/ai/spawn.go`
- Create: `internal/ai/spawn_test.go`

- [ ] **Step 1：先写测试 `spawn_test.go`**

```go
package ai_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestSpawnSubprocess_HappyPath(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "log.txt")

	spec := ai.SubprocessSpec{
		Binary:  "/bin/sh",
		Args:    []string{"-c", "echo hello"},
		WorkDir: dir,
		LogPath: logPath,
		Env:     os.Environ(),
	}

	proc, out, err := ai.SpawnSubprocess(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if proc.PID() == 0 {
		t.Error("PID should be non-zero")
	}

	select {
	case ev := <-out:
		if ev.Type != ai.OutputDone {
			t.Errorf("want Done, got %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	data, _ := os.ReadFile(logPath)
	if string(data) != "hello\n" {
		t.Errorf("log content = %q", string(data))
	}
}

func TestSpawnSubprocess_NonZeroExit(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "log.txt")

	spec := ai.SubprocessSpec{
		Binary:  "/bin/sh",
		Args:    []string{"-c", "exit 7"},
		WorkDir: dir,
		LogPath: logPath,
		Env:     os.Environ(),
	}

	_, out, err := ai.SpawnSubprocess(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-out:
		if ev.Type != ai.OutputError {
			t.Errorf("want Error, got %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestSpawnSubprocess_OpenLogFailure(t *testing.T) {
	spec := ai.SubprocessSpec{
		Binary:  "/bin/sh",
		LogPath: "/nonexistent-dir/log.txt",
	}
	_, _, err := ai.SpawnSubprocess(context.Background(), spec)
	if err == nil {
		t.Fatal("want error opening log")
	}
}
```

- [ ] **Step 2：实现 `internal/ai/spawn.go`**

```go
package ai

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// SubprocessSpec describes a subprocess to launch via SpawnSubprocess.
// Stdin is supplied via the optional Stdin string; both stdout and stderr
// are redirected to LogPath unless StdoutOverride is non-nil.
type SubprocessSpec struct {
	Binary         string
	Args           []string
	WorkDir        string
	LogPath        string
	Env            []string
	Stdin          string // optional; if empty, stdin is not connected
	StdoutOverride io.Writer // optional; if nil, stdout goes to logfile
	StderrOverride io.Writer // optional; if nil, stderr goes to logfile
	// PostWait, if set, runs after cmd.Wait() returns and can override the
	// terminal Output. Returning nil means "use the standard mapping":
	// non-nil cmd error → OutputError, nil → OutputDone.
	PostWait func(waitErr error, logPath string) *Output
}

// SpawnSubprocess starts a subprocess described by spec, redirects its
// output to spec.LogPath, and returns a Process handle plus a 1-buffered
// channel that will receive exactly one Output event (Done or Error)
// when the process exits.
func SpawnSubprocess(ctx context.Context, spec SubprocessSpec) (Process, <-chan Output, error) {
	logFile, err := os.OpenFile(spec.LogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.CommandContext(ctx, spec.Binary, spec.Args...)
	cmd.Dir = spec.WorkDir
	if spec.Stdin != "" {
		cmd.Stdin = strings.NewReader(spec.Stdin)
	}
	if spec.StdoutOverride != nil {
		cmd.Stdout = spec.StdoutOverride
	} else {
		cmd.Stdout = logFile
	}
	if spec.StderrOverride != nil {
		cmd.Stderr = spec.StderrOverride
	} else {
		cmd.Stderr = logFile
	}
	cmd.Env = spec.Env
	ConfigureCmd(cmd)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("start subprocess: %w", err)
	}

	proc := NewCmdProcess(cmd)
	ch := make(chan Output, 1)

	go func() {
		defer close(ch)
		defer logFile.Close()
		waitErr := cmd.Wait()
		if spec.PostWait != nil {
			if ev := spec.PostWait(waitErr, spec.LogPath); ev != nil {
				ch <- *ev
				return
			}
		}
		if waitErr != nil {
			ch <- Output{Type: OutputError, Content: waitErr.Error()}
		} else {
			ch <- Output{Type: OutputDone}
		}
	}()

	return proc, ch, nil
}
```

- [ ] **Step 3：测试**

```bash
go test ./internal/ai/ -run TestSpawnSubprocess -v
```

预期：PASS。

- [ ] **Step 4：Commit**

```bash
git add internal/ai/spawn.go internal/ai/spawn_test.go
git commit -m "feat(ai): add SpawnSubprocess helper for engine subprocess launch"
```

---

### Task 2.2：迁移 claude invoker 到 `SpawnSubprocess`

**Files:**
- Modify: `internal/ai/claude/invoker.go`

- [ ] **Step 1：替换 `Run` 方法**

改前（`claude/invoker.go:85-141`）：见原文。

改后：

```go
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	args := []string{
		"--dangerously-skip-permissions",
		"--verbose",
		"--output-format", "stream-json",
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

	spec := ai.SubprocessSpec{
		Binary:  inv.binary,
		Args:    args,
		WorkDir: workDir,
		LogPath: logPath,
		Env:     ai.BuildRunEnv(inv.baseEnv, opts.ExtraEnv, opts.APIKey),
		Stdin:   prompt,
		PostWait: func(waitErr error, logPath string) *ai.Output {
			if waitErr != nil {
				return nil // default mapping → OutputError
			}
			result, isError, _ := scanResultLog(logPath)
			if !isError {
				return nil // default → OutputDone
			}
			if result == "" {
				result = "bee execution failed (no details available)"
			}
			return &ai.Output{Type: ai.OutputError, Content: result}
		},
	}
	return ai.SpawnSubprocess(ctx, spec)
}
```

- [ ] **Step 2：清理无用 import（`os/exec`、`strings`、`os` 可能仍需）**

`os.Open` 还在 `scanResultLog`；`exec` 不再用；`strings.NewReader` 不再用。

- [ ] **Step 3：测试**

```bash
go test ./internal/ai/claude/...
```

- [ ] **Step 4：Commit**

```bash
git add internal/ai/claude/invoker.go
git commit -m "refactor(ai/claude): migrate invoker to SpawnSubprocess"
```

---

### Task 2.3：迁移 kimi invoker 到 `SpawnSubprocess`

**Files:**
- Modify: `internal/ai/kimi/invoker.go`

- [ ] **Step 1：替换 `Run` 方法**

改后：

```go
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {

	spec := ai.SubprocessSpec{
		Binary:  inv.binary,
		Args:    buildArgs(opts.SessionID, opts.ExtraArgs),
		WorkDir: workDir,
		LogPath: logPath,
		Env:     ai.BuildRunEnv(inv.baseEnv, opts.ExtraEnv, opts.APIKey),
		Stdin:   prompt,
	}
	return ai.SpawnSubprocess(ctx, spec)
}
```

- [ ] **Step 2：清理 import**

- [ ] **Step 3：测试 & Commit**

```bash
go test ./internal/ai/kimi/...
git add internal/ai/kimi/invoker.go
git commit -m "refactor(ai/kimi): migrate invoker to SpawnSubprocess"
```

**注意**：codex / pi 暂不迁移到 `SpawnSubprocess`：codex 需要 pipe + 早期 extractSessionID，pi 需要逐行 scanner + stripping；强行套 helper 会让 helper API 失控。在 Phase 6 的 codex pipe 优化里再单独处理。

---

## Phase 3 — Invoker 构造统一（一.2）

目标：4 个 `NewInvoker` 的 `base := ai.BuildBaseEnv(); inv := &Invoker{... baseEnv: ai.AppendExtraEnv(base, extraEnv) ...}` 模板用一个 helper 替代。结合 Phase 6 的"BuildBaseEnv 只建一次"目标。

### Task 3.1：新增 `ai.NewBaseEnv(extraEnv)` helper

**Files:**
- Modify: `internal/ai/process.go`

- [ ] **Step 1：新增 helper**

在 `process.go` 中 `BuildBaseEnv` 之后：

```go
// NewBaseEnv combines BuildBaseEnv with non-empty entries from extraEnv,
// returning a properly clipped slice safe for concurrent appends. It is the
// canonical way to build the static portion of an engine subprocess env.
func NewBaseEnv(extraEnv map[string]string) []string {
	return AppendExtraEnv(BuildBaseEnv(), extraEnv)
}
```

- [ ] **Step 2：把 4 个 NewInvoker 改为调用它**

每个 invoker 的构造函数从：

```go
base := ai.BuildBaseEnv()
return &Invoker{binary: binary, baseEnv: ai.AppendExtraEnv(base, extraEnv)}
```

改为：

```go
return &Invoker{binary: binary, baseEnv: ai.NewBaseEnv(extraEnv)}
```

涉及文件：
- `internal/ai/claude/invoker.go`
- `internal/ai/codex/invoker.go`
- `internal/ai/kimi/invoker.go`
- `internal/ai/pi/invoker.go`

- [ ] **Step 3：测试**

```bash
go build ./...
go test ./internal/ai/...
```

- [ ] **Step 4：Commit**

```bash
git add internal/ai/process.go internal/ai/claude/invoker.go internal/ai/codex/invoker.go \
        internal/ai/kimi/invoker.go internal/ai/pi/invoker.go
git commit -m "refactor(ai): centralise invoker base env construction"
```

---

## Phase 4 — Adapter 统一（一.1 + 一.6 + 三.1）

目标：4 个 adapter 的 `Run` 和 `CollectTokenUsage` 模板用一个嵌入式 `BaseAdapter` 收敛。同时把 `RunResult.ExtractResult` 从 `func(logPath string) string` 改为无参 `func() string`，消除 logPath 的 leaky abstraction。

### Task 4.1：把 `RunResult.ExtractResult` 改为无参 closure

牵涉调用方：`worker/execution.go`、`bee/feeder.go`、`dynamic_test.go`。

**Files:**
- Modify: `internal/ai/contracts.go`
- Modify: `internal/ai/claude/adapter.go`、`codex/adapter.go`、`kimi/adapter.go`、`pi/adapter.go`
- Modify: `internal/ai/dynamic_test.go`
- Modify: `internal/domain/worker/execution.go`
- Modify: `internal/domain/bee/feeder.go`
- Modify: `internal/ai/dynamic.go`（如果有 ExtractResult 使用）

- [ ] **Step 1：先改 contracts.go**

```go
// RunResult is the handle returned from EngineAdapter.Run.
type RunResult struct {
	Process       Process
	Output        <-chan Output
	ExtractResult func() string
}

func NewRunResult(proc Process, out <-chan Output, err error, extract func() string) (RunResult, error) {
	if err != nil {
		return RunResult{}, err
	}
	return RunResult{Process: proc, Output: out, ExtractResult: extract}, nil
}
```

- [ ] **Step 2：改 4 个 adapter 的 Run，把 logPath 闭包绑定到 extractor**

以 `claude/adapter.go` 为例。改前（第 41-45 行）：

```go
func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := a.invoker.Run(ctx, workDir, prompt, opts, logPath)
	return ai.NewRunResult(proc, out, err, ExtractResultFromLog)
}
```

改后：

```go
func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := a.invoker.Run(ctx, workDir, prompt, opts, logPath)
	return ai.NewRunResult(proc, out, err, func() string {
		return ExtractResultFromLog(logPath)
	})
}
```

codex/kimi/pi 同样改。

- [ ] **Step 3：改调用方**

`internal/domain/worker/execution.go:104, 109, 123`：

```bash
# 替换 runRes.ExtractResult(logPath) → runRes.ExtractResult()
```

`internal/domain/bee/feeder.go:258`：同样。

`internal/ai/dynamic_test.go:74`：

```go
if got := res.ExtractResult("/log"); got != "a-result" {
```

改为：

```go
if got := res.ExtractResult(); got != "a-result" {
```

且测试里造 RunResult 的地方也要把 ExtractResult 改成 `func() string { return "a-result" }`。先看完整文件：

```bash
cat internal/ai/dynamic_test.go
```

逐处修。

- [ ] **Step 4：构建 & 测试**

```bash
go build ./...
go test ./...
```

预期：全绿。

- [ ] **Step 5：Commit**

```bash
git add internal/ai/contracts.go \
        internal/ai/claude/adapter.go internal/ai/codex/adapter.go \
        internal/ai/kimi/adapter.go internal/ai/pi/adapter.go \
        internal/ai/dynamic_test.go \
        internal/domain/worker/execution.go internal/domain/bee/feeder.go
git commit -m "refactor(ai): make ExtractResult close over logPath"
```

---

### Task 4.2：新增 `ai.BaseAdapter` 嵌入式骨架

**Files:**
- Create: `internal/ai/base_adapter.go`
- Create: `internal/ai/base_adapter_test.go`

- [ ] **Step 1：定义类型与接口**

```go
package ai

import "context"

// EngineInvoker is the minimal subprocess launcher contract every engine implements.
type EngineInvoker interface {
	Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath string) (Process, <-chan Output, error)
}

// EngineCollector is the minimal token-usage reader contract.
type EngineCollector interface {
	Collect(ctx context.Context, sessionID string) ([]TokenUsage, error)
}

// BaseAdapter implements the parts of EngineAdapter that are identical across
// all engines: Run wires the invoker output into a RunResult; CollectTokenUsage
// delegates to the collector. Engines embed BaseAdapter and supply their own
// Prepare hook.
type BaseAdapter struct {
	Invoker   EngineInvoker
	Collector EngineCollector
	// Extract is the per-engine result extractor (e.g. claude.ExtractResultFromLog).
	Extract func(logPath string) string
}

// Run launches the invoker and binds Extract to logPath in the returned RunResult.
func (b *BaseAdapter) Run(ctx context.Context, workDir, prompt string,
	opts RunOptions, logPath string) (RunResult, error) {
	proc, out, err := b.Invoker.Run(ctx, workDir, prompt, opts, logPath)
	return NewRunResult(proc, out, err, func() string { return b.Extract(logPath) })
}

func (b *BaseAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]TokenUsage, error) {
	return b.Collector.Collect(ctx, sessionID)
}

// Prepare default-impl: no-op. Engines override.
func (b *BaseAdapter) Prepare(string, PrepareOptions) error { return nil }
```

- [ ] **Step 2：写单元测试 `base_adapter_test.go`**

```go
package ai_test

import (
	"context"
	"errors"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

type fakeInvoker struct {
	proc ai.Process
	ch   <-chan ai.Output
	err  error
}

func (f *fakeInvoker) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return f.proc, f.ch, f.err
}

type fakeCollector struct {
	usages []ai.TokenUsage
	err    error
}

func (f *fakeCollector) Collect(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return f.usages, f.err
}

func TestBaseAdapter_RunBindsExtract(t *testing.T) {
	ch := make(chan ai.Output)
	close(ch)
	got := ""
	b := &ai.BaseAdapter{
		Invoker:   &fakeInvoker{ch: ch},
		Collector: &fakeCollector{},
		Extract:   func(logPath string) string { got = logPath; return "x" },
	}
	res, err := b.Run(context.Background(), "/wd", "p", ai.RunOptions{}, "/the/log")
	if err != nil {
		t.Fatal(err)
	}
	if r := res.ExtractResult(); r != "x" {
		t.Errorf("got %q", r)
	}
	if got != "/the/log" {
		t.Errorf("logPath not bound; got %q", got)
	}
}

func TestBaseAdapter_RunPropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	b := &ai.BaseAdapter{
		Invoker:   &fakeInvoker{err: wantErr},
		Collector: &fakeCollector{},
		Extract:   func(string) string { return "" },
	}
	_, err := b.Run(context.Background(), "/wd", "", ai.RunOptions{}, "/log")
	if !errors.Is(err, wantErr) {
		t.Errorf("want wantErr, got %v", err)
	}
}

func TestBaseAdapter_PrepareIsNoop(t *testing.T) {
	b := &ai.BaseAdapter{}
	if err := b.Prepare("/wd", ai.PrepareOptions{}); err != nil {
		t.Error(err)
	}
}
```

- [ ] **Step 3：跑测试**

```bash
go test ./internal/ai/ -run TestBaseAdapter -v
```

- [ ] **Step 4：Commit**

```bash
git add internal/ai/base_adapter.go internal/ai/base_adapter_test.go
git commit -m "feat(ai): add BaseAdapter for engine adapter skeleton sharing"
```

---

### Task 4.3：让 codex / kimi / pi 用 `BaseAdapter`（无 Prepare 覆盖）

**Files:**
- Modify: `internal/ai/codex/adapter.go`
- Modify: `internal/ai/kimi/adapter.go`
- Modify: `internal/ai/pi/adapter.go`

- [ ] **Step 1：codex/adapter.go**

改后整个文件：

```go
package codex

import (
	"fmt"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register(ai.EngineCodex, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineCodex), cfg.ExtraEnv())
	})
}

func NewAdapter(binaryPath string, extraEnv map[string]string) (ai.EngineAdapter, error) {
	store, err := NewSessionStore()
	if err != nil {
		return nil, fmt.Errorf("init codex session store: %w", err)
	}
	return &ai.BaseAdapter{
		Invoker:   NewInvoker(binaryPath, store, extraEnv),
		Collector: NewCollector(),
		Extract:   ExtractResultFromLog,
	}, nil
}
```

注意：原 `codexAdapter` 类型整体删除。

- [ ] **Step 2：kimi/adapter.go**

```go
package kimi

import (
	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register(ai.EngineKimi, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineKimi), cfg.ExtraEnv()), nil
	})
}

func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter {
	return &ai.BaseAdapter{
		Invoker:   NewInvoker(binaryPath, extraEnv),
		Collector: NewCollector(),
		Extract:   ExtractResultFromLog,
	}
}
```

- [ ] **Step 3：pi/adapter.go**

```go
package pi

import (
	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register(ai.EnginePi, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EnginePi), cfg.ExtraEnv())
	})
}

func NewAdapter(binaryPath string, extraEnv map[string]string) (ai.EngineAdapter, error) {
	inv, err := NewInvoker(binaryPath, extraEnv)
	if err != nil {
		return nil, err
	}
	return &ai.BaseAdapter{
		Invoker:   inv,
		Collector: NewCollector(),
		Extract:   ExtractResultFromLog,
	}, nil
}
```

- [ ] **Step 4：让 `*Invoker` 实现 `ai.EngineInvoker`**

`EngineInvoker.Run` 返回 `(ai.Process, <-chan ai.Output, error)`，所有现有 `*Invoker.Run` 签名匹配（核对一遍）。无需改 invoker 本身。

也要让 `*Collector` 实现 `ai.EngineCollector`，即有 `Collect(ctx, sessionID) ([]ai.TokenUsage, error)`。核对：

- claude/token_usage.go:53 ✅
- codex/token_usage.go:90 ✅
- kimi/token_usage.go:46 ✅
- pi/token_usage.go:49 ✅

- [ ] **Step 5：测试**

```bash
go build ./...
go test ./internal/ai/...
```

如有测试断言 `*codexAdapter`/`*kimiAdapter`/`*piAdapter` 类型存在，改成接口断言。

- [ ] **Step 6：Commit**

```bash
git add internal/ai/codex/adapter.go internal/ai/kimi/adapter.go internal/ai/pi/adapter.go
git commit -m "refactor(ai): adopt BaseAdapter in codex/kimi/pi"
```

---

### Task 4.4：让 claude 用 `BaseAdapter`（带自定义 Prepare）

Claude 的 Prepare 不是 no-op：它要清理 `.openbee.md` 和 `CLAUDE.md` 中的 import line。需要自定义类型嵌入 BaseAdapter 并覆盖 Prepare。

**Files:**
- Modify: `internal/ai/claude/adapter.go`

- [ ] **Step 1：重写 adapter.go**

```go
package claude

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)

const (
	systemRulesFile = ".openbee.md"
	importLine      = "@" + systemRulesFile
)

func init() {
	ai.Register(ai.EngineClaude, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineClaude), cfg.ExtraEnv()), nil
	})
}

type claudeAdapter struct {
	*ai.BaseAdapter
}

func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter {
	return &claudeAdapter{
		BaseAdapter: &ai.BaseAdapter{
			Invoker:   NewInvoker(binaryPath, extraEnv),
			Collector: NewCollector(),
			Extract:   ExtractResultFromLog,
		},
	}
}

func (a *claudeAdapter) Prepare(workDir string, _ ai.PrepareOptions) error {
	rulesPath := filepath.Join(workDir, systemRulesFile)
	if err := os.Remove(rulesPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", systemRulesFile, err)
	}
	return removeImportLine(workDir)
}

func removeImportLine(workDir string) error {
	claudePath := filepath.Join(workDir, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read CLAUDE.md: %w", err)
	}

	target := []byte(importLine)
	lines := bytes.Split(data, []byte("\n"))
	out := lines[:0]
	for _, line := range lines {
		if !bytes.Equal(bytes.TrimRight(line, "\r"), target) {
			out = append(out, line)
		}
	}
	cleaned := bytes.Join(out, []byte("\n"))
	if bytes.Equal(cleaned, data) {
		return nil
	}
	return os.WriteFile(claudePath, cleaned, 0o644)
}

// Run is inherited from BaseAdapter; no need to redeclare since *claudeAdapter
// embeds *ai.BaseAdapter and Go promotes the method.

// context is imported above for the interface signature; keep it even if unused
// directly in this file.
var _ context.Context = nil
```

> **注意**：`context` import 在文件里没直接使用，但保留 `var _ context.Context = nil` 是丑陋的 workaround。**更好方案**：测试编译会告诉我们是否真的需要这个 import，能删则删。先删掉这两行（包括 import 和 var _），运行 `go build`，按编译器提示再决定。

- [ ] **Step 2：测试**

```bash
go test ./internal/ai/claude/...
go test ./...
```

- [ ] **Step 3：Commit**

```bash
git add internal/ai/claude/adapter.go
git commit -m "refactor(ai/claude): adopt BaseAdapter with custom Prepare"
```

---

## Phase 5 — provider.go 表驱动与 env-key 常量（三.3 + 三.5）

`internal/ai/claude/provider.go` 是 ~400 行的巨型 switch + env builder。表驱动可砍掉 ~50% LOC。

### Task 5.1：抽取 env-key 常量块

**Files:**
- Modify: `internal/ai/claude/provider.go`

- [ ] **Step 1：在文件顶部新增**

在 `import` 之后、`var ErrInterrupted` 之前插入：

```go
const (
	envAnthropicAuthToken           = "ANTHROPIC_AUTH_TOKEN"
	envAnthropicAPIKey              = "ANTHROPIC_API_KEY"
	envAnthropicBaseURL             = "ANTHROPIC_BASE_URL"
	envAnthropicModel               = "ANTHROPIC_MODEL"
	envAnthropicSmallFastModel      = "ANTHROPIC_SMALL_FAST_MODEL"
	envAnthropicDefaultSonnetModel  = "ANTHROPIC_DEFAULT_SONNET_MODEL"
	envAnthropicDefaultOpusModel    = "ANTHROPIC_DEFAULT_OPUS_MODEL"
	envAnthropicDefaultHaikuModel   = "ANTHROPIC_DEFAULT_HAIKU_MODEL"
	envClaudeCodeSubagentModel      = "CLAUDE_CODE_SUBAGENT_MODEL"
	envEnableToolSearch             = "ENABLE_TOOL_SEARCH"
	envAPITimeoutMS                 = "API_TIMEOUT_MS"
	envClaudeCodeDisableNonessential = "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"
)
```

- [ ] **Step 2：把所有 env builder 函数里的字符串字面量替换成常量**

涉及：`kimiCodeEnv`、`moonshotEnv`、`glmEnv`、`minimaxEnv`、`deepseekEnv`、`standardProviderEnv`、`aliyunEnv`、`volcengineEnv`、`tencentEnv`、`mimoEnv`、`customEnv`。

- [ ] **Step 3：把 `providerEnvKeys` 改为引用常量**

```go
var providerEnvKeys = []string{
	envAnthropicAuthToken,
	envAnthropicAPIKey,
	// ... 全部 ...
}
```

- [ ] **Step 4：构建 & 测试**

```bash
go build ./...
go test ./internal/ai/claude/...
```

- [ ] **Step 5：Commit**

```bash
git add internal/ai/claude/provider.go
git commit -m "refactor(ai/claude): extract Anthropic env keys into constants"
```

---

### Task 5.2：表驱动 ConfigureProvider

**Files:**
- Modify: `internal/ai/claude/provider.go`

把 9 个 case 分支抽到一个 `providerSpec` 切片。每个 spec 表达：display name、key prompt i18n key、env builder（可能需要 model 选项）、需要 .claude.json、可选 model 选项列表 & 默认值、可选自定义 baseURL+key 两段输入。

- [ ] **Step 1：先定义 spec 类型**

在 `func promptAPIKey` 之上：

```go
type providerSpec struct {
	Name           string
	KeyPrompt      string // i18n key for survey prompt
	BuildEnv       func(apiKey, model string) map[string]string
	NeedClaudeJSON bool
	ModelOptions   []string
	ModelDefault   string
	// Custom is set for the two providers that need (baseURL, apiKey) instead of
	// (apiKey, model): ProviderMimo and ProviderCustom.
	BaseURLPrompt string // empty unless dual-prompt
}
```

- [ ] **Step 2：定义全部 spec**

```go
var providerSpecs = []providerSpec{
	{Name: ProviderKimiCode, KeyPrompt: "KeyKimiCode", BuildEnv: func(k, _ string) map[string]string { return kimiCodeEnv(k) }},
	{Name: ProviderMoonshot, KeyPrompt: "KeyMoonshot", BuildEnv: func(k, _ string) map[string]string { return moonshotEnv(k) }},
	{Name: ProviderDeepSeek, KeyPrompt: "KeyDeepSeek", BuildEnv: func(k, _ string) map[string]string { return deepseekEnv(k) }},
	{Name: ProviderGLM, KeyPrompt: "KeyGLM", BuildEnv: func(k, _ string) map[string]string { return glmEnv(k) }, NeedClaudeJSON: true},
	{Name: ProviderMiniMax, KeyPrompt: "KeyMiniMax", BuildEnv: func(k, _ string) map[string]string { return minimaxEnv(k) }, NeedClaudeJSON: true},
	{
		Name: ProviderAliyun, KeyPrompt: "KeyAliyun",
		BuildEnv:     aliyunEnv,
		ModelOptions: []string{"qwen3.5-plus", "kimi-k2.5", "glm-5", "MiniMax-M2.5"},
		ModelDefault: "qwen3.5-plus",
	},
	{
		Name: ProviderVolcengine, KeyPrompt: "KeyVolcengine",
		BuildEnv:       volcengineEnv,
		NeedClaudeJSON: true,
		ModelOptions:   []string{"doubao-seed-2.0-code", "doubao-seed-2.0-pro", "doubao-seed-2.0-lite", "doubao-seed-code", "minimax-m2.5", "glm-4.7", "deepseek-v3.2", "kimi-k2.5"},
		ModelDefault:   "doubao-seed-2.0-code",
	},
	{
		Name: ProviderTencent, KeyPrompt: "KeyTencent",
		BuildEnv:       tencentEnv,
		NeedClaudeJSON: true,
		ModelOptions:   []string{"tc-code-latest（auto）", "hunyuan-2.0-instruct", "hunyuan-2.0-thinking", "minimax-m2.5", "kimi-k2.5", "glm-5", "hunyuan-t1", "hunyuan-turbos"},
		ModelDefault:   "tc-code-latest（auto）",
	},
	{Name: ProviderMimo, KeyPrompt: "KeyMimoToken", BuildEnv: func(k, b string) map[string]string { return mimoEnv(b, k) }, NeedClaudeJSON: true, BaseURLPrompt: "KeyMimoURL"},
	{Name: ProviderCustom, KeyPrompt: "KeyCustomToken", BuildEnv: func(k, b string) map[string]string { return customEnv(b, k) }, BaseURLPrompt: "KeyCustomURL"},
}
```

- [ ] **Step 3：把 ConfigureProvider 改为查表 + 一段通用流程**

```go
func ConfigureProvider() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeJSONPath := filepath.Join(home, ".claude.json")

	if _, err := os.Stat(settingsPath); err == nil {
		var skip bool
		if err := survey.AskOne(&survey.Confirm{
			Message: i18n.M.Provider.FoundSettings,
			Default: true,
		}, &skip); err != nil {
			return HandleSurveyErr(err)
		}
		if skip {
			return nil
		}
	}

	names := make([]string, len(providerSpecs))
	specByName := make(map[string]*providerSpec, len(providerSpecs))
	for i := range providerSpecs {
		names[i] = providerSpecs[i].Name
		specByName[providerSpecs[i].Name] = &providerSpecs[i]
	}

	var providerName string
	if err := survey.AskOne(&survey.Select{
		Message: i18n.M.Provider.Select,
		Options: names,
	}, &providerName); err != nil {
		return HandleSurveyErr(err)
	}

	spec, ok := specByName[providerName]
	if !ok {
		return fmt.Errorf("unknown provider: %s", providerName)
	}

	var baseURL string
	if spec.BaseURLPrompt != "" {
		v, err := promptAPIKey(i18n.M.Provider.Field(spec.BaseURLPrompt))
		if err != nil {
			return err
		}
		baseURL = v
	}

	apiKey, err := promptAPIKey(i18n.M.Provider.Field(spec.KeyPrompt))
	if err != nil {
		return err
	}

	var model string
	if len(spec.ModelOptions) > 0 {
		if err := survey.AskOne(&survey.Select{
			Message: i18n.M.Provider.SelectModel,
			Options: spec.ModelOptions,
			Default: spec.ModelDefault,
		}, &model); err != nil {
			return HandleSurveyErr(err)
		}
	} else if baseURL != "" {
		model = baseURL // pass baseURL via the second arg for Mimo/Custom builders
	}

	env := spec.BuildEnv(apiKey, model)

	if err := mergeClaudeSettings(settingsPath, env); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}
	fmt.Println(i18n.M.Provider.WrittenSettings)

	if spec.NeedClaudeJSON {
		if err := mergeClaudeJSON(claudeJSONPath); err != nil {
			return fmt.Errorf("write .claude.json: %w", err)
		}
		fmt.Println(i18n.M.Provider.WrittenJSON)
	}
	return nil
}
```

> **关键点**：`i18n.M.Provider.Field(name)` 需要 `internal/infra/i18n` 包提供——如果不存在该方法，**不要新增 i18n 接口**。改为在 spec 里直接持有 `func() string { return i18n.M.Provider.KeyKimiCode }` 或 `*string` 指向 i18n 字段。改 spec 字段为：
>
> ```go
> KeyPromptFn func() string
> ```
>
> 然后每个 spec 写 `KeyPromptFn: func() string { return i18n.M.Provider.KeyKimiCode }`。**调研 i18n.M.Provider 结构后选合适方案**。

- [ ] **Step 4：手动测试两到三个 provider 分支**

```bash
go run ./cmd/openbee provider configure   # （或对应实际命令）
```

确认交互体验未变。

- [ ] **Step 5：单元测试 + 构建**

```bash
go build ./...
go test ./internal/ai/claude/...
```

- [ ] **Step 6：Commit**

```bash
git add internal/ai/claude/provider.go
git commit -m "refactor(ai/claude): table-drive ConfigureProvider"
```

---

## Phase 6 — 效率修复（二.1 ~ 二.6）

### Task 6.1：Claude 日志双扫缓存（二.1）

claude `Run` 完成时调 `scanResultLog`，调用方通过 `RunResult.ExtractResult` 再扫一次。把第一次扫描的结果用 `sync.Once` 或闭包变量缓存。

经过 Phase 4 改造后，`ExtractResult` 是 `func() string` 闭包，可以让 claude 的闭包返回缓存值。

**Files:**
- Modify: `internal/ai/claude/adapter.go`
- Modify: `internal/ai/claude/invoker.go`

> **设计选择**：Phase 4 的 `BaseAdapter.Run` 把 `Extract` 设为 `func(logPath string) string`，每次调用都重新扫。Claude 需要"运行完成时扫一次，结果共享给调用方"。所以 Claude **不能用纯 BaseAdapter**——它已在 Phase 4 Task 4.4 走自定义 Prepare 但 Run 走 BaseAdapter；这里要再覆盖 Run。

- [ ] **Step 1：在 claude/invoker.go 让 Invoker.Run 返回缓存版 result**

修改 invoker 让它把 `scanResultLog` 的输出缓存到一个 `*atomic.Pointer[string]` 或在第一次完成时回填。但 Invoker.Run 当前返回 `(Process, <-chan Output, error)`，不包含 "result"。

**方案 A（不变 Invoker 签名）**：在 claude adapter 覆盖 `Run`，自己起 goroutine 转发 `<-chan Output`，并在 OutputDone/OutputError 之前把扫描结果缓存到一个共享变量；返回的 ExtractResult 闭包直接读这个变量。

```go
func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := a.BaseAdapter.Invoker.Run(ctx, workDir, prompt, opts, logPath)
	if err != nil {
		return ai.RunResult{}, err
	}

	var (
		mu        sync.Mutex
		cached    string
		cachedSet bool
	)
	getResult := func() string {
		mu.Lock()
		defer mu.Unlock()
		if !cachedSet {
			cached = ExtractResultFromLog(logPath)
			cachedSet = true
		}
		return cached
	}

	// Forward channel; 由于 Invoker.Run 在 goroutine 里写 OutputError 时已经调用过
	// scanResultLog（claude/invoker.go:129），所以这里不需要重复扫描。但调用方拿到
	// 的 ExtractResult 是 cached 函数 —— 第一次扫描发生在调用方调用时，避免在 invoker
	// 内提前扫。要进一步优化让 invoker 直接共享结果，需扩展 Invoker 返回值，下次再做。
	return ai.RunResult{Process: proc, Output: out, ExtractResult: getResult}, nil
}
```

> 实际上原先 `claude/invoker.go:129` 就在 invoker 里扫了一次（为了检错 isError）。**真正的优化**是把那次扫描结果通过额外机制（如 `Invoker` 返回 4 元组、或 invoker.Run 写到调用方传入的 `*string`）共享给 adapter。简单方案如下：

**方案 B（推荐）**：扩展 `Invoker.Run` 把扫描结果写入调用方传入的 `*string`。但这破坏了 EngineInvoker 接口。

**方案 C（折中）**：保留 invoker 内部扫描（用于检错），adapter 端用 `sync.Once` 包装 ExtractResult，让多次调用 ExtractResult 不重复扫。

选 **方案 C** 实施：

- [ ] **Step 2：用 sync.Once 包装 ExtractResult**

```go
import "sync"

// adapter 自定义 Run：
func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := a.BaseAdapter.Invoker.Run(ctx, workDir, prompt, opts, logPath)
	if err != nil {
		return ai.RunResult{}, err
	}
	var (
		once   sync.Once
		result string
	)
	return ai.RunResult{
		Process: proc,
		Output:  out,
		ExtractResult: func() string {
			once.Do(func() { result = ExtractResultFromLog(logPath) })
			return result
		},
	}, nil
}
```

这至少让**调用方**多次调用 `ExtractResult` 只扫一次。仍存在"invoker 内部扫一次 + adapter 扫一次"问题。在 `worker/execution.go` 调用 3 次 `runRes.ExtractResult(logPath)` 的场景，从 1+3=4 次扫描降到 1+1=2 次。**够本期可接受**。

更激进的优化（让 invoker 把结果传出去）作为 follow-up 留 TODO 在代码注释里。

- [ ] **Step 3：测试**

```bash
go test ./internal/ai/...
```

- [ ] **Step 4：Commit**

```bash
git add internal/ai/claude/adapter.go
git commit -m "perf(ai/claude): cache ExtractResult output across repeated calls"
```

---

### Task 6.2：Kimi 双 unmarshal 改用 prefix 检查（二.2）

**Files:**
- Modify: `internal/ai/kimi/invoker.go`

- [ ] **Step 1：在 ExtractResultFromLog 里加 prefix 判断**

定位 `kimi/invoker.go:106-126` 的 content 解析段：

改前：

```go
if len(msg.Content) == 0 {
	return true
}
var s string
if json.Unmarshal(msg.Content, &s) == nil && s != "" {
	if !strings.HasPrefix(s, "(Empty response:") {
		lastText = s
	}
	return true
}
var blocks []kimiContentBlock
if json.Unmarshal(msg.Content, &blocks) != nil {
	return true
}
// ...
```

改后：

```go
if len(msg.Content) == 0 {
	return true
}
trimmed := bytes.TrimSpace(msg.Content)
if len(trimmed) == 0 {
	return true
}
if trimmed[0] == '"' {
	var s string
	if json.Unmarshal(msg.Content, &s) == nil && s != "" && !strings.HasPrefix(s, kimiEmptyPrefix) {
		lastText = s
	}
	return true
}
if trimmed[0] != '[' {
	return true
}
var blocks []kimiContentBlock
if json.Unmarshal(msg.Content, &blocks) != nil {
	return true
}
// ...
```

加 `import "bytes"`。

- [ ] **Step 2：跑测试**

```bash
go test ./internal/ai/kimi/...
```

确保 `invoker_test.go` 用例覆盖了 string content 与 array content 两条分支；若没有，补两条用例。

- [ ] **Step 3：Commit**

```bash
git add internal/ai/kimi/invoker.go
git commit -m "perf(ai/kimi): branch content parsing by prefix to avoid double unmarshal"
```

---

### Task 6.3：Pi stripThinkingSignature 字节级实现（二.3）

`stripThinkingSignature` 当前对**每行 stdout** 跑 `json.Unmarshal` → 遍历 → 重新 `json.Marshal`，命中时还递归处理每个 message/content 块。在直播 stdout 上是热路径。

**目标**：保持现有"删 `thinkingSignature` 字段"语义，但减少 marshal 次数。

策略：先用 `bytes.Contains` 早早返回（已有），命中时**只对包含 thinkingSignature 的最内层 JSON 对象**做 unmarshal+marshal，外层用字节拼接。但 JSON 字段顺序与转义复杂，纯字节级删除一个键的可靠实现需要 JSON 词法分析。

**实用折中**：使用 `json.Decoder` 流式跳过，不构建中间 map。这仍然有解析开销但避免了构建 + 重序列化整棵树。

> 实际上现有实现已经做了短路（`bytes.Contains` 不命中直接返回原 line），是命中时才慢。命中频率有多高？取决于 pi 行为。**先 benchmark 一下命中时的具体开销**。

- [ ] **Step 1：写 benchmark `pi/invoker_bench_test.go`**

```go
package pi

import (
	"strings"
	"testing"
)

func BenchmarkStripThinkingSignature_NoMatch(b *testing.B) {
	line := []byte(`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}`)
	b.ResetBytes(int64(len(line)))
	for i := 0; i < b.N; i++ {
		stripThinkingSignature(line)
	}
}

func BenchmarkStripThinkingSignature_OneMessageOneBlock(b *testing.B) {
	body := `{"type":"thinking","thinkingSignature":"` + strings.Repeat("x", 256) + `","thinking":"some"}`
	line := []byte(`{"type":"message_end","message":{"role":"assistant","content":[` + body + `]}}`)
	b.ResetBytes(int64(len(line)))
	for i := 0; i < b.N; i++ {
		stripThinkingSignature(line)
	}
}

func BenchmarkStripThinkingSignature_AgentEndManyMessages(b *testing.B) {
	body := `{"type":"thinking","thinkingSignature":"` + strings.Repeat("x", 256) + `","thinking":"some"}`
	msg := `{"role":"assistant","content":[` + body + `]}`
	var msgs []string
	for i := 0; i < 8; i++ {
		msgs = append(msgs, msg)
	}
	line := []byte(`{"type":"agent_end","messages":[` + strings.Join(msgs, ",") + `]}`)
	b.ResetBytes(int64(len(line)))
	for i := 0; i < b.N; i++ {
		stripThinkingSignature(line)
	}
}
```

- [ ] **Step 2：跑 bench 基线**

```bash
go test -bench=BenchmarkStripThinkingSignature -benchmem -run=^$ ./internal/ai/pi/
```

记录每次操作的 ns/op 和 B/op。

- [ ] **Step 3：评估是否值得优化**

如果 `OneMessageOneBlock` < 10μs/op，**直接跳过 6.3**（用户接受度足够，再写更复杂代码得不偿失）。

如果远超那个水平，考虑用 `jsonparser`（外部包，不引入）或自己写一个简单的"在 map 反序列化前先用 json.Valid 过滤 + bytes.Replace 替换 `"thinkingSignature":"...","` 段"。后者风险点：值可能含转义引号，需要正确处理。

**保守做法**：把现有 `stripThinkingSignatureFromMessage` 改用 `json.NewDecoder(bytes.NewReader(...))` 流式跳过，写到 `bytes.Buffer`，可避免一次完整 map[string]json.RawMessage 构建。

**本计划暂停在"跑 bench 看必要性"。** 如确认必要再写 Task 6.3.4 实现。

- [ ] **Step 4：决策记录**

把 bench 结果写到 commit 信息或 PR 描述里。

- [ ] **Step 5：Commit（无论是否优化）**

```bash
git add internal/ai/pi/invoker_bench_test.go
git commit -m "test(ai/pi): add benchmarks for stripThinkingSignature hot path"
```

---

### Task 6.4：Codex pipe 早关（二.4）

`codex/invoker.go:115-156` 用 `io.MultiWriter(logFile, pw)` 让 stdout 双写到 logfile + pipe；pipe 读端 `extractSessionID` 只需读到第一行 `thread.started`，但读完后 `io.Copy(io.Discard, pr)` 把剩余 stdout 全部 drain（双写整生命周期）。

优化：`extractSessionID` 返回后立刻让 stdout 不再写到 pipe，直接走 logfile。

**Files:**
- Modify: `internal/ai/codex/invoker.go`

- [ ] **Step 1：把 MultiWriter 换为切换式 writer**

设计：用一个自定义 `switchableWriter`，初始写 logfile + pipe，`extractSessionID` 完成后调用 `Detach()` 改为只写 logfile，并关闭 pipe 写端。

```go
type switchableWriter struct {
	mu     sync.Mutex
	main   io.Writer
	branch io.Writer
}

func (s *switchableWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	branch := s.branch
	s.mu.Unlock()
	n, err := s.main.Write(p)
	if branch != nil {
		_, _ = branch.Write(p[:n])
	}
	return n, err
}

func (s *switchableWriter) Detach() {
	s.mu.Lock()
	s.branch = nil
	s.mu.Unlock()
}
```

> **风险**：`cmd.Stdout` 在 cmd.Wait 完成前可能被并发写。`switchableWriter.Write` 用 lock 保护 branch 字段，主 write 不加锁（因为 io.Writer 不要求线程安全且本身只有一个 stdout writer）。

- [ ] **Step 2：改写 Run goroutine**

```go
pr, pw := io.Pipe()
writer := &switchableWriter{main: logFile, branch: pw}

cmd.Stdout = writer
// ...

go func() {
	defer close(ch)
	defer logFile.Close()

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Wait()
		writer.Detach()
		pw.Close()
	}()

	if !resume {
		if newThreadID := extractSessionID(pr); newThreadID != "" {
			if err := inv.store.Set(opts.SessionID, newThreadID); err != nil {
				log.Error("store codex session", zap.String("uuid", opts.SessionID), zap.Error(err))
			}
			writer.Detach() // stop branching to pipe early; pipe writes become no-op
		}
	}
	io.Copy(io.Discard, pr) // drain any remaining bytes after detach

	if err := <-doneCh; err != nil {
		ch <- ai.Output{Type: ai.OutputError, Content: err.Error()}
	} else {
		ch <- ai.Output{Type: ai.OutputDone}
	}
}()
```

- [ ] **Step 3：测试**

```bash
go test ./internal/ai/codex/...
```

确保 `invoker_test.go` 仍通过；如果需要更细的测试覆盖 pipe 提前关闭路径，补一条。

- [ ] **Step 4：Commit**

```bash
git add internal/ai/codex/invoker.go
git commit -m "perf(ai/codex): detach stdout pipe after thread_id extraction"
```

---

### Task 6.5：BuildBaseEnv 全局共享（二.5）

4 个 invoker 启动时各调一次 `BuildBaseEnv`，每次都 `os.Environ()` + PATH 重写。这是启动期一次性开销，但**毫秒级**。

**评估**：边际收益小，不值得引入 `sync.Once` 缓存（缓存破坏可测试性、增加状态）。**跳过 Task 6.5**。

- [ ] **Skip：在 commit message 或 plan summary 中记一笔"二.5 评估后跳过：每进程一次性开销可接受"**

---

### Task 6.6：sessionfile TOCTOU stat-then-open 修复（二.6）

`utils/sessionfile.go:40-46` `FindWithLegacyFast` 先 `os.Stat` 再 walk，stat 命中后返回路径让调用方 open。其实直接 `os.Open` 一次系统调用就能完成"检查+打开"，但当前 API 返回 path 而非 *os.File，签名修改牵涉调用方。

实际影响：legacy hit 时多一个 stat syscall。**收益微薄，且改 API 牵连四个 provider 测试**。

- [ ] **决策：跳过 Task 6.6（成本不划算）**

在 `sessionfile.go` 顶部加注释说明：

```go
// Note: FindWithLegacyFast performs an os.Stat probe before WalkDir, which is a
// TOCTOU pattern. We accept it because (a) callers re-open by path immediately
// after, and (b) the cost is a single fs syscall on the happy path.
```

- [ ] **Commit**

```bash
git add internal/utils/sessionfile/sessionfile.go
git commit -m "docs(sessionfile): document TOCTOU rationale on FindWithLegacyFast"
```

---

## Phase 7 — 验收

### Task 7.1：全量回归

- [ ] **Step 1：跑全部测试**

```bash
go test ./... -race -count=1
```

- [ ] **Step 2：lint**

```bash
go vet ./...
gofmt -l internal/ai
```

- [ ] **Step 3：构建 release binary smoke test**

```bash
go build -o /tmp/openbee ./cmd/openbee
/tmp/openbee --version
```

- [ ] **Step 4：手动跑一次 worker 任务**

确认 4 个引擎都能正常执行（或至少 claude 作为代表）。

- [ ] **Step 5：写完成总结**

把每个 Phase 的实际改动行数和测试结果汇总到 PR 描述。

---

## 自检（self-review）

**Spec coverage** — 三类共 18 个发现，本计划覆盖如下：

| 编号 | 描述 | 状态 |
|------|------|------|
| 一.1 | Adapter 重复 | Phase 4 |
| 一.2 | Invoker 构造重复 | Phase 3 |
| 一.3 | Run 子进程脚手架重复（claude/kimi）| Phase 2 |
| 一.4 | Token Collector 重复 | Phase 1 |
| 一.5 | env-or-default 三份 | Phase 1（kimi 跳过） |
| 一.6 | ExtractResultFromLog 模板 | 部分 Phase 4（顺便统一了 closure 形态，未抽 helper —— 因为每个 provider extract 函数体差异大）|
| 二.1 | Claude 日志双扫 | Phase 6 |
| 二.2 | Kimi 双 unmarshal | Phase 6 |
| 二.3 | Pi stripThinkingSignature | Phase 6（bench 后决定）|
| 二.4 | Codex pipe 整生命周期 | Phase 6 |
| 二.5 | BuildBaseEnv 重建 | 跳过（评估） |
| 二.6 | sessionfile TOCTOU | 跳过（评估） |
| 三.1 | ExtractResult 泄漏 logPath | Phase 4 |
| 三.2 | SystemRulesFile/ImportLine 位置 | Phase 0 |
| 三.3 | ConfigureProvider 嵌套 + stringly | Phase 5 |
| 三.4 | contracts.go DrainUsageMap | Phase 1 |
| 三.4 | CmdProcess.mu 噪声 | Phase 0 |
| 三.5 | event-type stringly | Phase 0 |
| 三.6 | 无用注释 | Phase 0 |

**Placeholder scan** — 无 TBD/TODO 留在执行步骤里。Phase 6.3 / 6.5 / 6.6 明确写"评估后决定/跳过"，是有意识的决策而非占位。

**Type consistency** — 检查跨 Task 的命名：
- `EngineInvoker` / `EngineCollector` 接口在 Phase 4 定义，4 个 provider 的 `*Invoker` / `*Collector` 已实现签名匹配。
- `ai.NewBaseEnv` 在 Phase 3 定义、被 Phase 3 内部使用。
- `ai.AggregateUsage[T]` 泛型签名 `(path string, fold func(T, map[string]*TokenUsage)) ([]TokenUsage, error)` 一致。
- `ai.SubprocessSpec` 字段在 Phase 2 定义并在 2.2 / 2.3 内引用。

**已识别的风险**：
1. Task 4.4 claude Prepare 嵌入 BaseAdapter 后又有 Run 覆盖（Task 6.1），需要 verify Go 嵌入式方法解析正确。
2. Task 5.2 i18n 字段访问方式（`i18n.M.Provider.Field(...)`）取决于 i18n 包是否提供动态访问；如不提供，按 plan 注释回退到 spec 持函数。
3. Task 6.4 `switchableWriter` 的并发性：cmd 的 stdout 写来自单一 goroutine（cmd 主进程），但 `Detach` 来自 wait goroutine，需要锁保护 branch。

---

## 执行选项

**Plan complete and saved to `docs/superpowers/plans/2026-05-11-internal-ai-cleanup.md`. Two execution options:**

**1. Subagent-Driven（推荐）** — 每个 Task 一个 fresh subagent，task 之间审查，反馈快。

**2. Inline Execution** — 当前会话内顺序执行，分 Phase 设 checkpoint。

请选择执行方式。
