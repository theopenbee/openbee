# /status 斜杠命令设计

- 日期：2026-04-29
- 状态：已批准设计草案，待写实现计划
- 作者：貂蝉

## 1. 背景与目标

OpenBee 已有三个聊天端斜杠命令：`/engine`、`/clear`、`/stop`，分别用于切换引擎、清空会话上下文、停止当前 bee。
另有 CLI 端 `openbee status`，用于查看守护进程是否在跑。

本次新增 **聊天端** `/status` 斜杠命令，让用户**在聊天里**就能看到 *当前会话* 的运行情况，无需切到终端或调 ctl 接口。
与 CLI 端的 status 是两个独立指令、面向不同读者；CLI 端不在本次改动范围内。

## 2. 范围

- 作用域：当前会话（`replyTo.SessionKey`）。
- 不支持参数。任何额外参数均回复 usage 提示。
- 不做权限控制（与现有 `/engine`、`/clear`、`/stop` 一致；它们也未加权限）。
- 不展示守护进程级、跨会话或全局指标。

## 3. 命令语义与回复格式

### 3.1 命令

`/status` —— 查看当前会话的活跃 bee 和正在执行的即时任务。

### 3.2 输出模板（含具体示例）

输出含两段：**已激活 bee** 与 **进行中任务**。两段始终都会出现；为空时段内显示 `(无)`，结构稳定。

正常情况：

```
当前会话状态：
已激活 bee（2）：
  - 貂蝉   引擎: claude   最近活跃: 2m 前
  - 吕布   引擎: codex    最近活跃: 5h 前
进行中任务（2）：
  - [貂蝉] 新增 /status 指令…       已运行 1m23s   exec: a1b2c3d4
  - [吕布] 修复登录 bug             已运行 12s     exec: e5f6a7b8
```

全空：

```
当前会话状态：
已激活 bee（0）：
  (无)
进行中任务（0）：
  (无)
```

### 3.3 字段说明

**已激活 bee 段**（数据源 `store.SessionAgent`）：
| 显示 | 数据 | 备注 |
|------|------|------|
| `name` | `SessionAgent.Name` | 已包含 worker 名 / "bee" / "(deleted)" |
| `引擎` | `SessionAgent.Engine` | |
| `最近活跃` | `now - SessionAgent.UpdatedAt` | 相对时间，见 §3.4 |

**进行中任务段**（数据源 `model.Task`，过滤 `status=running` + `type=immediate`）：
| 显示 | 数据 | 备注 |
|------|------|------|
| `worker` | 通过 `WorkerID` 查 `Worker.Name` | 见 §4.3 实现选项 |
| `内容缩略` | `Task.Instruction` | 超过 40 字符截断，结尾加 `…`；换行替换为空格 |
| `已运行` | `now - Task.CreatedAt` | 相对时间，见 §3.4 |
| `exec` | `Task.ExecutionID` 前 8 位 | 便于在 ctl/日志里 grep |

### 3.4 相对时间格式

`<60s` → `Ns`，`<60m` → `Nm`，`<24h` → `Nh`，否则 `Nd`。
单位向下取整。复用类似 `cmd/openbee/status.go::formatUptime` 的风格，但要求统一在 domain 层实现以便测试，不直接依赖 cmd 包。

### 3.5 异常输入

- `/status x`、`/status x y` 等带参数 → 回复 `用法：/status` 单行提示。

## 4. 实现设计

### 4.1 新增文件

- `internal/domain/command/status.go` —— Handler 实现、格式化函数。
- `internal/domain/command/status_test.go` —— 单测。

### 4.2 命令常量与 Handler 形态

在 `engine.go` 中新增常量：
```go
const CmdStatus = "/status"
```

Handler 与现有三者同构：
```go
type StatusCommandHandler struct {
    sessions  StatusSessionLister
    tasks     StatusTaskLister
    workers   StatusWorkerLookup
    senders   map[string]platform.PlatformSenderAdapter
    engineCfg *enginecfg.Store
}
func NewStatusCommandHandler(...) *StatusCommandHandler
func (h *StatusCommandHandler) IsCommand(content string) bool
func (h *StatusCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool
```

### 4.3 数据获取与依赖接口

接口在 `status.go` 新增（**不复用** `Clear*` 接口，避免命名混淆；签名与现有 store 完全一致，所以无需新增 store 方法）：

