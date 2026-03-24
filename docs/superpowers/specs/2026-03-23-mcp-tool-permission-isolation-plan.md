# MCP 工具权限隔离 — 实施计划

> 日期：2026-03-23
> 关联设计文档：`docs/superpowers/specs/2026-03-23-mcp-tool-permission-isolation-design.md`
> 状态：草案

---

## 总览

本计划将设计文档中的方案 2+3 融合方案拆分为 4 个实施阶段、11 个任务步骤。各阶段按依赖关系串行推进，阶段内的任务可以部分并行。

**影响范围**：约 9 个源文件修改 + 测试文件新增/修改
**预计总工作量**：约 2-3 小时（单人执行）

---

## 阶段一：配置与基础设施

> 目标：为 Worker 引入独立的 API Key 配置，奠定隔离基础。
> 影响文件：2 个

### 步骤 1.1：MCPConfig 增加 WorkerAPIKey 字段

**文件**：`internal/config/config.go`

**任务**：
1. 在 `MCPConfig` 结构体中新增 `WorkerAPIKey string \`yaml:"worker_api_key"\`` 字段
2. 在 `applyDefaults()` 中添加逻辑：若 `WorkerAPIKey` 为空，则自动生成随机密钥
3. 在 `BuildApp()` 中（`app.go`）增加校验：确保 `WorkerAPIKey` 非空（与现有 `APIKey` 校验对称）

**验收标准**：
- 不配置 `worker_api_key` 时，程序启动后 `cfg.Bee.MCP.WorkerAPIKey` 自动填充一个非空随机值
- 显式配置 `worker_api_key` 时，使用配置值
- 现有 `api_key` 逻辑不受影响

### 步骤 1.2：更新配置模板

**文件**：`internal/config/config.yaml.tmpl`

**任务**：
1. 在 `bee.mcp` 部分新增 `worker_api_key` 配置项，附注释说明"缺失时自动生成"

**验收标准**：
- 模板中包含 `worker_api_key` 字段及说明注释

### 阶段一里程碑

- [ ] `go build ./...` 编译通过
- [ ] `go test ./internal/config/...` 通过
- [ ] 手动验证：启动时日志或调试输出中 `WorkerAPIKey` 非空

---

## 阶段二：MCP Server 拆分

> 目标：将单一 MCPServer 拆分为 Bee 和 Worker 两个实例，各自拥有独立的工具集。
> 影响文件：2-3 个
> 依赖：阶段一完成

### 步骤 2.1：工具定义分组

**文件**：`internal/mcp/tools.go`

**任务**：
1. 将现有 `toolSchemas()` 重命名为 `beeToolSchemas()`（返回全部 19 个工具）
2. 新增 `workerToolSchemas()` 函数，仅返回 5 个工具的定义：
   - `send_message`
   - `mark_task_complete`
   - `save_memory`
   - `get_memory`
   - `delete_memory`
3. 保留导出函数 `ToolSchemas()` 调用 `beeToolSchemas()` 以维持测试兼容

**验收标准**：
- `beeToolSchemas()` 返回 19 个工具
- `workerToolSchemas()` 返回 5 个工具
- 现有 `ToolSchemas()` 行为不变

### 步骤 2.2：工具调用分组

**文件**：`internal/mcp/tools.go`

**任务**：
1. 将现有 `callTool()` 方法重命名为 `beeCallTool()`
2. 新增 `workerCallTool()` 方法，switch 仅包含 5 个 case：
   - `mark_task_complete` → `s.toolMarkTaskComplete(args)`
   - `send_message` → `s.toolSendMessage(args)`
   - `save_memory` → `s.toolSaveMemory(args)`
   - `get_memory` → `s.toolGetMemory(args)`
   - `delete_memory` → `s.toolDeleteMemory(args)`
   - default → 返回 `fmt.Errorf("tool not found: %s", name)`
