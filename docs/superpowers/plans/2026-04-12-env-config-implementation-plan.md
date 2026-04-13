# Env 配置功能实现计划

**关联 spec：** `docs/superpowers/specs/2026-04-12-env-config-design.md`  
**日期：** 2026-04-12

---

## 实现步骤

### Step 1 — 配置层：新增 env_secret

**文件：** `internal/infra/config/config.go`

- 在 `AdvancedConfig`（或合适的顶层结构体）中新增字段：
  ```go
  EnvSecret string `yaml:"env_secret"`
  ```
- 在 `applyDefaults()` 中添加自动生成逻辑（与 mcp_token_secret 一致）：
  ```go
  if cfg.Advanced.EnvSecret == "" {
      cfg.Advanced.EnvSecret = GenerateRandomSecret()
  }
  ```
- 在 `config.yaml` 模板中添加注释说明该字段

---

### Step 2 — 数据库迁移：创建 bee_env_configs 表

**文件：** `internal/infra/store/db.go`

在 `migrations` 切片末尾追加 migration #30：

```go
{
    Version: 30,
    Name:    "create_bee_env_configs_table",
    SQL: `
        CREATE TABLE IF NOT EXISTS bee_env_configs (
            id          TEXT PRIMARY KEY,
            scope       TEXT NOT NULL,
            scope_id    TEXT,
            key         TEXT NOT NULL,
            enc_value   TEXT NOT NULL,
            masked      TEXT NOT NULL,
            created_at  INTEGER NOT NULL,
            updated_at  INTEGER NOT NULL,
            UNIQUE(scope, scope_id, key)
        );
        CREATE INDEX IF NOT EXISTS idx_bee_env_configs_scope
            ON bee_env_configs(scope, scope_id);
    `,
},
```

注意：`scope` 合法值为 `'global'`、`'bee'`、`'department'`、`'worker'`；`scope_id` 在 global 时存 NULL。

---

### Step 3 — 加密工具：AES-256-GCM

**新建文件：** `internal/infra/crypto/aes.go`

实现两个函数：

```go
package crypto

// Encrypt 使用 AES-256-GCM 加密明文，返回 base64 编码的密文（nonce 前缀）
func Encrypt(key, plaintext string) (string, error)

// Decrypt 解密 Encrypt 产生的密文，返回明文
func Decrypt(key, ciphertext string) (string, error)
```

实现要点：
- key 为 hex 字符串（32 字节 = 64 hex 字符），`hex.DecodeString` 解码
- 随机生成 12 字节 nonce，前缀拼接到密文
- 输出：`base64(nonce + ciphertext_with_tag)`

另实现打码工具函数：

```go
// Mask 生成打码展示值
// 规则：保留前 4 位和后 4 位，中间替换为 ****；总长 < 8 位则全部打码
func Mask(value string) string
```

---

### Step 4 — 数据模型

**新建文件：** `internal/infra/model/env_config.go`

```go
package model

type EnvConfig struct {
    ID        string `json:"id"         db:"id"`
    Scope     string `json:"scope"      db:"scope"`
    ScopeID   string `json:"scope_id"   db:"scope_id"`
    Key       string `json:"key"        db:"key"`
    EncValue  string `json:"-"          db:"enc_value"`
    Masked    string `json:"masked"     db:"masked"`
    CreatedAt int64  `json:"created_at" db:"created_at"`
    UpdatedAt int64  `json:"updated_at" db:"updated_at"`
}
```

---

### Step 5 — Store 层

**新建文件：** `internal/infra/store/env_config_store.go`

```go
type EnvConfigStore struct {
    db *sql.DB
}

func NewEnvConfigStore(db *sql.DB) *EnvConfigStore

// 方法：
func (s *EnvConfigStore) Create(cfg *model.EnvConfig) error
func (s *EnvConfigStore) List(scope, scopeID string) ([]*model.EnvConfig, error)
func (s *EnvConfigStore) Get(id string) (*model.EnvConfig, error)
func (s *EnvConfigStore) Update(id, encValue, masked string) error
func (s *EnvConfigStore) Delete(id string) error

// 用于运行时解析：获取 worker 所属部门中 scope=department 的 env（按 scope_id 字典序排序）
func (s *EnvConfigStore) ListForDepartments(departmentIDs []string) ([]*model.EnvConfig, error)
```

实现规范：
- 时间戳用 `time.Now().UnixMilli()`
- ID 用 UUID（参考现有 worker store 中的 ID 生成方式）
- 错误用 `fmt.Errorf("xxx: %w", err)` 包裹

---

### Step 6 — Domain 服务层

**新建文件：** `internal/domain/env/service.go`

```go
type Service struct {
    store     *store.EnvConfigStore
    workerStore *store.WorkerStore   // 用于查 worker 所属部门
    deptStore *store.DepartmentStore
    encKey    string                  // config.Advanced.EnvSecret
}

func NewService(envStore *store.EnvConfigStore, ws *store.WorkerStore, ds *store.DepartmentStore, encKey string) *Service

// Create 加密明文值并持久化
func (s *Service) Create(scope, scopeID, key, plainValue string) (*model.EnvConfig, error)

// UpdateValue 更新 env 值（重新加密）
func (s *Service) UpdateValue(id, plainValue string) error

// ResolveWorkerEnv 返回 Worker 执行时的完整环境变量（KEY=VALUE 格式切片）
// 解析链：global ← department（按 dept_id 字典序）← worker
func (s *Service) ResolveWorkerEnv(workerID string) ([]string, error)

// ResolveBeeEnv 返回 Bee 执行时的完整环境变量（KEY=VALUE 格式切片）
// 解析链：global ← bee
func (s *Service) ResolveBeeEnv(beeID string) ([]string, error)

// 内部 merge 函数：后者覆盖前者同名 key
func merge(layers ...[]*model.EnvConfig) map[string]string
```

