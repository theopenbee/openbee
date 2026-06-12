# openbee service: 一键自启动子命令设计

- 日期：2026-06-11
- 范围：新增 `openbee service` 子命令组，将 openbee 注册为 macOS / Linux / Windows 用户级开机自启动
- 状态：草案，待评审

## 1. 背景与目标

当前 openbee 仅提供 `openbee server` 在后台启动 daemon，但无法在用户登录或系统启动时自动拉起。用户若想长期托管 openbee，需要手工编写 launchd plist、systemd unit 或 Windows 服务/任务计划，门槛高且容易出错。

本 spec 设计一个跨平台的 `openbee service` 命令组，封装三大桌面操作系统的「用户级」自启动注册流程，使用户只需 `openbee service install` 即可完成原本繁琐的手工配置。

### 非目标

- 不提供系统级（root / 管理员）自启动。需要系统级托管的场景请使用各平台原生工具。
- 不引入 systemd 之外的 Linux init 系统（OpenRC、SysV init、runit）。systemd user session 不可用时直接报错。
- 不实现内置的日志轮转。日志文件复用 `openbee server` 现有的 `~/.openbee/daemon.log`，轮转策略沿用现状。

## 2. 设计决策摘要

经过 brainstorming 与老板锁定的关键决策：

| 维度 | 决定 |
|---|---|
| 作用范围 | **仅用户级**（不需要 sudo / 管理员） |
| 命令风格 | `openbee service <action>`（systemctl 风格） |
| 子命令集 | `install` / `uninstall` / `start` / `stop` / `status` |
| Config 路径 | `install` 时显式 `--config <path>` 优先，否则用 `~/.openbee/config.yaml` |
| macOS 机制 | LaunchAgent (`~/Library/LaunchAgents/com.theopenbee.openbee.plist`) |
| Linux 机制 | systemd user unit (`~/.config/systemd/user/openbee.service`) |
| Windows 机制 | Task Scheduler 用户级任务（任务名 `OpenBee`） |
| start/stop 走向 | 委托给原生服务管理器（launchctl / systemctl --user / schtasks），不复用 openbee daemon 流程 |
| 崩溃自动重启 | 开启（`KeepAlive` / `Restart=on-failure` / Task Scheduler 失败重试） |
| 日志去向 | 复用 `~/.openbee/daemon.log` |
| Linux 回退 | systemd user session 不可用 → 直接报错并提示文档，**不**做 cron @reboot 等静默回退 |
| install 行为 | 默认「注册并立刻启动一次」，可加 `--no-start` 仅注册 |
| status 输出 | 三段：是否已安装 / 服务管理器视角的运行状态 / PID 与 uptime |

## 3. 用户视角的 CLI

```
openbee service install   [--config <path>] [--no-start] [--force]
openbee service uninstall
openbee service start
openbee service stop
openbee service status
```

### 子命令语义

- `install`：渲染并写入 plist / unit / task 文件；向服务管理器注册（launchctl bootstrap / systemctl --user enable / schtasks /Create）；除非加 `--no-start`，立刻启动一次（`enable --now` 等价语义）。若已存在同名注册，报错并提示使用 `--force` 覆盖。
- `uninstall`：先 stop，再向服务管理器注销，最后删除 plist / unit / task 文件。即使部分步骤失败，也尽量做完后续步骤并把所有错误汇总返回。
- `start`：调用服务管理器拉起进程。
- `stop`：调用服务管理器停止进程。
- `status`：输出三段信息：
  1. 自启动是否已注册（plist / unit / task 是否存在）
  2. 服务管理器视角下的运行状态（Running / Stopped / Failed / Unknown）
  3. 如在运行，输出 PID 和 uptime（uptime 通过 `ps -o etime` 或 `Get-Process` 推断；获取失败时省略不报错）

### 参数

- `--config <path>`：仅 `install` 接受。显式给出时使用该路径并记录到 plist/unit/task 的 ProgramArguments。未给出时回退到 `~/.openbee/config.yaml`，且若该文件不存在则报错提示先运行 `openbee config`。
- `--no-start`：仅 `install` 接受。注册后不立即启动。
- `--force`：仅 `install` 接受。覆盖已存在的注册。

### 与现有 `openbee server / stop / status` 的关系

- `openbee server` 仍保留：用户若不需要自启动，可继续手动启停。
- 一旦 `service install` 完成，运行时由服务管理器接管。手动跑 `openbee server` 会因为 PID 文件已被占用而拒绝启动（沿用现状的 `IsProcessAlive` 检测）。
- `openbee status` 仍展示 daemon PID 视角；`openbee service status` 额外报告「是否被服务管理器拉起」。两者不冲突。