3. `callTool` 改为一个 dispatch 函数，调用 `MCPServer` 内部存储的 handler

**验收标准**：
- Worker 的 `callTool` 调用 `create_worker` 等 Bee 专用工具时返回 "tool not found" 错误
- Bee 的 `callTool` 调用任何工具均正常

### 步骤 2.3：MCPServer 构造函数拆分

**文件**：`internal/mcp/server.go`

**任务**：
1. 在 `MCPServer` 结构体中新增两个字段：
   - `schemas func() []toolSchema` — 返回该实例可用的工具定义
   - `callToolFn func(name string, args json.RawMessage) (any, error)` — 该实例的工具调用 handler
2. 修改 `dispatch()` 中 `tools/list` 分支：调用 `s.schemas()` 代替硬编码 `toolSchemas()`
3. 修改 `handleToolCall()` 中的调用：使用 `s.callToolFn(...)` 代替 `s.callTool(...)`
4. 新增 `NewBeeServer(...)` 构造函数：
   - 接收全部依赖（与现有 `NewServer` 签名一致）
   - 设置 `schemas = beeToolSchemas`，`callToolFn = s.beeCallTool`
5. 新增 `NewWorkerServer(...)` 构造函数：
   - 仅接收 Worker 工具所需的依赖：`TaskStore`、`MessageStore`、`senders`、`MemoryStore`
   - 设置 `schemas = workerToolSchemas`，`callToolFn = s.workerCallTool`
   - 不需要的字段（`workerStore`、`manager`、`executionStore` 等）保留零值
6. 保留现有 `NewServer(...)` 函数作为 `NewBeeServer` 的别名（向后兼容测试代码），或将其标记为 deprecated 并更新调用方

**验收标准**：
- `NewBeeServer` 创建的实例 `tools/list` 返回 19 个工具
- `NewWorkerServer` 创建的实例 `tools/list` 返回 5 个工具
- 两个实例各自独立管理 SSE session
- 编译通过

### 阶段二里程碑

- [ ] `go build ./...` 编译通过
- [ ] `go test ./internal/mcp/...` 通过（更新/新增的测试）
- [ ] 验证 `NewWorkerServer` 实例调用 Bee 工具返回错误

---

## 阶段三：路由与连接层

> 目标：注册双端点，让 Bee 和 Worker 分别连接到各自的 MCP Server 实例。
> 影响文件：4 个
> 依赖：阶段二完成

### 步骤 3.1：Invoker URL 拼接调整

**文件**：`internal/claude/invoker.go`

**任务**：
1. 将 `NewInvoker` 中 `mcpURL` 的拼接逻辑从 `mcpBaseURL + "/mcp/sse"` 改为 `mcpBasePath + "/sse"`
2. 参数名从 `mcpBaseURL` 改为 `mcpBasePath`，明确其含义为"含子路径的 base"
3. 更新函数注释

**验收标准**：
- `NewInvoker("claude", "http://localhost:8080/mcp/bee", "key")` 构建的 `mcpURL` 为 `http://localhost:8080/mcp/bee/sse`
- 现有 `invoker_test.go` 更新后通过

### 步骤 3.2：Bee 连接路径更新

**文件**：`internal/bee/bee_process.go`

**任务**：
1. 将 `NewBeeProcess` 中 `claude.NewInvoker` 的第二个参数从 `cfg.MCPBaseURL` 改为 `cfg.MCPBaseURL + "/mcp/bee"`
2. API Key 不变，仍使用 `cfg.MCP.APIKey`

**验收标准**：
- Bee 连接到 `/mcp/bee/sse` 端点

### 步骤 3.3：Worker 连接路径和 Key 更新

**文件**：`internal/worker/manager.go`

**任务**：
1. 将 `NewManager` 中 `claude.NewInvoker` 的第二个参数从 `bc.MCPBaseURL` 改为 `bc.MCPBaseURL + "/mcp/worker"`
2. 将第三个参数从 `bc.MCP.APIKey` 改为 `bc.MCP.WorkerAPIKey`

