# MCP 工具权限隔离设计文档

> 日期：2026-03-23
> 状态：草案
> 作者：毛毛

## 1. 问题背景

### 1.1 现状

OpenBee 采用中央大脑（Bee）+ 多 Worker 的架构。Bee 负责接收用户消息、理解意图、分配任务给 Worker；Worker 负责执行具体任务并回复用户。

当前系统中只有一个 MCP 服务实例，暴露在 `/mcp/sse` 和 `/mcp/messages` 端点上。Bee 和所有 Worker 通过同一个 API Key 连接到同一个 MCP 服务，能够看到并调用全部 19 个工具。

### 1.2 安全风险

Worker 由 Claude CLI 驱动，其行为受 prompt 指引。恶意用户可以通过 prompt injection（在消息内容中嵌入诱导性指令）让 Worker 调用不应拥有权限的工具，例如：

| 风险类别 | 可被滥用的工具 | 危害 |
|---------|-------------|------|
| 操控 Worker | `create_worker`、`update_worker`、`delete_worker` | 创建恶意 Worker、篡改或删除现有 Worker |
| 越权任务管理 | `create_task`、`cancel_task` | 伪造 Bee 创建任务、取消合法任务 |
| 破坏会话 | `clear_session`、`clear_worker_session` | 清除所有会话状态，中断正常服务 |
| 信息泄露 | `get_system_overview`、`list_bee_executions`、`list_workers` | 窥探系统全貌、其他 Worker 信息、Bee 决策历史 |

### 1.3 设计目标

1. **最小权限原则**：Worker 仅能访问完成任务所需的最少工具集
2. **密钥级隔离**：通过不同的 API Key 区分角色，防止 prompt injection 绕过
3. **独立工具集**：Bee 和 Worker 各自拥有独立的工具定义和处理逻辑
4. **向后兼容**：旧配置无需手动修改，`worker_api_key` 缺失时自动生成
5. **可扩展**：架构支持未来添加更多角色（如只读观察者）

## 2. 方案概述

采用**方案二（分离 API Key 绑定角色）与方案三（独立 MCP 服务实例）的融合方案**：

- **分离 API Key**（来自方案二）：为 Bee 和 Worker 配置不同的 API Key，认证中间件根据 Key 判定角色，从根源上防止角色伪造
- **独立服务实例**（来自方案三）：创建两个 MCP Server 实例，分别注册到不同路由端点，各自拥有独立的工具列表和 session 管理

这种融合方案实现了**双重隔离**：认证层隔离 + 服务实例隔离。即使某一层被绕过，另一层仍能阻止越权访问。

## 3. 架构设计

### 3.1 整体架构

```
┌──────────────────────────────────────────────────────────────┐
│                      OpenBee Core                            │
│                                                              │
│  ┌─────────────┐                    ┌─────────────────────┐  │
│  │  Bee Feeder  │                   │  Worker Manager     │  │
│  │  (Claude CLI)│                   │  (Claude CLI × N)   │  │
│  └──────┬───────┘                   └──────────┬──────────┘  │
│         │ bee_api_key                          │ worker_api_key│
│         ▼                                      ▼              │
│  ┌──────────────────┐              ┌──────────────────────┐  │
│  │ /mcp/bee/sse     │              │ /mcp/worker/sse      │  │
│  │ /mcp/bee/messages│              │ /mcp/worker/messages │  │
│  │                  │              │                      │  │
│  │ APIKeyMiddleware │              │ APIKeyMiddleware     │  │
│  │ (bee_api_key)    │              │ (worker_api_key)     │  │
│  └──────┬───────────┘              └──────────┬───────────┘  │
│         ▼                                      ▼              │
│  ┌──────────────────┐              ┌──────────────────────┐  │
│  │ BeeMCPServer     │              │ WorkerMCPServer      │  │
│  │                  │              │                      │  │
│  │ 全部 19 个工具    │              │ 仅 5 个工具          │  │
│  │ 独立 session 管理 │              │ 独立 session 管理    │  │
│  └──────────────────┘              └──────────────────────┘  │
│                                                              │
│                    ┌──────────────┐                           │
│                    │ 共享后端服务   │                           │
│                    │ - WorkerStore │                           │
│                    │ - TaskStore   │                           │
│                    │ - MessageStore│                           │
│                    │ - MemoryStore │                           │
│                    └──────────────┘                           │
└──────────────────────────────────────────────────────────────┘
```

