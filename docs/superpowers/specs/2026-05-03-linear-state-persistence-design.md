# Linear 平台状态文件持久化 —— 设计稿

- 日期：2026-05-03
- 分支：`feat/linear-platform`
- 作者：貂蝉（Worker）+ 老板

## 1. 背景

Linear 平台 receiver 当前持有两份运行时状态：

1. **Cursor**（poller 高水位线）
   - 位置：`internal/platform/linear/cursor.go`
   - 存储：DB 中的 `system_configs` 表，key=`linear.last_sync_at`（常量 `model.SystemConfigKeyLinearLastSync`）
   - 内容：`time.Time`，RFC3339Nano 字符串
2. **selfComments**（bot 自发评论 ID 集合，用于让 receiver 跳过自己的回复）
   - 位置：`internal/platform/linear/handler.go`
   - 存储：**进程内存**，ring buffer，`cap=1024`，FIFO 淘汰
   - 完全没有持久化

selfComments 没有持久化导致一个具体风险：进程重启后内存集合清空，下一轮 poll 会把 bot 自己刚发的评论当成用户输入再处理一次，触发循环回复。

## 2. 目标

- 把 cursor 和 selfComments 都持久化到 `~/.openbee/.linear/`，与现有 `~/.openbee/.codex/` `~/.openbee/.pi/` `~/.openbee/telegram/` 等模块约定保持一致。
- 修复 selfComments 重启后丢失的 bug。
- Linear 平台模块不再依赖 `SystemConfigStore`，运行时不再读写 `system_configs` 表。

## 3. 非目标

- **不**做 DB → 文件的迁移（应用未上线，无在用数据需要保留）。
- **不**做 fsync —— openbee 不是金融系统，正常退出走 close 已足够；硬宕机的最坏后果只是一次重复回复，不是数据损坏。
- **不**做淘汰 / 文件压缩 / rotate —— selfComments 永久保存。
- **不**引入第三方文件锁库。

## 4. 文件布局

```
~/.openbee/.linear/
├── cursor.json          # 单条 JSON：高水位线
└── self_comments.log    # 一行一个 comment ID（追加写）
```

### 4.1 `cursor.json`

```json
{"last_sync": "2026-05-03T10:00:00.000000000Z"}
```

- 字段：`last_sync`，RFC3339Nano UTC 字符串
- 写入策略：每次 `Save` 时先写 `cursor.json.tmp`，再 `os.Rename` 覆盖 `cursor.json`（POSIX 原子 rename）
- 读取策略：文件不存在 / parse 失败 → 返回 `time.Now().Add(-1 * time.Hour)`（保留现有 fallback 行为）

### 4.2 `self_comments.log`

- 每行一个 comment ID，行末 `\n`
- 写入：`os.OpenFile(path, O_WRONLY|O_CREATE|O_APPEND, 0o600)` 持有句柄；每次 Add 写一行
- POSIX 保证小于 PIPE_BUF（4 KB）的 append write 是原子的；comment ID 远小于 4 KB，多 goroutine 并发 append 不会互相切割
- 加载：启动时按行扫描，写入内存 `set`

### 4.3 目录权限

`os.MkdirAll(dir, 0o700)`，文件权限 `0o600`。

## 5. 组件改动

### 5.1 `internal/platform/linear/cursor.go`

去掉 `SystemConfigStore` 依赖，构造与方法签名调整：

- `NewCursor(dir string) *Cursor` —— 只依赖目录路径
- `Cursor` 结构体字段：`dir string`（不再持有 store）
- `Load(ctx) (time.Time, error)`：
  - 读 `<dir>/cursor.json`
  - `os.IsNotExist(err)` 或 JSON parse 失败 → 返回 `time.Now().Add(-1*time.Hour), nil`
  - 其他读错误 → 返回 error
- `Save(ctx, t) error`：
  - 序列化为 `{"last_sync": <RFC3339Nano UTC>}`
  - 写 `<dir>/cursor.json.tmp` → `os.Rename(...)` 覆盖
  - `os.MkdirAll(dir, 0o700)` 在写之前确保目录存在

### 5.2 `internal/platform/linear/handler.go` —— `selfComments`

- 取消 cap：删除常量 `selfCommentsCap`、字段 `order []string`、FIFO 淘汰逻辑
- 新增字段：
  - `path string`
  - `f *os.File`（O_APPEND 写句柄）
- 构造函数改为 `newSelfComments(dir string) (*selfComments, error)`：
  1. `os.MkdirAll(dir, 0o700)`
  2. 打开 `<dir>/self_comments.log` 用于读，逐行扫描进 `set`
  3. 用 `O_WRONLY|O_CREATE|O_APPEND` 打开同一文件，存到 `f`