**验证：** `Create` 和 `UpdateValue` 时检查 key 不得为 `OPENBEE_API_KEY`，否则返回错误。

---

### Step 7 — 传递 ExtraEnv 到进程调用链

**文件：** `internal/ai/` 下的 RunOptions

在 `ai.RunOptions` 结构体中新增字段：

```go
type RunOptions struct {
    SessionID string
    Resume    bool
    APIKey    string
    ExtraEnv  []string  // 新增：额外注入的 KEY=VALUE 环境变量
}
```

**修改各 Invoker：**（`claude/invoker.go`、`codex/invoker.go`、`pi/invoker.go`）

在设置 `cmd.Env` 时追加 ExtraEnv：

```go
cmd.Env = append(inv.baseEnv, opts.ExtraEnv...)
cmd.Env = append(cmd.Env, "OPENBEE_API_KEY="+opts.APIKey)
// OPENBEE_API_KEY 最后追加，确保不被 ExtraEnv 覆盖
```

---

### Step 8 — Worker 执行时注入

**文件：** `internal/domain/worker/manager.go`

1. 在 `Manager` 结构体中添加 `envService *env.Service` 字段
2. 在 `NewManager` 构造函数中接收并赋值 `envService`
3. 在 `launchRuntime` 中调用：

```go
extraEnv, err := m.envService.ResolveWorkerEnv(worker.ID)
if err != nil {
    return fmt.Errorf("resolve worker env: %w", err)
}

proc, outputCh, err := m.engine.Run(execCtx, worker.WorkDir, prompt, ai.RunOptions{
    SessionID: exec.SessionID,
    Resume:    resume,
    APIKey:    token,
    ExtraEnv:  extraEnv,  // 新增
}, logPath)
```

---

### Step 9 — Bee 执行时注入

**文件：** `internal/domain/bee/bee_process.go`

1. 在 `BeeProcess` 结构体中添加 `envService *env.Service` 和 `beeID string` 字段
2. 在 `NewBeeProcess` 中接收并赋值（beeID 从 BeeConfig 或调用方传入）
3. 在 `Run` 方法中注入：

```go
func (p *BeeProcess) Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
    token, err := auth.GenerateBeeToken(p.tokenSecret, p.tokenTTL)
    if err != nil {
        return nil, nil, fmt.Errorf("generate bee token: %w", err)
    }

    extraEnv, err := p.envService.ResolveBeeEnv(p.beeID)
    if err != nil {
        return nil, nil, fmt.Errorf("resolve bee env: %w", err)
    }

    opts.APIKey = token
    opts.ExtraEnv = extraEnv
    return p.engine.Run(ctx, workDir, prompt, opts, logPath)
}
```

---

### Step 10 — API Handler

**新建文件：** `internal/api/env_handler.go`

```go
type EnvHandler struct {
    svc *env.Service
}

func NewEnvHandler(svc *env.Service) *EnvHandler
```

**请求/响应结构体：**

```go
type createEnvRequest struct {
    Scope   string `json:"scope"    binding:"required,oneof=global bee department worker"`
    ScopeID string `json:"scope_id"`
    Key     string `json:"key"      binding:"required"`
    Value   string `json:"value"    binding:"required"`
}

type updateEnvRequest struct {
    Value string `json:"value" binding:"required"`
}
```

**Handler 方法：**

- `List(c *gin.Context)` — 读取 query 参数 `scope` 和 `scope_id`，调用 `store.List()`，返回含 `masked` 的结果
- `Create(c *gin.Context)` — 绑定请求，调用 `svc.Create()`，返回 201
- `Update(c *gin.Context)` — 绑定请求，调用 `svc.UpdateValue()`，返回 200
- `Delete(c *gin.Context)` — 调用 `store.Delete()`，返回 204

---

### Step 11 — 路由注册

**文件：** `internal/routes/api.go`

在 `registerAPIRoutes` 中追加：

```go
r.GET("/envs",     s.Envs.List)
r.POST("/envs",    s.Envs.Create)
r.PUT("/envs/:id", s.Envs.Update)
r.DELETE("/envs/:id", s.Envs.Delete)
```

**文件：** `internal/routes/server.go`（或依赖注入入口）

在 `ServerParams` 中添加 `Envs *api.EnvHandler`，并在构建时完成依赖连接：

```go
envStore  := store.NewEnvConfigStore(db)
envSvc    := env.NewService(envStore, workerStore, deptStore, cfg.Advanced.EnvSecret)
envHandler := api.NewEnvHandler(envSvc)
```

同时将 `envSvc` 注入到 `worker.Manager` 和 `bee.BeeProcess` 的构造函数中。

---

## 实现顺序建议

```
Step 1  → Step 2  → Step 3  → Step 4  → Step 5
  ↓
Step 6  → Step 7  → Step 8  → Step 9
  ↓
Step 10 → Step 11
```

每个 Step 完成后建议手动验证该层的基本功能，再进入下一层。

---

## 关键约束提醒

- `OPENBEE_API_KEY` 为系统保留 key，在 `Service.Create` 和 `Service.UpdateValue` 中拦截
- `cmd.Env` 中 `OPENBEE_API_KEY` 必须最后追加，防止被用户 env 覆盖
- API 响应永远只返回 `masked`，不返回 `enc_value`
- Bee 目前无数据库实体，`beeID` 暂用固定标识符（如 `"default"`），待 Bee 实体化后扩展
