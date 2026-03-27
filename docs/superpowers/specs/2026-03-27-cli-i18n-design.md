# CLI 多语言（i18n）支持设计规范

**日期：** 2026-03-27
**范围：** CLI 层（cobra 命令描述 + survey 交互提示）
**状态：** 已批准

---

## 背景与目标

openbee 当前所有 CLI 文本均为中文硬编码，包括 cobra 命令的 Short/Long 描述、`openbee config` 交互提示以及 CLI 错误消息。随着 openbee 面向更广泛用户（包括非中文用户），需要支持多语言界面。

**目标：**
- 支持 zh（中文，默认）和 en（英文）两种语言
- 语言检测采用组合优先级策略，用户可灵活控制
- 目录结构为未来扩展更多语言预留空间
- 不引入外部 i18n 依赖，保持轻量

**非目标：**
- AI 系统提示词（bee.go）国际化（不在本次范围内）
- IM 平台通知消息国际化（不在本次范围内）
- 复数形式、日期/货币格式化等复杂 i18n 特性

---

## 语言检测策略

语言按以下优先级从高到低选取（高优先级覆盖低优先级）：

```
--lang flag（CLI 参数，最高优先级）
    ↓
OPENBEE_LANG 环境变量
    ↓
config.yaml 中的 language 字段
    ↓
默认语言：zh
```

这符合 12-factor app 惯例，适合开发环境（env var）、服务器部署（config）、临时调试（flag）等不同场景。

---

## 目录结构

```
internal/i18n/
  locales/
    zh.yaml          # 中文翻译（默认语言，必须完整）
    en.yaml          # 英文翻译
  messages.go        # Messages 结构体定义
  i18n.go            # Load()、M 变量、embed 声明

cmd/openbee/
  lang.go            # detectLang() 函数
  main.go            # 启动时调用 i18n.Load(detectLang())
```

---

## 核心数据结构

### `internal/i18n/messages.go`

```go
package i18n

// Messages 包含所有 CLI 用户可见文本。
// 字段名与 YAML 键名通过 yaml tag 对应。
type Messages struct {
    Cmd    CmdMessages    `yaml:"cmd"`
    Prompt PromptMessages `yaml:"prompt"`
    Error  ErrorMessages  `yaml:"error"`
}

type CmdMessages struct {
    Root   struct {
        Short string `yaml:"short"`
        Long  string `yaml:"long"`
    } `yaml:"root"`
    Config struct {
        Short string `yaml:"short"`
        Long  string `yaml:"long"`
    } `yaml:"config"`
    Start  struct{ Short string `yaml:"short"` } `yaml:"start"`
    Stop   struct{ Short string `yaml:"short"` } `yaml:"stop"`
    Status struct{ Short string `yaml:"short"` } `yaml:"status"`
    // 按需扩展其他命令
}

type PromptMessages struct {
    ServerPort  string `yaml:"server_port"`
    ServerHost  string `yaml:"server_host"`
    DBPath      string `yaml:"db_path"`
    MCPAPIKey   string `yaml:"mcp_api_key"`
    WorkerAPIKey string `yaml:"worker_api_key"`
    // ... 其余 survey 提示字段
}

type ErrorMessages struct {
    InvalidLang    string `yaml:"invalid_lang"`
    ConfigNotFound string `yaml:"config_not_found"`
}
```

### YAML 文件示例

**`locales/zh.yaml`**
```yaml
cmd:
  root:
    short: "OpenBee 核心服务"
  config:
    short: "交互式生成配置文件"
    long: "通过交互式问答生成 openbee 配置文件"
  start:
    short: "启动 openbee 守护进程"
  stop:
    short: "停止 openbee 守护进程"
prompt:
  server_port: "服务端口"
  server_host: "服务地址"
  db_path: "数据库路径"
error:
  invalid_lang: "不支持的语言：%s，支持的语言：zh, en"
  config_not_found: "配置文件不存在：%s"
```

**`locales/en.yaml`**
```yaml
cmd:
  root:
    short: "OpenBee core service"
  config:
    short: "Interactively generate a config file"
    long: "Generate an openbee config file via interactive prompts"
  start:
    short: "Start the openbee daemon"
  stop:
    short: "Stop the openbee daemon"
prompt:
  server_port: "Server port"
  server_host: "Server host"
  db_path: "Database path"
error:
  invalid_lang: "Unsupported language: %s, supported: zh, en"
  config_not_found: "Config file not found: %s"
```