### 3.2 关键设计决策

**Q: 为什么不只用分离 API Key + 单实例的角色过滤？**

单实例方案需要在 `tools/list` 和 `tools/call` 两处都做角色过滤。如果过滤逻辑有 bug（如漏过某个工具名），就会造成越权。两个独立实例从编译期就确定了各自的工具集，无需运行时过滤，更安全。

**Q: 为什么不只用独立实例、不区分 API Key？**

独立端点已经提供了路径级隔离，但 Claude CLI 在运行时通过 `--mcp-config` 参数接收 MCP URL，如果 Worker 的 Claude CLI 进程通过 prompt injection 被诱导访问 Bee 的端点（虽然在当前实现中不太可能，因为 URL 在启动时固定），分离 API Key 提供了额外的密钥级保护。

**Q: 旧端点 `/mcp/sse` 如何处理？**

保留原有端点 `/mcp/sse` 和 `/mcp/messages` 作为 Bee 端点的别名（或重定向到 `/mcp/bee/sse`），确保向后兼容。在未来版本中可考虑废弃。

## 4. 核心模块设计

### 4.1 配置变更

```yaml
bee:
  mcp:
    api_key: "bee-key-xxx"          # Bee 专用（原有字段，语义不变）
    worker_api_key: "worker-key-yyy" # Worker 专用（新增，缺失时自动生成）
```

**`config.go` 变更：**

```go
type MCPConfig struct {
    APIKey       string `yaml:"api_key"`
    WorkerAPIKey string `yaml:"worker_api_key"`
}
```

在 `applyDefaults` 中，如果 `WorkerAPIKey` 为空，自动生成一个随机密钥（与现有 `api_key` 自动生成逻辑一致）。

### 4.2 MCP Server 拆分

当前 `MCPServer` 包含所有 19 个工具的定义和实现。拆分策略：

#### 4.2.1 保留共享的 MCPServer 基础结构

`MCPServer` 保留 SSE session 管理、JSON-RPC dispatch、heartbeat 等通用逻辑。通过构造函数参数控制暴露哪些工具。

```go
// ToolSet 定义一组可用工具
type ToolSet struct {
    Schemas []toolSchema
    Handler func(name string, args json.RawMessage) (any, error)
}

// NewServer 接受 ToolSet 参数来控制暴露的工具
func NewServer(toolSet ToolSet, /* ...其他依赖... */) *MCPServer
```

#### 4.2.2 两个构造函数

```go
// NewBeeServer 创建 Bee 专用 MCP Server（全部工具）
func NewBeeServer(
    ws *store.WorkerStore,
    mgr *worker.Manager,
    ts *store.TaskStore,
    ms *store.MessageStore,
    senders map[string]platform.PlatformSenderAdapter,
    execStopper ExecutionStopper,
    sessionClearer SessionClearer,
    es *store.ExecutionStore,
    memStore *store.MemoryStore,
    sessionStore *store.SessionStore,
) *MCPServer

// NewWorkerServer 创建 Worker 专用 MCP Server（受限工具）
func NewWorkerServer(
    ts *store.TaskStore,
    ms *store.MessageStore,
    senders map[string]platform.PlatformSenderAdapter,
    memStore *store.MemoryStore,
) *MCPServer
```

`NewWorkerServer` 只注入 Worker 工具所需的依赖，无需 `WorkerStore`、`ExecutionStore`、`SessionStore` 等。