## 4. 代码结构

新增包 `cmd/openbee/internal/cli/servicecmd`，与 `daemoncmd` 平级。

```
cmd/openbee/internal/cli/servicecmd/
  command.go              # NewCommand() 装配 install/uninstall/start/stop/status 子命令
  install.go              # install action handler
  uninstall.go            # uninstall action handler
  start.go                # start action handler
  stop.go                 # stop action handler
  status.go               # status action handler
  manager.go              # Manager 接口、Status / InstallOptions 类型
  manager_darwin.go       # build:darwin   launchd 实现
  manager_linux.go        # build:linux    systemd --user 实现
  manager_windows.go      # build:windows  Task Scheduler 实现
  manager_other.go        # build:!darwin,!linux,!windows  stub，全部返回不支持错误
  templates/
    launchd.plist.tmpl
    systemd.service.tmpl
    schtask.xml.tmpl
  manager_darwin_test.go  # 模板渲染快照 + 命令字符串构造测试
  manager_linux_test.go
  manager_windows_test.go
  install_test.go         # 命令层用假 Manager 验证流程
```

在 `cmd/openbee/internal/cli/root.go` 增加：

```go
root.AddCommand(servicecmd.NewCommand())
```

## 5. 平台无关抽象

```go
// manager.go
type RunState int

const (
    RunStateUnknown RunState = iota
    RunStateStopped
    RunStateRunning
    RunStateFailed
)

type Status struct {
    Installed  bool
    RunState   RunState
    PID        int    // 0 if not running
    UptimeSecs int64  // 0 if unknown
}

type InstallOptions struct {
    ExePath    string  // 绝对路径，来自 utils.ResolveExecutable()
    ConfigPath string  // 绝对路径
    LogPath    string  // 绝对路径，~/.openbee/daemon.log
    AutoStart  bool    // false 表示 --no-start
    Force      bool    // 是否覆盖已有注册
}

type Manager interface {
    Install(ctx context.Context, opts InstallOptions) error
    Uninstall(ctx context.Context) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Status(ctx context.Context) (Status, error)
}

// NewManager 由 build tag 选中平台实现
func NewManager() (Manager, error)
```

命令层（`install.go` 等）的职责：解析 flag → 构造 `InstallOptions` → 调用 `Manager` → 渲染 i18n 文案。不直接拼接 plist / unit / XML。

## 6. 三平台实现要点

### 6.1 macOS（launchd LaunchAgent）

- Plist 路径：`~/Library/LaunchAgents/com.theopenbee.openbee.plist`
- Label：`com.theopenbee.openbee`
- 关键字段：
  - `ProgramArguments`：`[<exe>, server, -c, <config>]`
  - `RunAtLoad`：`true`
  - `KeepAlive`：`true`
  - `ThrottleInterval`：`10`
  - `StandardOutPath` / `StandardErrorPath`：`~/.openbee/daemon.log`
  - `EnvironmentVariables`：注入 `HOME`、`PATH`（取自当前进程 env，确保 daemon 与交互 shell 行为一致）
- 命令：
  - 注册：`launchctl bootstrap gui/$UID <plist>`
  - 注销：`launchctl bootout gui/$UID/com.theopenbee.openbee`
  - 启动：`launchctl kickstart gui/$UID/com.theopenbee.openbee`
  - 停止：`launchctl kill SIGTERM gui/$UID/com.theopenbee.openbee`
  - 状态：`launchctl print gui/$UID/com.theopenbee.openbee`（解析 `state =`、`pid =`）

### 6.2 Linux（systemd --user）

- Unit 路径：`~/.config/systemd/user/openbee.service`
- 关键字段：
  - `[Service]`
    - `Type=simple`
    - `ExecStart=<exe> server -c <config>`
    - `Restart=on-failure`
    - `RestartSec=10`
    - `StandardOutput=append:<log>`
    - `StandardError=append:<log>`
    - `Environment=HOME=<home>`
  - `[Install]`
    - `WantedBy=default.target`
- 命令：
  - 注册：`systemctl --user daemon-reload && systemctl --user enable [--now] openbee.service`
  - 注销：`systemctl --user disable --now openbee.service && systemctl --user daemon-reload`
  - 启动：`systemctl --user start openbee.service`
  - 停止：`systemctl --user stop openbee.service`
  - 状态：`systemctl --user show -p ActiveState,SubState,MainPID,ExecMainStartTimestampMonotonic openbee.service`
