# Session Prefix Refactor — Design

## 背景

`internal/ai/ai.go` 中四个函数协作生成 worker / bee 的会话起始 prompt：

- `WorkerPersona(name, description, constraints) string`
- `BuildWorkerSessionPrefix(persona string) string`
- `BuildBeeSessionPrefix() string`
- `writePrefixStep1(sb *strings.Builder, skillName string)`

实际调用现场：

```go
// internal/domain/task/dispatcher.go (executeFresh)
persona = ai.WorkerPersona(worker.Name, worker.Description, worker.Constraints)
prefix := ai.BuildWorkerSessionPrefix(persona)
d.manager.ExecuteWorker(ctx, task.WorkerID, prefix+instruction, sessionID, false)

// internal/domain/bee/feeder.go (drainSession)
prefix := ""
if !resume {
    prefix = ai.BuildBeeSessionPrefix()
}
prompt := buildPrompt(msgs, prefix)
```

## 问题

1. **prefix 这个中间产物被外露**。调用方需要先拿 prefix 字符串，再自己拼到 content 前面。`prefix + content` 的装配责任被推给了调用方。
2. **persona 与 worker prefix 拆成两个函数**。调用方需要先调 `WorkerPersona` 再调 `BuildWorkerSessionPrefix`，且要持有中间 persona 字符串。
3. **resume 分支泄漏在调用方**。bee 那边 `if !resume { prefix = ... }` 实质上是 ai 包的会话语义，却下沉到了 feeder。
4. **没有共享抽象**。Bee 和 Worker 两个 builder 是兄弟结构，但通过 `writePrefixStep1` 这种"对调用方 builder 做副作用"的 leaky helper 共享代码，而不是用 `Role` 类型驱动。
5. **`writePrefixStep1` 是 leaky helper**。它接收 `*strings.Builder` 直接写入调用方的 buffer，违反单向数据流。
6. **位置不对**。`internal/ai/ai.go` 顶层包定位为"对外契约"（`Role`、`EngineAdapter`、`RunOptions` 等），但 prompt 装配是**实现**，应该收敛到 `internal/ai/core`。

## 目标

- 调用方只传入 role / identity / resume / content，由 core 完成所有 prompt 装配。
- 不再向调用方暴露 "prefix 字符串" 这个中间概念。
- 角色差异（skill 名、Step 2 header、是否支持 persona）集中在一处配置表。
- Bee 消息装配的内部协议（`<message_meta>` / `<message_content>`）保留在 feeder，因为它依赖 `store.ClaimedMessage`，搬到 ai 包族会形成反向依赖。
- 实现层下沉到 `internal/ai/core`，与已有的 `BaseAdapter`、`SubprocessSpec`、`AggregateUsage` 等实现保持同一定位。

## 分层

| 包                       | 角色          | 此次新增内容                                                     |
|--------------------------|---------------|----------------------------------------------------------------|
| `internal/ai`            | 契约层         | 保留现有 `Role` / `RoleWorker` / `RoleBee`                       |
| `internal/ai/core`       | 实现层         | 新增 `SessionRequest`、`WorkerIdentity`、`BuildSessionPrompt` 及私有实现 |
| `internal/domain/task`   | 业务调用方     | 改为 import `core`，一次调用 `core.BuildSessionPrompt`            |
| `internal/domain/bee`    | 业务调用方     | 同上；`buildPrompt` 拆出 `assembleMessages`                       |

> 注意：core 的定位由此从"engine 执行机制层"扩展为"ai 包族整体实现层"。已有的 `BaseAdapter` 等也是面向 engine adapter 这个调用方的实现，本次新增 `BuildSessionPrompt` 是面向业务调用方（dispatcher / feeder）的实现，两者并列即可。

## 设计

### 1. core 包新增对外 API

新文件 `internal/ai/core/session.go`：