#### 4.2.3 工具分组

将 `tools.go` 中的工具按角色分组：

```go
// beeToolSchemas 返回 Bee 可用的全部工具定义
func beeToolSchemas() []toolSchema { /* 全部 19 个 */ }

// workerToolSchemas 返回 Worker 可用的工具定义
func workerToolSchemas() []toolSchema { /* 仅 5 个 */ }
```

`callTool` 同理拆分为 `beeCallTool` 和 `workerCallTool`，Worker 版本的 switch 只包含 5 个 case，其余工具名直接返回 "tool not found" 错误。

### 4.3 路由注册变更

```go
// router.go

func (s *Server) registerMCPRoutes() {
    // Bee MCP endpoints
    beeGroup := s.router.Group("/mcp/bee")
    beeGroup.Use(mcp.APIKeyMiddleware(s.beeAPIKey))
    beeGroup.GET("/sse", s.beeMCPServer.HandleSSE)
    beeGroup.POST("/messages", s.beeMCPServer.HandleMessages)

    // Worker MCP endpoints
    workerGroup := s.router.Group("/mcp/worker")
    workerGroup.Use(mcp.APIKeyMiddleware(s.workerAPIKey))
    workerGroup.GET("/sse", s.workerMCPServer.HandleSSE)
    workerGroup.POST("/messages", s.workerMCPServer.HandleMessages)

    // 向后兼容：保留原有路径作为 Bee 端点的别名
    legacyGroup := s.router.Group("/mcp")
    legacyGroup.Use(mcp.APIKeyMiddleware(s.beeAPIKey))
    legacyGroup.GET("/sse", s.beeMCPServer.HandleSSE)
    legacyGroup.POST("/messages", s.beeMCPServer.HandleMessages)
}
```

### 4.4 Invoker 层变更

`claude.Invoker` 当前在构造时接收一个 `apiKey`，构建 MCP URL。需要让 Bee 和 Worker 使用不同的 URL。

**方案**：不改动 `Invoker` 结构，而是创建两个 `Invoker` 实例。

```go
// worker/manager.go — Worker 使用 worker 端点和 worker key
func NewManager(...) *Manager {
    return &Manager{
        invoker: claude.NewInvoker(
            bc.Claude.Path,
            bc.MCPBaseURL + "/mcp/worker",  // Worker 专用路径
            bc.MCP.WorkerAPIKey,             // Worker 专用 Key
        ),
        // ...
    }
}

// bee/bee_process.go — Bee 使用 bee 端点和 bee key
func NewBeeProcess(cfg config.BeeConfig) *BeeProcess {
    return &BeeProcess{
        invoker: claude.NewInvoker(
            cfg.Claude.Path,
            cfg.MCPBaseURL + "/mcp/bee",     // Bee 专用路径
            cfg.MCP.APIKey,                   // Bee 专用 Key
        ),
    }
}
```

注意：`Invoker.mcpURL` 当前在构造时拼接 `/mcp/sse` 后缀。改为由外部传入完整的 base path（如 `/mcp/bee`），`Invoker` 内部追加 `/sse`。

**`claude/invoker.go` 变更**：

```go
// NewInvoker 的 mcpBasePath 参数含义变更
// 之前: mcpBaseURL = "http://host:port"，内部拼接 "/mcp/sse"
// 之后: mcpBasePath = "http://host:port/mcp/bee"，内部拼接 "/sse"
func NewInvoker(binary, mcpBasePath, apiKey string) *Invoker {
    return &Invoker{
        binary: binary,
        mcpURL: mcpBasePath + "/sse",  // 之前是 mcpBaseURL + "/mcp/sse"
        apiKey: apiKey,
    }
}
```

### 4.5 app.go 装配变更