- 预检：执行 `systemctl --user is-system-running` 或检测 `DBUS_SESSION_BUS_ADDRESS`，失败则报错并提示用户开启 lingering / 启用 systemd（包含文档链接）。

### 6.3 Windows（Task Scheduler）

- 任务名：`OpenBee`
- 创建方式：渲染 task XML → `schtasks /Create /XML <tmpfile> /TN OpenBee /F`（避免命令行参数拼接陷阱）
- XML 关键节点：
  - `Triggers/LogonTrigger`：登录时触发，限定当前 UserId
  - `Principals/Principal`：`RunLevel=LeastPrivilege`，`LogonType=InteractiveToken`
  - `Settings/RestartOnFailure`：`Interval=PT1M`、`Count=3`
  - `Settings/Hidden`：`true`（不显示在登录后的可见窗口里）
  - `Actions/Exec`：
    - `Command`：`cmd.exe`
    - `Arguments`：`/c ""<exe>" server -c "<config>" >> "<log>" 2>&1"`
- 命令：
  - 注册：`schtasks /Create /XML <file> /TN OpenBee /F`
  - 注销：`schtasks /Delete /TN OpenBee /F`
  - 启动：`schtasks /Run /TN OpenBee`
  - 停止：`schtasks /End /TN OpenBee`
  - 状态：`schtasks /Query /TN OpenBee /V /FO LIST`（解析 `Status:` 字段；PID 通过 `tasklist /FI "IMAGENAME eq openbee.exe"` 兜底）

## 7. 日志、PID、并发约束

- 所有平台 stdout/stderr 复用 `~/.openbee/daemon.log`，与 `openbee server` 现有日志合并。
- 服务管理器拉起的 daemon 仍走现有 `server` 的代码路径，会写 `~/.openbee/openbee.pid`。`openbee status` 因此天然可见。
- 重复运行保护沿用 `daemoncmd` 现有逻辑：`server` 启动时若 PID 文件存活则拒绝。`service install --no-start` 后再 `openbee server` 也会因后续 `service start` 启动后 PID 占用而被拒绝。

## 8. 错误处理与文案

- 通过 i18n.M.Output.Service.* 提供新文案 key；模仿现有 `Output.Daemon.*` 的组织方式。
- 典型错误信息：
  - 已存在注册：`service already installed, use 'openbee service uninstall' first or pass --force`
  - 缺少 config：`config file not found at <path>, run 'openbee config' first`
  - Linux systemd 不可用：`systemd user session unavailable; see docs/service-autostart.md for prerequisites`
  - 服务管理器调用失败：原样回传 stderr 并提示 `~/.openbee/daemon.log` 查看更多。

## 9. 测试策略

- **单元测试**：
  - 模板渲染：每个平台一组「输入 `InstallOptions` → 期望文本」快照测试。
  - 命令层：注入假 `Manager`（实现接口的 stub），验证 flag 解析、错误传播、`--no-start` 与 `--force` 等开关行为。
- **平台集成测试**：
  - macOS / Linux / Windows 各自有 `//go:build` 限定的集成测试，跑真实的 `launchctl` / `systemctl --user` / `schtasks`，但只在对应 OS 的 CI job 上启用。
  - 集成测试操作隔离前缀：`com.theopenbee.openbee.test`、`openbee-test.service`、`OpenBeeTest`，避免污染开发者本地环境。
- **不测试**：操作系统服务管理器自身的正确性、日志轮转。

## 10. 文档

- README 增加「自启动」章节，给出三平台 install/uninstall 示例。
- 新增 `docs/service-autostart.md`：详细说明三平台预备条件、systemd lingering 提示、Windows 任务的可见位置（任务计划程序 → 任务计划程序库 → OpenBee）等。

## 11. 实施分阶段建议

1. **阶段 1**：抽象接口 + 命令骨架 + macOS 实现（开发者本机最容易调试）。
2. **阶段 2**：Linux 实现 + 集成测试。
3. **阶段 3**：Windows 实现 + 集成测试。
4. **阶段 4**：文档、i18n 文案、README 更新。

每个阶段独立可发布，互不阻塞。

## 12. 未决事项 / 显式不做

- Linux 上不引入 OpenRC / SysV 兼容。
- 不做 GUI 安装器。
- 不做 `openbee service logs` 子命令（直接看 `~/.openbee/daemon.log` 即可）。
- 不做 `openbee service edit` 子命令（用户若需自定义，建议直接改 plist/unit/task 后用平台原生工具管理）。
