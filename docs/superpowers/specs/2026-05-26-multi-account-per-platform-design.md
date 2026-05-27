# 多账号支持设计（每平台 N 个账号）

## 背景与目标

当前 openbee 每个平台只支持一个账号配置：飞书一个 app、企微一个 bot、微信一个账号。但实际使用场景中：

- 飞书可以在一个开发者后台下创建多个机器人（不同用途、不同部门）
- 微信可以同时接入多个账号
- 企微同样支持多个 bot

目标：让每个平台能在 `config.yaml` 中配置任意数量的账号，且各账号的会话、消息、任务互不干扰。

## 设计原则

1. **完全隔离 per-account 资源**：消息、会话、任务、执行记录都按账号隔离
2. **全局共享基础设施**：worker、engine 配置、env 变量、departments、auth 保持全局
3. **强制迁移**：YAML 必须改成 list 格式才能启动；DB 一次性 migration 把老数据填 `account_name="default"`
4. **平台对称**：所有 IM 平台（含 Linear）统一走 list 化路径，避免特例

## 资源边界

| 维度 | 资源 |
|---|---|
| **全局（不带 account_name）** | engine 配置、env 变量、departments、worker、auth、system_config、token_stats |
| **Per-account（带 account_name）** | messages、outbound_messages、sessions、tasks、executions |

含义：同一个 worker 可被任何账号/任何平台的会话引用；session 内容、消息历史、任务调度记录均按账号隔离。

## 配置 Schema

`PlatformsConfig` 每个平台字段从单元素结构体变为 list。

```yaml
bee:
  platforms:
    feishu:
      - name: marketing-bot        # 平台内唯一，[a-z0-9_-]
        enabled: true
        app_id: cli_xxx
        app_secret: xxx
        bot_name: "营销小蜜"
        max_media_size: 104857600
      - name: support-bot
        enabled: true
        app_id: cli_yyy
        app_secret: yyy
        bot_name: "客服小蜜"
    wecom:
      - name: default
        enabled: true
        bot_id: ...
        secret: ...
        bot_name: "..."
    weixin: []                     # 空 list = 该平台不启用
    dingtalk: []
    telegram: []
    linear: []
```

### 约束

- `name` 平台内唯一；启动时校验，重名直接报错退出
- `name` 字符集 `[a-z0-9_-]`
- 缺 `name` 字段 → 启动报错
- 老格式（`feishu: {...}` 而非 `feishu: [...]`）→ 启动报错，提示升级
- `local` 平台不变（固定单实例，无凭据）
- 跨平台允许重名（`feishu/marketing-bot` 与 `wecom/marketing-bot` 共存合法）
- DB 复合标识：`(platform, account_name)`

## 数据模型

新增 `account_name TEXT NOT NULL DEFAULT 'default'` 列：

| 表 | 用途 |
|---|---|
| `bee_platform_messages` | 入站消息记录 |
| `bee_outbound_messages` | 出站消息记录 |
| `bee_sessions` | 会话状态 |
| `bee_tasks` | 任务调度记录（冗余存便于查询） |
| `bee_worker_executions` | 执行记录 |

### 索引

- 现有 `(platform, ...)` 索引扩成 `(platform, account_name, ...)`
- SessionKey 本身已包含 account_name（见下），但额外索引便于按账号过滤

### 不动的表

`bee_workers`、`bee_departments`、`bee_env_configs`、`bee_system_configs`、`bee_token_stats`、`bee_constraints`。

## 标识传播

### SessionKey 格式

- 旧：`feishu:<chatID>:<userID>`
- 新：`feishu:<account_name>:<chatID>:<userID>`
- Linear 旧 `linear:<teamKey>:<identifier>` → 新 `linear:<account_name>:<teamKey>:<identifier>`

各平台 handler 里构造 SessionKey 的位置都改造。

### InboundMessage

```go
type InboundMessage struct {
    Platform     string  // "feishu"
    AccountName  string  // 新增："marketing-bot"
    SenderID     string
    SessionKey   string  // "feishu:marketing-bot:chatID:userID"
    Content      string
    RawContent   string
    Raw          string
    PlatformMessageID string
    MessageTime  int64
}
```

### OutboundMessage 路由

- `sendersByPlatform map[string]PlatformSenderAdapter` → `sendersByAccount map[string]PlatformSenderAdapter`
- key 格式 `"<platform>:<account_name>"`，例如 `"feishu:marketing-bot"`
- 所有使用 `sendersByPlatform` 的位置（5 个 command handler、`PlatformFailureNotifier`、`buildAPIServer` 注入、`beeRPCSrv` 注入）按账号 key 查
- OutboundMessage 路由 key 派生自 `msg.ReplyTo.Platform + ":" + msg.ReplyTo.AccountName`