```go
package core

import (
    "github.com/theopenbee/openbee/internal/ai"
)

// WorkerIdentity 描述 worker 的身份信息，最终被嵌入到 <worker_persona> 块。
type WorkerIdentity struct {
    Name        string
    Description string
    Constraints string
}

// SessionRequest 是调用方告诉 core "我要起一个会话"的全部输入。
type SessionRequest struct {
    Role     ai.Role        // ai.RoleWorker / ai.RoleBee
    Identity WorkerIdentity // 仅 RoleWorker 有意义，零值表示无 persona
    Resume   bool           // true 表示复用已有 session，跳过前缀
    Content  string         // 业务内容（worker 的 instruction；bee 已装配好的消息段）
}

// BuildSessionPrompt 返回一个完整的 session prompt。
// 当 Resume 为 true 时直接返回 Content；否则根据 Role 在 Content 前装配
// Step 1 + persona（若适用）+ Step 2 header 的前缀。
func BuildSessionPrompt(req SessionRequest) string { ... }
```

### 2. core 包内部结构（私有，同样在 session.go）

```go
type sessionPrefixSpec struct {
    skillName   string
    step2Header string
    personaTag  string // 空串表示该角色不支持 persona
}

var rolePrefixSpecs = map[ai.Role]sessionPrefixSpec{
    ai.RoleWorker: {
        skillName:   "openbee-worker",
        step2Header: "## Step 2: Execute the task\n",
        personaTag:  "worker_persona",
    },
    ai.RoleBee: {
        skillName:   "openbee-bee",
        step2Header: "## Step 2: Handle the messages below\n",
        personaTag:  "",
    },
}

// 装配单个 session 前缀。
func buildSessionPrefix(role ai.Role, persona string) string { ... }

// 把 Identity 转成嵌入 <worker_persona> 的内容；零值返回 ""。
func (id WorkerIdentity) persona() string { ... }
```

`BuildSessionPrompt` 的内部流程：

1. 若 `req.Resume == true`，直接返回 `req.Content`。
2. 调 `id.persona()` 得到 persona 字符串（worker 用；bee 的零值 identity 返回 ""）。
3. 调 `buildSessionPrefix(req.Role, persona)` 拼出前缀。
4. 返回 `prefix + req.Content`。

### 3. 删除的旧 API（顶层 ai 包）

`internal/ai/ai.go` 中的 Section 5「Helper utilities」涉及 prompt 的部分全部删除：

- `WorkerPersona` —— 逻辑下沉为 core 包内 `(WorkerIdentity).persona()` 私有方法。
- `BuildWorkerSessionPrefix` —— 被 `core.BuildSessionPrompt` 取代。
- `BuildBeeSessionPrefix` —— 被 `core.BuildSessionPrompt` 取代。
- `writePrefixStep1` —— 被 core 包内 `buildSessionPrefix` 吸收。

> `EngineArgsMap` / `ParseEngineArgs` / `MergeEngineArgs` / `ParseEngineArgsJSON` / `splitCLIArgs` 等同 Section 5 的工具不在此次变动范围。

### 4. 调用方改动

**internal/domain/task/dispatcher.go (`executeFresh`)**

```go
import (
    "github.com/theopenbee/openbee/internal/ai"
    "github.com/theopenbee/openbee/internal/ai/core"
)

identity := core.WorkerIdentity{}
if d.workerLookup != nil {
    if worker == nil {
        return model.WorkerExecution{}, fmt.Errorf("worker %q not found", task.WorkerID)
    }
    identity = core.WorkerIdentity{
        Name: worker.Name, Description: worker.Description, Constraints: worker.Constraints,
    }
}
prompt := core.BuildSessionPrompt(core.SessionRequest{
    Role: ai.RoleWorker, Identity: identity, Content: instruction,
})
return d.manager.ExecuteWorker(ctx, task.WorkerID, prompt, sessionID, false)
```

**internal/domain/bee/feeder.go**

把原 `buildPrompt(msgs, prefix)` 拆成两步：

```go
import (
    "github.com/theopenbee/openbee/internal/ai"
    "github.com/theopenbee/openbee/internal/ai/core"
)

// 1. 域内的纯消息装配：仍然知道 ClaimedMessage 字段、<message_meta>/<message_content> 协议
messagesText := assembleMessages(msgs)

// 2. core 包负责前缀和 resume 判断
prompt := core.BuildSessionPrompt(core.SessionRequest{
    Role: ai.RoleBee, Resume: resume, Content: messagesText,
})
```

原 `buildPrompt` 拆分后变成 `assembleMessages(msgs []store.ClaimedMessage) string`，只处理消息部分。