```go
func BuildApp(cfg config.Config) (*App, error) {
    // ...existing code...

    // 创建两个 MCP Server 实例
    beeMCPSrv := mcp.NewBeeServer(
        s.workerStore, mgr, s.taskStore, s.msgStore,
        sendersByPlatform, mgr, disp,
        s.execStore, s.memoryStore, s.sessionStore,
    )
    workerMCPSrv := mcp.NewWorkerServer(
        s.taskStore, s.msgStore, sendersByPlatform, s.memoryStore,
    )

    srv, err := buildAPIServer(
        cfg.Server, cfg.Bee.MCP, s, mgr, logRegistry,
        beeMCPSrv, workerMCPSrv, localChat,
    )
    // ...
}
```

## 5. 工具权限模型

### 5.1 角色定义

| 角色 | 说明 | API Key 来源 | MCP 端点 |
|------|------|-------------|---------|
| **bee** | 中央大脑/协调者 | `bee.mcp.api_key` | `/mcp/bee/sse` |
| **worker** | 任务执行者 | `bee.mcp.worker_api_key` | `/mcp/worker/sse` |

### 5.2 工具权限矩阵

| 工具名称 | Bee | Worker | 说明 |
|---------|:---:|:------:|------|
| `list_workers` | ✅ | ❌ | Worker 无需了解其他 Worker |
| `get_worker` | ✅ | ❌ | 同上 |
| `create_worker` | ✅ | ❌ | 仅 Bee 可管理 Worker |
| `update_worker` | ✅ | ❌ | 同上 |
| `delete_worker` | ✅ | ❌ | 同上 |
| `create_task` | ✅ | ❌ | 仅 Bee 可分配任务 |
| `list_tasks` | ✅ | ❌ | Worker 无需列出其他任务 |
| `cancel_task` | ✅ | ❌ | 仅 Bee 可取消任务 |
| `mark_task_complete` | ✅ | ✅ | Worker 需要标记自身任务完成 |
| `send_message` | ✅ | ✅ | 两者都需要回复用户 |
| `clear_session` | ✅ | ❌ | 仅 Bee 可管理会话 |
| `get_worker_status` | ✅ | ❌ | 仅 Bee 需要监控 Worker |
| `get_system_overview` | ✅ | ❌ | 仅 Bee 需要系统概览 |
| `list_bee_executions` | ✅ | ❌ | 仅 Bee 可查看自身历史 |
| `save_memory` | ✅ | ✅ | 两者都可保存记忆 |
| `get_memory` | ✅ | ✅ | 两者都可读取记忆 |
| `delete_memory` | ✅ | ✅ | 两者都可删除记忆 |
| `list_session_contexts` | ✅ | ❌ | 仅 Bee 可查看会话上下文 |
| `clear_worker_session` | ✅ | ❌ | 仅 Bee 可清除 Worker 会话 |

**Worker 可用工具（5 个）**：`send_message`、`mark_task_complete`、`save_memory`、`get_memory`、`delete_memory`

### 5.3 越权调用行为

当 Worker 尝试调用未授权的工具时，`workerCallTool` 返回标准的工具错误：

```json
{
  "content": [{"type": "text", "text": "tool not found: create_worker"}],
  "isError": true
}
```

由于 Worker 的 `tools/list` 不会返回这些工具，正常情况下 Claude CLI 不会尝试调用。此错误仅在极端 prompt injection 场景下触发。

## 6. 实现步骤

### 阶段一：配置与基础设施（影响 2 个文件）

1. **`config.go`**：`MCPConfig` 增加 `WorkerAPIKey` 字段，`applyDefaults` 中自动生成
2. **`config.yaml.tmpl`**：增加 `worker_api_key` 配置项（注释说明自动生成）

### 阶段二：MCP Server 拆分（影响 2-3 个文件）