### Local 平台

`AccountName = "default"`，固定占位，保证逻辑统一。

## 装配（app.go）

- `buildPlatforms` 从"每平台调一次构造函数"改为"对每个平台的 list 循环构造"
- 每个 account 构造一对 `(Receiver, Sender)`，注册到 `sendersByAccount`
- 每个 receiver 独立 goroutine（飞书每个 bot 有独立长连接，企微每个 bot 有独立 ws）
- `ingest.WithPlatformBotNames` 改名为 `WithAccountBotNames`，key 为 `"<platform>:<account_name>"`，用于剥离 @mention

伪代码：

```go
for _, fc := range cfg.Bee.Platforms.Feishu {
    if !fc.Enabled { continue }
    p := feishu.NewPlatform(fc, mediaSvc)  // fc 现含 Name 字段
    key := platformAccountKey(feishu.PlatformID, fc.Name)
    sendersByAccount[key] = store.NewLoggingPlatformSenderAdapter(
        p.Sender(), s.outboundMsgStore, key)
    platforms = append(platforms, p)
}
```

## 命令可见范围

- `+list`、`+status`：**全部 worker**（全局视图）
- 隔离的是会话内容和任务记录，不是 worker 发现层
- 同一个 worker 在不同 account 各自的 session 是独立的

## 迁移

### YAML

- 启动时检测到 `feishu`、`wecom` 等不是 list → 报错并给出明确升级指引
- list 为空（`feishu: []`）= 该平台禁用，等同于无 receiver

### DB Migration（一次性）

- 新增 migration 文件（按现有 migration 命名规则）
- `ALTER TABLE` 给 5 张表加 `account_name TEXT NOT NULL DEFAULT 'default'`
- 老数据由 DEFAULT 子句自动填 `'default'`
- 重建相关索引

### 老用户操作流程

1. 把现有 `feishu: {app_id: ..., app_secret: ...}` 改成 `feishu: [{name: default, app_id: ..., app_secret: ...}]`
2. 启动 → migration 自动执行 → 老数据全部 `account_name='default'`，业务行为不变

## 测试策略

### 单测

- `config_test.go`：list 格式解析、`name` 重名校验、老格式（非 list）报错、空 list 视为禁用
- 各平台 `handler_test.go`：SessionKey 构造包含 account_name；InboundMessage 携带 AccountName
- `command/*_test.go`（engine/clear/stop/status/list）：路由按 `<platform>:<account_name>` key 查 sender
- `failure_notifier_test.go`：失败消息回到正确账号
- `msgingest_test.go`：bot_name 剥离按账号查表

### 集成测试

- 启动两个 feishu account（mock receiver），各自发消息 → 各自创建独立 session/message，互不污染
- 同一个 worker 被两个账号引用 → 各自的 session 独立、各自的 execution 独立
- DB migration 测试：从老 schema + 老数据起步，跑 migration 后老数据全部带 `account_name='default'`

### 回归测试

- 老 yaml 单 bot 用例（升级成 list 含 `name: default`）行为完全不变
- `+list`、`+status` 等命令在多账号下输出正确

## 影响范围（粗估）

主要改动文件：

- `internal/infra/config/config.go`、`config.yaml.tmpl`：list 化所有平台 config
- `internal/platform/interfaces.go`：InboundMessage 增加 AccountName
- `internal/platform/{feishu,dingtalk,wecom,weixin,telegram,linear}/handler.go`：SessionKey 构造、account_name 注入
- `internal/app/app.go`：buildPlatforms 改 loop；sendersByPlatform → sendersByAccount
- `internal/infra/store/*.go`：5 张表的 store 增加 account_name 字段读写
- `internal/infra/store/db.go`：migration 增加 ALTER TABLE
- `internal/domain/command/*.go`：所有用 sendersByPlatform 的命令 handler 改 key
- `internal/domain/task/failure_notifier.go`：路由 key 更新
- `internal/domain/msgingest/*.go`：bot_name 表按账号查
- 各对应 `*_test.go`

不动：worker store / department store / env service / engine 注册 / web UI（worker/department/engine 视图不变；session/message/task 视图未来可加 account 过滤，本期不做）。

## 非目标（Out of Scope）

- Web UI 增加"按账号过滤"的查询能力（本期 worker/engine 视图不动；future work）
- 账号热加载（增删账号需重启进程）
- 跨账号 worker 调用权限控制
- 账号级别的指标/审计
- 多账号下的 Web 管理 UI

未来如需热加载或 Web 管理，可在本设计基础上把 YAML list 迁移到 DB（保留 YAML 作为 bootstrap 入口）。