```go
type StatusSessionLister interface {
    ListActiveSessionContexts(ctx context.Context, sessionKey, beeEngine string) ([]store.SessionAgent, error)
}
type StatusTaskLister interface {
    ListBySessionKey(ctx context.Context, sessionKey, status, taskType string) ([]model.Task, error)
}
type StatusWorkerLookup interface {
    GetByID(id string) (model.Worker, error)
}
```

`workerStore.GetByID` 的存在性需在实现阶段确认；若不存在，则改为：
- 先 `ListActiveSessionContexts`，将其中 `AgentType="worker"` 的 `(AgentID → Name)` 建表；
- 任务 worker 名优先从该表查；
- 无命中（任务 worker 已无 session context）时显示 `(unknown)`。

**这两种实现方式在写实现计划时再二选一**；功能上等价。

### 4.4 流程

```
HandleCommand(ctx, content, replyTo):
  if fields(content) != ["/status"]:
    reply(Usage); return true
  agents, err := sessions.ListActiveSessionContexts(ctx, sessionKey, engineCfg.Get())
  if err: log + reply(LookupFailed); return true
  tasks, err := tasks.ListBySessionKey(ctx, sessionKey, "running", "immediate")
  if err: log + reply(LookupFailed); return true
  text := format(agents, tasks)
  reply(text)
  return true
```

### 4.5 接入位置

在 `internal/app/app.go:161` 的 `cmdChain` 末尾追加 `statusCmdHandler`：
```go
cmdChain := msgingest.ChainHandlers(
    engineCmdHandler, clearCmdHandler, stopCmdHandler, statusCmdHandler,
)
```
顺序在最后即可（chain 路由按是否匹配命令前缀）。

### 4.6 i18n

在 `internal/infra/i18n/locales/{zh,en}.yaml` 的 `runtime` 节点新增 `status_command`：

```yaml
status_command:
  usage: "用法：/status"
  lookup_failed: "⚠️ 查询会话状态失败，请稍后重试；若持续出现请检查服务端日志。"
  header: "当前会话状态："
  section_bees: "已激活 bee（%d）："
  section_tasks: "进行中任务（%d）："
  empty_marker: "  (无)"
  bee_line: "  - %s   引擎: %s   最近活跃: %s"
  task_line: "  - [%s] %s       已运行 %s   exec: %s"
```

英文 yaml 同步增加对应键，文案翻译保持简洁。
i18n Go 端结构体（`internal/infra/i18n/messages.go`）相应新增 `StatusCommand` 字段。

## 5. 错误处理

任一 store 查询失败 → log.Error + 回复统一 `lookup_failed` 文案，不输出半截结果。
失败仅影响本次回复，不影响后续命令。

## 6. 测试

`internal/domain/command/status_test.go` 沿用 `engine_test.go` / `clear_test.go` 的 fake store + 收集 sender 模式：

| 用例 | 期望 |
|------|------|
| `IsCommand("/status")` | true |
| `IsCommand("/status x")` | true（让 HandleCommand 返回 Usage） |
| `IsCommand("/statuses")` | false |
| 正常路径：bees + tasks 都非空 | 输出包含两段，每段含正确条数 |
| 仅 bee 无任务 | tasks 段显示 `(无)` |
| 仅任务无 bee | bees 段显示 `(无)` |
| 全空 | 两段都显示 `(无)` |
| `/status x` | 回复 Usage |
| `sessions.List` 失败 | 回复 LookupFailed，不查 tasks |
| `tasks.List` 失败 | 回复 LookupFailed |
| Instruction 超过 40 字符 | 截断 + `…` |
| Instruction 含换行 | 换行替换为空格 |
| 时间格式：12s / 5m / 3h / 2d 边界 | 各产生预期单位 |

## 7. 不在本次范围

- CLI 端 `openbee status` 输出格式（保持现状）。
- `/status all` 跨会话视图。
- `/status <worker>` 单 worker 子命令。
- 守护进程版本号、内存等系统级指标。
- 权限/审计。

## 8. 风险

- `formatUptime` 当前位于 `cmd/openbee` 包，不能被 domain 层引用；需要在 domain 层重写一个。两份实现存在轻微漂移风险，但作用域不同（CLI 显示守护进程 uptime；domain 显示活动相对时间），可接受。
- `Task.CreatedAt` 不完全等同于"开始执行时间"——immediate 任务从创建到 running 通常很短（debounce 之后即派发），近似可接受；如需精确，未来可在 task 模型加 `StartedAt` 字段，本次不做。