- `Add(id)`：
  - 在已有的 `mu` 锁内执行
  - 已存在 → 直接返回
  - 否则：先写文件（`f.Write([]byte(id + "\n"))`），写成功再加入 `set`
  - 写失败：`log.Error` 但不 panic、不阻塞 Send 调用方；最坏情况是该 ID 没记到磁盘，重启后重复处理一次
- `Has(id)` 不变
- 不新增 `Close()`：进程退出时 OS 自动回收文件句柄；O_APPEND 写也不依赖关闭刷盘。如后续接入 graceful shutdown 再补

### 5.3 `internal/platform/linear/handler.go` —— `NewPlatform`

签名变更：

```go
// 旧
func NewPlatform(cfg config.LinearConfig, sysCfg *store.SystemConfigStore) platform.Platform

// 新
func NewPlatform(cfg config.LinearConfig) (platform.Platform, error)
```

实现：

- 计算目录：`filepath.Join(home, ".openbee", ".linear")`，沿用其他模块通过 `os.UserHomeDir()` 获取 home 的写法
- `cursor := NewCursor(dir)`
- `self, err := newSelfComments(dir)`；err 直接返回给上层 wiring（启动期失败优于运行时静默崩）
- 其余构造逻辑不变

### 5.4 上层 wiring

`internal/app/app.go` `buildPlatforms`：

- 当前签名：`buildPlatforms(... sysCfg *store.SystemConfigStore) []platform.Platform`
- 新签名：`buildPlatforms(...) ([]platform.Platform, error)` —— 去掉 `sysCfg` 参数，加上 error 返回（因为 `linear.NewPlatform` 现在会 error）
- 调用点 `linear.NewPlatform(lc, sysCfg)` 改为接收 `(p, err)`，err 直接 return 给 `buildPlatforms` 调用方
- `buildPlatforms` 的所有调用方同步改成处理 error；该函数全仓库只有一个 caller（在 `app.go` 内）

经 grep 确认 `sysCfg` 在 `buildPlatforms` 内除 linear 行外无其他使用，可安全摘除。

### 5.5 常量与依赖清理

- 删除 `internal/infra/model/system_config.go` 中的 `SystemConfigKeyLinearLastSync` 常量及其注释
- 经 grep，`linear.last_sync_at` / `SystemConfigKeyLinearLastSync` 只在 `system_config.go` 与 `cursor.go` 中被引用，无 i18n / 文档 / 配置使用
- `internal/platform/linear/cursor.go` 不再 import `internal/infra/model` `internal/infra/store`

## 6. 并发与崩溃安全

| 场景 | 处理 |
|---|---|
| `Cursor.Save` 写到一半进程崩溃 | tmp+rename 保证 `cursor.json` 要么旧版本要么新版本，不会半截 |
| `Cursor.Save` 多 goroutine 并发 | 调用方只在 `tickOnce` 内单 goroutine 调用，不加锁 |
| `selfComments.Add` 多 goroutine 并发 | 已有 `mu` 串行化；O_APPEND 也提供 OS 级原子保证，二者叠加足够 |
| 写入失败（磁盘满 / 权限丢失） | log error，不 panic；`Save` 返回 error 给调用方决定是否退出（保留现有行为）；`Add` 不向 Send 抛错 |
| 进程被 SIGKILL | 最近若干秒未刷盘的写丢失：cursor 丢一点 → 下次 poll 会重叠拉一段，幂等无副作用；self_comments 丢一两条 → 重启后那几条评论被重复处理一次 |

## 7. 测试

### 7.1 `cursor_test.go`

- 用 `t.TempDir()` 当 dir
- 文件不存在 → Load 返回 `now-1h ± 1s` 的时间
- Save 后 Load 往返一致
- 损坏的 JSON → Load 返回 `now-1h` fallback
- 验证 `cursor.json.tmp` 在 Save 成功后不再存在（已被 rename 覆盖原文件）

### 7.2 `handler_test.go`（涉及 selfComments 的部分）

- `newSelfComments(dir)` + `Add` 后用同一目录再 `newSelfComments` → `Has(id)` 返回 true（重启恢复）
- 100 个 goroutine 并发 `Add` 不同 ID → 全部 `Has(id)==true`，文件行数等于 ID 数（无丢失、无切割）
- 同一 ID 多次 `Add` → 文件中只出现一次

删除现有 cap=1024 / FIFO 淘汰相关的测试用例。

## 8. 迁移与回滚

- 应用未上线，无迁移路径需要支持
- 回滚：删除 `~/.openbee/.linear/` 目录后 revert 本次 commit 即可恢复 DB 行为；不留中间态