**关于换行的行为保留**：
- 旧 worker 路径：`prefix + instruction`，prefix 末尾自带 `\n`（来自 step2Header），instruction 直接拼。新实现 `prefix + Content` 等价。
- 旧 bee 路径：`prefix + "\n" + <message>...`，prefix 自带 `\n` 后又额外加了一个 `\n`，即 Step 2 header 与首条 message 之间有一个空行。新实现里 `assembleMessages` 让首条消息前加一个 `\n`，把"空行"语义封装进 messages 段；这样 `BuildSessionPrompt` 始终用 `prefix + Content` 这一种拼法，无角色分支。

### 4a. 可选：worker resume 路径统一

`dispatcher.go` 的 `resolveExecution` 在 resume 命中时直接传 `instruction`：

```go
exec, err := d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, sessionID, true)
```

可以统一改为：

```go
prompt := core.BuildSessionPrompt(core.SessionRequest{
    Role: ai.RoleWorker, Resume: true, Content: instruction,
})
exec, err := d.manager.ExecuteWorker(ctx, task.WorkerID, prompt, sessionID, true)
```

由于 `Resume: true` 直接返回 `Content`，行为完全等价，但调用方的"是否前缀"决策完全交给 core 包。**建议做**：让所有 `ExecuteWorker` 的 prompt 都走同一个入口，未来变更（如 resume 时也要前缀）只改 core 包。

### 5. 测试调整

- 新增 `internal/ai/core/session_test.go`，以 `BuildSessionPrompt` 为入口：
  - `TestBuildSessionPrompt_Worker_WithPersona`：构造 `WorkerIdentity` 非零值的 `SessionRequest{Role: ai.RoleWorker}`，断言：包含 `## Step 1`、`openbee-worker`、`<worker_persona>` 包装、Identity 字段、`## Step 2: Execute the task`、以及末尾的 Content 段。
  - `TestBuildSessionPrompt_Worker_NoPersona`：`WorkerIdentity` 零值；断言无 `<worker_persona>` 块。
  - `TestBuildSessionPrompt_Bee`：`Role: ai.RoleBee`；断言含 `openbee-bee`、`## Step 2: Handle the messages below`、不含 `<worker_persona>`。
  - `TestBuildSessionPrompt_Resume`：`Resume: true`；断言返回值与 `Content` 完全相等（无任何前缀）。
- 删除 `internal/ai/ai_test.go` 中的 `TestWorkerPersona_*` / `TestBuildWorkerSessionPrefix_*` / `TestBuildBeeSessionPrefix`。

dispatcher.go 和 feeder.go 现有的集成行为测试保持不变；它们的断言对象（最终 prompt 字符串）行为应当等价。

### 6. 文件落点

新增：
- `internal/ai/core/session.go`
- `internal/ai/core/session_test.go`

修改：
- `internal/ai/ai.go` —— 删除旧的 `WorkerPersona` / `BuildWorkerSessionPrefix` / `BuildBeeSessionPrefix` / `writePrefixStep1`。
- `internal/ai/ai_test.go` —— 删除旧的 prompt 相关用例。
- `internal/domain/task/dispatcher.go` —— 改 `executeFresh` 调用（可选: `resolveExecution` resume 分支也接到 `BuildSessionPrompt`）。
- `internal/domain/bee/feeder.go` —— 改 `drainSession` 调用 + 把 `buildPrompt` 拆为 `assembleMessages`。

## 影响范围

- `internal/ai` 公开 API 收紧：移除 4 个公开函数；不新增公开符号。
- `internal/ai/core` 新增 1 个公开函数（`BuildSessionPrompt`）+ 2 个公开类型（`SessionRequest`、`WorkerIdentity`）。
- 业务调用方 dispatcher / feeder 首次 import `internal/ai/core`，配合 `internal/ai` 一起使用（前者出实现，后者出 `Role`）。
- 新增角色只需在 `rolePrefixSpecs` 加一项；若该角色支持 persona，新增对应 Identity 结构体。
- 不影响 engine 适配层、registry、dynamic 路由、`EngineArgsMap` 等任何其他部分。

## 不做的事

- 不把 bee 的 `<message_meta>` / `<message_content>` 装配搬到 core（会让 core 反向依赖 bee 域）。
- 不引入新的 Role 值；现有 RoleBee / RoleWorker 保持不变。
- 不改 engine 相关 API。
- 不动 Section 5 中跟 prompt 无关的 `EngineArgsMap` / `ParseEngineArgs` 等工具。