3. **`mcp/tools.go`**：将 `toolSchemas()` 拆为 `beeToolSchemas()` 和 `workerToolSchemas()`；将 `callTool()` 拆为 `beeCallTool()` 和 `workerCallTool()`
4. **`mcp/server.go`**：`NewServer` 改为接受 `ToolSet` 参数；新增 `NewBeeServer()` 和 `NewWorkerServer()` 构造函数
5. （可选）将 Bee 专用工具实现和 Worker 专用工具实现分到 `bee_tools.go` 和 `worker_tools.go`

### 阶段三：路由与连接（影响 3-4 个文件）

6. **`api/router.go`**：`Server` 结构体增加 `workerMCPServer` 和 `workerAPIKey` 字段；`registerMCPRoutes` 注册双端点 + 向后兼容
7. **`claude/invoker.go`**：`NewInvoker` 的 `mcpBaseURL` 参数语义调整（含子路径）
8. **`bee/bee_process.go`**：传入 `/mcp/bee` 路径
9. **`worker/manager.go`**：使用 `WorkerAPIKey` 和 `/mcp/worker` 路径创建 Invoker

### 阶段四：装配与测试（影响 2+ 个文件）

10. **`app/app.go`**：创建两个 MCP Server 实例，传入 `buildAPIServer`
11. **测试**：验证 Worker 无法调用 Bee 专用工具；验证 API Key 交叉使用被拒绝；验证向后兼容

## 7. 风险与注意事项

### 7.1 向后兼容

- 保留 `/mcp/sse` 作为 `/mcp/bee/sse` 的别名，确保旧版 Bee 进程（如果有外部直接连接的场景）仍能工作
- `worker_api_key` 缺失时自动生成，不强制用户修改配置文件

### 7.2 内存开销

- 两个 `MCPServer` 实例各自维护独立的 `sessions map`，增加少量内存
- 实际影响极小：每个 session 仅占一个 channel（16 缓冲）+ 一个 map entry

### 7.3 未来扩展

- 如需增加新角色（如 supervisor），只需新增一个 `NewSupervisorServer()` 构造函数和对应端点
- 可考虑将工具权限模型抽象为配置驱动，但当前两个角色的场景下不建议过度抽象

### 7.4 测试策略

| 测试类型 | 验证内容 |
|---------|---------|
| 单元测试 | `workerCallTool` 调用 Bee 专用工具返回错误 |
| 单元测试 | `workerToolSchemas()` 仅返回 5 个工具 |
| 单元测试 | `beeToolSchemas()` 返回全部 19 个工具 |
| 集成测试 | Worker API Key 无法访问 `/mcp/bee/sse` |
| 集成测试 | Bee API Key 无法访问 `/mcp/worker/sse` |
| 集成测试 | 旧端点 `/mcp/sse` 使用 Bee API Key 正常工作 |

### 7.5 Prompt Injection 残余风险

本方案将 Worker 的攻击面从 19 个工具缩减到 5 个。剩余的 5 个工具（尤其是 `send_message`）仍有被滥用的可能（如发送不当内容）。这属于 LLM 安全的通用问题，不在本设计范围内，可通过 content moderation 等手段进一步缓解。

## 8. 变更影响文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/config/config.go` | 修改 | 新增 `WorkerAPIKey` 字段和默认值生成 |
| `internal/config/config.yaml.tmpl` | 修改 | 新增 `worker_api_key` 配置项 |
| `internal/mcp/server.go` | 修改 | 支持 ToolSet 参数，新增构造函数 |
| `internal/mcp/tools.go` | 修改 | 工具分组，拆分 schemas 和 callTool |
| `internal/api/router.go` | 修改 | 双端点注册，新增字段 |
| `internal/claude/invoker.go` | 修改 | URL 拼接逻辑调整 |
| `internal/bee/bee_process.go` | 修改 | 使用 Bee 专用路径 |
| `internal/worker/manager.go` | 修改 | 使用 Worker 专用 Key 和路径 |
| `internal/app/app.go` | 修改 | 创建双实例，调整装配 |
| `internal/mcp/*_test.go` | 新增/修改 | 权限隔离测试 |