---

## 加载机制

### `internal/i18n/i18n.go`

```go
package i18n

import (
    "embed"
    "fmt"

    "gopkg.in/yaml.v3"
)

//go:embed locales/*.yaml
var localesFS embed.FS

// M 是全局翻译实例，在 main() 最早阶段初始化，之后只读。
var M *Messages

// SupportedLangs 列出所有支持的语言代码。
var SupportedLangs = []string{"zh", "en"}

// Load 加载指定语言的翻译。若语言不支持，fallback 到 zh。
func Load(lang string) error {
    data, err := localesFS.ReadFile("locales/" + lang + ".yaml")
    if err != nil {
        // 不支持的语言，静默 fallback 到默认语言
        data, err = localesFS.ReadFile("locales/zh.yaml")
        if err != nil {
            return fmt.Errorf("i18n: failed to load default locale: %w", err)
        }
    }
    M = &Messages{}
    return yaml.Unmarshal(data, M)
}
```

### `cmd/openbee/lang.go`

```go
package main

import (
    "os"

    "github.com/theopenbee/openbee/internal/config"
)

// detectLang 按优先级确定界面语言：flag > 环境变量 > config > 默认。
func detectLang(flagLang string) string {
    if flagLang != "" {
        return flagLang
    }
    if env := os.Getenv("OPENBEE_LANG"); env != "" {
        return env
    }
    if cfg := config.GetLang(); cfg != "" {
        return cfg
    }
    return "zh"
}
```

### `cmd/openbee/main.go` 启动序列

cobra 的 `Short`/`Long` 字段在各文件的 `init()` 阶段赋值，早于任何命令执行。因此语言加载必须在 `Execute()` 之前完成，并通过 `applyTranslations()` 统一覆写命令描述字段（详见"与 cobra 集成"一节）。

---

## 与 cobra 集成

由于 cobra 的描述字段在 `init()` 中设置，需要在 `init()` 被调用前完成语言加载。推荐做法：

```go
// cmd/openbee/main.go
func main() {
    // 1. 先解析 --lang flag（不触发 cobra 执行）
    langFlag := parseLangFlag(os.Args)

    // 2. 加载翻译
    i18n.Load(detectLang(langFlag))

    // 3. 设置所有命令描述（cobra init() 之后统一赋值）
    applyTranslations()

    // 4. 正常执行
    rootCmd.Execute()
}

func applyTranslations() {
    m := i18n.M
    rootCmd.Short = m.Cmd.Root.Short
    configCmd.Short = m.Cmd.Config.Short
    configCmd.Long = m.Cmd.Config.Long
    // ...
}
```

---

## 与 survey 集成

survey 提示文字直接在 `runConfig()` 中使用：

```go
survey.AskOne(&survey.Input{
    Message: i18n.M.Prompt.ServerPort,
    Default: "8080",
}, &values.ServerPort)
```

由于 survey 在命令执行时才调用（不在 `init()` 阶段），不存在时序问题。

---

## Fallback 策略

| 场景 | 行为 |
|------|------|
| 语言有效（zh/en） | 加载对应 YAML |
| 语言无效（如 `fr`） | 静默 fallback 到 zh，不报错 |
| YAML 文件损坏 | 返回错误，程序启动失败 |
| 字段值为空字符串 | 保留空字符串（不 fallback 到另一语言的同字段） |

---

## config.yaml 扩展

在配置文件中新增可选字段：

```yaml
# 界面语言，可选值：zh（默认）、en
language: en
```

`config.GetLang()` 读取该字段，返回空字符串表示未设置。

---

## 扩展新语言的流程

1. 在 `internal/i18n/locales/` 新建 `<lang>.yaml`，复制 `zh.yaml` 结构并翻译
2. 将新语言代码加入 `i18n.SupportedLangs`
3. 无需修改任何 Go 代码

---

## 测试策略

- 单元测试：`i18n.Load("en")`、`i18n.Load("zh")`、`i18n.Load("invalid")` 各场景
- 集成测试：设置 `OPENBEE_LANG=en` 后运行 `openbee --help`，验证输出为英文
- 快照测试：对比不同语言下 `openbee --help` 的输出内容