**验收标准**：
- Worker 连接到 `/mcp/worker/sse` 端点
- Worker 使用 `WorkerAPIKey` 认证

### 步骤 3.4：双端点路由注册

**文件**：`internal/api/router.go`

**任务**：
1. `Server` 结构体新增字段：
   - `workerMCPServer *mcp.MCPServer`
   - `workerAPIKey string`
2. 将现有 `mcpServer` 字段重命名为 `beeMCPServer`，`mcpAPIKey` 重命名为 `beeAPIKey`
3. 更新 `NewServer` 构造函数签名，接收两个 MCP Server 和两个 API Key
4. 修改 `registerMCPRoutes()`：
   - `/mcp/bee/sse` + `/mcp/bee/messages` → `beeMCPServer`，使用 `beeAPIKey`
   - `/mcp/worker/sse` + `/mcp/worker/messages` → `workerMCPServer`，使用 `workerAPIKey`
   - `/mcp/sse` + `/mcp/messages` → `beeMCPServer`，使用 `beeAPIKey`（向后兼容）
5. 更新 gzip 排除路径列表，新增 `/mcp/bee/sse`、`/mcp/worker/sse` 等

**验收标准**：
- 三组端点均正确注册
- 各端点使用正确的 API Key 中间件
- SSE 端点不被 gzip 压缩

### 阶段三里程碑

- [ ] `go build ./...` 编译通过
- [ ] `go test ./internal/claude/...` 通过
- [ ] `go test ./internal/worker/...` 通过
- [ ] 所有路由正确注册（可通过调试日志或单元测试验证）

---

## 阶段四：装配、测试与验收

> 目标：完成组件装配，编写全面测试，端到端验证隔离效果。
> 影响文件：2+ 个
> 依赖：阶段三完成

### 步骤 4.1：App 装配更新

**文件**：`internal/app/app.go`

**任务**：
1. 将 `mcpSrv := mcp.NewServer(...)` 改为：
   ```go
   beeMCPSrv := mcp.NewBeeServer(...)
   workerMCPSrv := mcp.NewWorkerServer(...)
   ```
2. 更新 `buildAPIServer` 调用，传入两个 MCP Server 实例和两个 API Key
3. 在启动校验中增加 `WorkerAPIKey` 非空检查

**验收标准**：
- `BuildApp` 正确创建两个 MCP Server 实例
- 编译通过，程序可正常启动

### 步骤 4.2：单元测试

**文件**：`internal/mcp/tools_test.go`（修改）、`internal/mcp/auth_test.go`（可能修改）

**任务**：
1. 新增测试：`TestWorkerToolSchemasCount` — 验证 `workerToolSchemas()` 返回恰好 5 个工具
2. 新增测试：`TestBeeToolSchemasCount` — 验证 `beeToolSchemas()` 返回 19 个工具
3. 新增测试：`TestWorkerCannotCallBeeTools` — 使用 `NewWorkerServer` 创建实例，逐一调用 Bee 专用工具（14 个），验证全部返回 "tool not found" 错误
4. 新增测试：`TestWorkerCanCallAllowedTools` — 使用 `NewWorkerServer` 创建实例，调用 5 个 Worker 工具，验证均能正常处理（无 "tool not found" 错误）
5. 更新现有 `tools_test.go` 中引用 `NewServer` 的测试，改用 `NewBeeServer`

**验收标准**：
- `go test ./internal/mcp/... -v` 全部通过
- 覆盖所有 Bee 专用工具的越权拒绝场景

### 步骤 4.3：集成测试（可选，建议后续补充）

**说明**：集成测试需要启动完整 HTTP Server，较为重量级。可在后续迭代中补充。

