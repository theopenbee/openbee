# Env 配置功能设计文档

**日期：** 2026-04-12  
**状态：** 待实现

---

## 背景

openbee2 目前所有配置集中在 `config.yaml`（全局级别），缺乏针对 Bee、部门、Worker 的精细化 env 配置能力。本功能引入三层 env 配置体系，支持对子进程执行环境的灵活注入。

---

## 层级与优先级模型

系统存在两条独立的优先级链，互不干涉：

### Worker 链（Worker 执行时生效）

```
Worker（bee_workers）> 部门（department）> 全局（global）
```

- 全局 env 作为所有 Worker 执行的基础默认值
- 部门 env 覆盖全局中的同名 key
- Worker env 覆盖部门（及全局）中的同名 key
- 若 Worker 属于多个部门且存在同名 key，取**部门 ID 字典序最小**的部门优先

### Bee 链（Bee 执行时生效）

```
Bee > 全局（global）
```

- 全局 env 作为所有 Bee 执行的基础默认值
- Bee env 覆盖全局中的同名 key

---

## 数据模型

使用单表存储所有层级的 env 配置：

```sql
CREATE TABLE bee_env_configs (
    id          TEXT PRIMARY KEY,
    scope       TEXT NOT NULL,
    -- 可选值：'global' | 'bee' | 'department' | 'worker'
    scope_id    TEXT,
    -- 对应实体 ID（bee_id / department_id / worker_id）
    -- scope 为 'global' 时为 NULL
    key         TEXT NOT NULL,
    enc_value   TEXT NOT NULL,
    -- AES-256-GCM 加密后的 base64 编码值
    masked      TEXT NOT NULL,
    -- 打码展示值，如 sk-****abcd，用于 API 返回和 UI 展示
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL,
    UNIQUE(scope, scope_id, key)
);
```

---

## 加密方案

- **算法：** AES-256-GCM
- **密钥来源：** `config.yaml` advanced 区域新增 `env_secret` 字段，首次启动时自动生成（与现有 `jwt_secret`、`mcp_token_secret` 机制一致，调用 `config.GenerateRandomSecret()`）
- **写入时：** 使用 `env_secret` 加密明文值，结果以 base64 存入 `enc_value`；同时计算打码值存入 `masked`（规则：保留前 4 位和后 4 位，中间替换为 `****`，总长度不足 8 位则全部打码）
- **读取时：** API 响应只返回 `masked`，不暴露 `enc_value`
- **注入时：** 在进程启动前解密 `enc_value`，得到明文传入子进程环境

---

## 运行时解析逻辑

### Worker 执行

```go
func ResolveWorkerEnv(workerID string) ([]string, error) {
    // 1. 获取全局 env
    globalEnv := store.ListEnv("global", "")
    // 2. 获取 worker 所属部门（按部门 ID 字典序排序，取第一个有值的部门）
    deptEnv := store.ListEnvForWorkerDepts(workerID)
    // 3. 获取 worker 自身 env
    workerEnv := store.ListEnv("worker", workerID)
    // 4. 合并（后者覆盖前者同名 key）
    resolved := merge(globalEnv, deptEnv, workerEnv)
    // 5. 解密并格式化为 "KEY=VALUE" 切片
    return decrypt(resolved)
}
```

### Bee 执行

```go
func ResolveBeeEnv(beeID string) ([]string, error) {
    globalEnv := store.ListEnv("global", "")
    beeEnv    := store.ListEnv("bee", beeID)
    resolved  := merge(globalEnv, beeEnv)
    return decrypt(resolved)
}
```

---

## 注入方式

在现有进程启动处，将解密后的 env 追加到 `cmd.Env`：

```go
// Worker 执行（internal/domain/worker/manager.go launchRuntime）
resolved, err := envService.ResolveWorkerEnv(workerID)
cmd.Env = append(baseEnv, resolved...)
cmd.Env = append(cmd.Env, "OPENBEE_API_KEY="+opts.APIKey)
// OPENBEE_API_KEY 最后追加，防止被用户 env 意外覆盖

// Bee 执行（internal/domain/bee/bee_process.go）
resolved, err := envService.ResolveBeeEnv(beeID)
cmd.Env = append(baseEnv, resolved...)
cmd.Env = append(cmd.Env, "OPENBEE_API_KEY="+opts.APIKey)
```

---

## API 端点

所有端点返回打码值（`masked`），不返回加密原文或明文。

| 方法   | 路径                                           | 说明               |
|--------|------------------------------------------------|--------------------|
| GET    | `/api/envs?scope=global`                       | 列出全局 env       |
| GET    | `/api/envs?scope=bee&scope_id={bee_id}`        | 列出 Bee env       |
| GET    | `/api/envs?scope=department&scope_id={dept_id}`| 列出部门 env       |
| GET    | `/api/envs?scope=worker&scope_id={worker_id}`  | 列出 Worker env    |
| POST   | `/api/envs`                                    | 新增 env 配置      |
| PUT    | `/api/envs/:id`                                | 更新 env 值        |
| DELETE | `/api/envs/:id`                                | 删除 env 配置      |

### POST /api/envs 请求体

```json
{
  "scope": "worker",
  "scope_id": "worker-uuid",
  "key": "OPENAI_API_KEY",
  "value": "sk-xxxxxx"
}
```

### GET 响应示例

```json
[
  {
    "id": "env-uuid",
    "scope": "worker",
    "scope_id": "worker-uuid",
    "key": "OPENAI_API_KEY",
    "masked": "sk-xx****xxxx",
    "created_at": "2026-04-12T10:00:00Z",
    "updated_at": "2026-04-12T10:00:00Z"
  }
]
```

---

## 代码结构规划

```
internal/
  infra/
    model/
      env_config.go          # EnvConfig 数据结构
    store/
      env_config_store.go    # CRUD + 多部门查询
  domain/
    env/
      service.go             # ResolveWorkerEnv / ResolveBeeEnv / 加密解密
  api/
    env_handler.go           # HTTP 处理器
  infra/
    config/
      config.go              # 新增 env_secret 字段
```

---

## 约束与边界

- `OPENBEE_API_KEY` 为系统保留 key，用户 env 中禁止使用此 key（API 层拦截）
- Worker 属于多个部门时，同名 key 取部门 ID 字典序最小的部门值
- Bee 目前无独立数据库实体，Bee env 的 `scope_id` 暂用配置中的 bee 标识（待 Bee 实体化后对齐）
- 前端 UI 暂不实现，后续单独迭代
