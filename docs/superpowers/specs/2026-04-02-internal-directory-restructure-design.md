# internal/ 目录结构重构设计

**日期：** 2026-04-02  
**状态：** 待实现

---

## 背景

`internal/` 当前有 24 个顶级子目录，存在两个主要问题：

1. **边界不清** — 包与包之间的职责划分不够清晰，难以判断某个功能属于哪个层次
2. **碎片化** — 部分包（如 `toolnames`、`ffmedia`）内容极少，造成目录过于零散

---

## 目标

将 24 个顶级包重组为 7 个顶级入口，通过分层 + 合并碎片包的方式提升可导航性和职责清晰度。

---

## 新目录结构

```
internal/
├── domain/              # 核心业务逻辑
│   ├── bee/             # Bee 协调进程
│   ├── worker/          # Worker 管理
│   ├── task/            # 任务调度与分发（原 task_scheduler + task_dispatcher）
│   └── msgingest/       # 消息接入网关
├── ai/                  # AI 集成
│   ├── claude/          # Claude 调用器 + CLAUDE.md 管理（原 claude + claudemd）
│   └── mcp/             # MCP 服务端
├── platform/            # 平台适配器（不变）
│   ├── dingtalk/
│   ├── feishu/
│   ├── wecom/
│   ├── local/
│   ├── weixin/
│   └── telegram/
├── infra/               # 基础设施
│   ├── config/
│   ├── logger/
│   ├── auth/
│   ├── store/
│   ├── backup/
│   ├── i18n/
│   ├── model/
│   ├── utils/           # 工具函数（原 utils + toolnames）
│   ├── media/           # 媒体服务（原 media + ffmedia）
│   └── skillinstall/
├── api/                 # HTTP 处理器（顶层，不变）
├── app/                 # 应用入口（顶层，不变）
└── ctlclient/           # 控制客户端（顶层，不变）
```

---

## 包合并细节

### 1. task_scheduler + task_dispatcher → domain/task/

```
task_scheduler/scheduler.go          → domain/task/scheduler.go
task_dispatcher/dispatcher.go        → domain/task/dispatcher.go
task_dispatcher/failure_notifier.go  → domain/task/failure_notifier.go
task_dispatcher/task.go              → domain/task/task.go
```

包名：`package task`

### 2. claude + claudemd → ai/claude/

```
claude/download.go    → ai/claude/download.go
claude/invoker.go     → ai/claude/invoker.go
claude/provider.go    → ai/claude/provider.go
claudemd/claudemd.go  → ai/claude/claudemd.go
claudemd/bee.go       → ai/claude/claudemd_bee.go
claudemd/worker.go    → ai/claude/claudemd_worker.go
```

包名：`package claude`

### 3. media + ffmedia → infra/media/

```
media/service.go    → infra/media/service.go
ffmedia/ffmedia.go  → infra/media/ffmedia.go
```

包名：`package media`

### 4. utils + toolnames → infra/utils/

```
utils/checksum.go    → infra/utils/checksum.go
utils/download.go    → infra/utils/download.go
utils/http.go        → infra/utils/http.go
utils/ptr.go         → infra/utils/ptr.go
utils/version.go     → infra/utils/version.go
toolnames/toolnames.go → infra/utils/toolnames.go
```

包名：`package utils`

---

## Import 路径变更

模块：`github.com/theopenbee/openbee`

| 旧路径 | 新路径 |
|--------|--------|
| `.../internal/task_scheduler` | `.../internal/domain/task` |
| `.../internal/task_dispatcher` | `.../internal/domain/task` |
| `.../internal/bee` | `.../internal/domain/bee` |
| `.../internal/worker` | `.../internal/domain/worker` |
| `.../internal/msgingest` | `.../internal/domain/msgingest` |
| `.../internal/claude` | `.../internal/ai/claude` |
| `.../internal/claudemd` | `.../internal/ai/claude` |
| `.../internal/mcp` | `.../internal/ai/mcp` |
| `.../internal/config` | `.../internal/infra/config` |
| `.../internal/logger` | `.../internal/infra/logger` |
| `.../internal/auth` | `.../internal/infra/auth` |
| `.../internal/store` | `.../internal/infra/store` |
| `.../internal/backup` | `.../internal/infra/backup` |
| `.../internal/i18n` | `.../internal/infra/i18n` |
| `.../internal/model` | `.../internal/infra/model` |
| `.../internal/utils` | `.../internal/infra/utils` |
| `.../internal/toolnames` | `.../internal/infra/utils` |
| `.../internal/media` | `.../internal/infra/media` |
| `.../internal/ffmedia` | `.../internal/infra/media` |
| `.../internal/skillinstall` | `.../internal/infra/skillinstall` |

`api/`、`app/`、`ctlclient/` 路径不变，仅需更新其内部 import 语句。

---

## 迁移策略

按依赖顺序执行，每步完成后运行 `go build ./...` 验证编译。

**第 1 步：迁移 infra 类包**（被依赖最多，优先稳定）
- 移动 config, logger, auth, store, backup, i18n, model → `infra/`
- 合并 utils + toolnames → `infra/utils/`
- 合并 media + ffmedia → `infra/media/`
- 移动 skillinstall → `infra/skillinstall/`

**第 2 步：迁移 ai 类包**
- 合并 claude + claudemd → `ai/claude/`
- 移动 mcp → `ai/mcp/`

**第 3 步：迁移 domain 类包**
- 合并 task_scheduler + task_dispatcher → `domain/task/`
- 移动 bee → `domain/bee/`
- 移动 worker → `domain/worker/`
- 移动 msgingest → `domain/msgingest/`

**第 4 步：更新顶层包 import**
- 更新 api/, app/, ctlclient/ 内的所有 import 路径

---

## 设计原则

- **package name 不变**：包名保持原有短名（如 `task`、`claude`），只有 import 路径变化，对调用方影响最小
- **platform/ 不动**：平台适配器已经分组良好，无需变动
- **api/app/ctlclient/ 顶层保留**：作为应用入口，放在顶层更直观