**建议测试点**：
1. 使用 Worker API Key 访问 `/mcp/bee/sse` → 返回 401
2. 使用 Bee API Key 访问 `/mcp/worker/sse` → 返回 401
3. 使用 Bee API Key 访问 `/mcp/sse`（旧端点）→ 正常连接
4. Worker 端连接后 `tools/list` 仅返回 5 个工具
5. Bee 端连接后 `tools/list` 返回 19 个工具

### 步骤 4.4：端到端验收

**任务**：
1. 启动 openbee，验证 Bee 和 Worker 分别连接到各自端点
2. 通过平台发送消息，验证 Bee 正常分发任务、Worker 正常执行并回复
3. 检查日志确认 Bee 连接到 `/mcp/bee/sse`，Worker 连接到 `/mcp/worker/sse`
4. （手动测试）构造一条包含 prompt injection 的消息，验证 Worker 无法调用 Bee 专用工具

### 阶段四里程碑

- [ ] `go test ./...` 全部通过
- [ ] `go build ./...` 编译通过
- [ ] 端到端功能验证通过
- [ ] 提交 git commit

---

## 执行顺序与依赖关系

```
阶段一（配置）
  ├── 步骤 1.1: MCPConfig 新增字段
  └── 步骤 1.2: 配置模板更新
         │
         ▼
阶段二（MCP Server 拆分）
  ├── 步骤 2.1: 工具定义分组
  ├── 步骤 2.2: 工具调用分组（依赖 2.1）
  └── 步骤 2.3: 构造函数拆分（依赖 2.1 + 2.2）
         │
         ▼
阶段三（路由与连接）
  ├── 步骤 3.1: Invoker URL 调整
  ├── 步骤 3.2: Bee 连接更新（依赖 3.1）
  ├── 步骤 3.3: Worker 连接更新（依赖 3.1 + 阶段一）
  └── 步骤 3.4: 双端点路由注册（依赖阶段二）
         │
         ▼
阶段四（装配与测试）
  ├── 步骤 4.1: App 装配（依赖阶段二 + 阶段三）
  ├── 步骤 4.2: 单元测试（依赖阶段二）
  ├── 步骤 4.3: 集成测试（依赖 4.1）
  └── 步骤 4.4: 端到端验收（依赖 4.1）
```

**可并行的步骤**：
- 步骤 1.1 和 1.2 可并行
- 步骤 3.1 和 3.4 可并行（不同文件，无代码依赖）
- 步骤 4.2 可在 4.1 之前编写（测试可以先写、后跑）

---

## 风险检查清单

| 风险 | 缓解措施 |
|------|---------|
| 修改 `NewInvoker` 参数语义导致现有调用方传错值 | 阶段三统一更新所有调用方（`bee_process.go` + `manager.go`），编译器会捕获遗漏 |
| `router.go` 重命名字段导致编译错误蔓延 | 步骤 3.4 与步骤 4.1 配合，确保 `NewServer` 签名和调用方同步更新 |
| 旧端点删除导致外部集成中断 | 保留 `/mcp/sse` 作为别名，不删除 |
| `WorkerAPIKey` 自动生成逻辑未触发 | 单元测试覆盖 `applyDefaults` 中 `WorkerAPIKey` 为空的 case |
| 工具实现方法（如 `toolSendMessage`）依赖 Worker Server 未注入的 store | `workerCallTool` 只路由到不依赖未注入 store 的方法；`NewWorkerServer` 的参数列表精确匹配所需依赖 |

---

## 交付物清单

| 阶段 | 交付物 |
|------|--------|
| 阶段一 | `config.go` 和 `config.yaml.tmpl` 变更 |
| 阶段二 | `server.go` 和 `tools.go` 变更，支持双实例 |
| 阶段三 | `invoker.go`、`bee_process.go`、`manager.go`、`router.go` 变更 |
| 阶段四 | `app.go` 变更、测试代码、端到端验证通过 |
| 最终 | 一个完整的 git commit，包含所有变更和测试 |
