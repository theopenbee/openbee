# 多用户 + RBAC + 超管初始化 — 设计文档

- 日期：2026-06-26
- 状态：已评审（待实现）
- 范围：Web 控制台的人类用户鉴权体系，从「单一配置账号」升级为「DB 多用户 + 可配置角色权限（RBAC）」，并支持系统首次启动通过设置向导初始化超级管理员。

## 1. 背景与现状

当前 Web 控制台的鉴权实现：

1. **单账号**：仅有一个管理员，用户名/密码来自 `config.yaml`（`server.auth.username` 默认 `admin`，`password` 为空时启动自动生成），**没有用户表**。
2. **JWT 登录**：`internal/infra/auth` 提供 access + refresh 双 token，`JWTMiddleware` 只校验 token 签名，**token 内不含用户身份/角色**。
3. **已有的 `permission_scopes` 与本需求无关**：它是给 worker/bee 的 **RPC token** 限制可调用工具用的（见 `internal/infra/auth/scopes.go`、`internal/infra/model/worker.go`），不是人类用户的权限。本设计**不改动**这套机制。
4. **DB**：SQLite，版本化迁移框架（`internal/infra/store/db.go`，当前到 v46）。
5. **前端**：React + Vite，登录页 `web/src/pages/login.tsx`，鉴权封装 `web/src/lib/auth.ts`、`web/src/components/auth-guard.tsx`，业务页在 `web/src/pages/`。

## 2. 目标

- 引入 DB 持久化的多用户体系，支持创建/禁用/重置用户。
- 引入可配置的 RBAC：角色可自定义，权限按「资源:动作」勾选；业务资源（worker/task/department 等）的读写也按角色分权。
- 系统首次启动（库内无任何用户）时，通过 **Web 设置向导** 创建第一个超级管理员。
- 后端为权限权威；前端仅按权限隐藏 UI。

## 3. 关键决策（已评审确认）

| 决策点 | 结论 |
|---|---|
| 权限模型粒度 | 完整 RBAC：可配置角色 + 可勾选权限点 |
| 权限边界 | 业务资源的查看/操作也按角色分权（非仅登录/用户/系统配置） |
| 超管初始化方式 | Web 首次设置向导创建超管 |
| 普通用户来源 | 仅由超管/管理员后台创建（无开放注册） |
| 用户↔角色关系 | **一人多角色**（关联表 `bee_user_roles`），有效权限取所有角色权限的并集 |
| 存量升级策略 | **一律走设置向导**，不从 `config.yaml` 种超管 |

## 4. 数据模型（新增迁移，v47 起）

### 4.1 表结构

**`bee_users`**

| 列 | 类型 | 说明 |
|---|---|---|
| id | TEXT PK | uuid |
| username | TEXT UNIQUE NOT NULL | 登录名 |
| password_hash | TEXT NOT NULL | bcrypt（依赖 `golang.org/x/crypto/bcrypt`，已在 go.mod） |
| display_name | TEXT NOT NULL DEFAULT '' | 展示名 |
| status | TEXT NOT NULL DEFAULT 'active' | `active` / `disabled` |
| created_by | TEXT NOT NULL DEFAULT '' | 创建者 user id（向导建的超管为空） |
| created_at | INTEGER NOT NULL | |
| updated_at | INTEGER NOT NULL | |

**`bee_roles`**

| 列 | 类型 | 说明 |
|---|---|---|
| id | TEXT PK | uuid |
| name | TEXT UNIQUE NOT NULL | 角色名 |
| description | TEXT NOT NULL DEFAULT '' | |
| is_system | INTEGER NOT NULL DEFAULT 0 | 内置角色标记（super-admin 锁定，不可删/降权） |
| created_at | INTEGER NOT NULL | |
| updated_at | INTEGER NOT NULL | |

**`bee_role_permissions`**

| 列 | 类型 | 说明 |
|---|---|---|
| role_id | TEXT NOT NULL REFERENCES bee_roles(id) ON DELETE CASCADE | |
| permission | TEXT NOT NULL | 权限点 key | 
| PRIMARY KEY (role_id, permission) | | |

**`bee_user_roles`**（用户↔角色多对多）

| 列 | 类型 | 说明 |
|---|---|---|
| user_id | TEXT NOT NULL REFERENCES bee_users(id) ON DELETE CASCADE | |
| role_id | TEXT NOT NULL REFERENCES bee_roles(id) ON DELETE CASCADE | |
| created_at | INTEGER NOT NULL | |
| PRIMARY KEY (user_id, role_id) | | |

> 用户有效权限 = 其所有角色权限点的并集；任一角色含 `*` 即视为超管。

### 4.2 内置角色（迁移时种子）

- **super-admin**：权限通配 `*`，拥有全部权限；`is_system=1`，**不可删除、不可降权**（保证系统永远有人能管理用户与角色）。仅由设置向导创建第一个该角色用户；其权限集合在代码层硬保证。
- **admin**：业务资源全读写 + `users:manage`；默认不含 `roles:manage`、`system_config:write`。`is_system=0`，可编辑/删除。
- **member**：业务资源只读。`is_system=0`，可编辑/删除。

> 注：内置角色行在 v47 迁移中插入。super-admin 角色行始终存在；admin/member 为可调整的默认模板。

## 5. 权限点目录

权限点常量定义在 Go 中（单一事实源，供后端校验与前端展示），格式为 `资源:动作`：

```
workers:read       workers:write
tasks:read         tasks:write
departments:read   departments:write
messages:read
sessions:read      sessions:write
stats:read
env:read           env:write
system_config:read system_config:write
users:manage
roles:manage
```

- `*` 为超管专用通配，匹配任意权限点。
- 新增模块时在该目录追加权限点；前端权限管理页从后端 `GET /api/permissions`（返回目录 + 分组）动态渲染勾选项，避免前后端常量漂移。

## 6. 鉴权改造（`internal/infra/auth`）

### 6.1 Token 与权限解析

- JWT access token payload 增加 `uid`（用户 id）。
- **权限不写入 token**：每次请求由服务端按 `user → roles → permissions（并集）` 解析，避免改角色后旧 token 权限滞留。解析结果加进程内缓存（按 user_id 失效；用户的角色绑定、角色权限变更时清除对应缓存）。
- refresh token 流程不变。

### 6.2 中间件

- `JWTMiddleware`：校验签名后，按 `uid` 加载用户，写入 `gin.Context`；用户 `status=disabled` 即返回 401/403。
- 新增 `RequirePermission("xxx:write")`：在受保护路由上声明所需权限点，超管 `*` 直通；无权返回 403。每个现有 API 路由按其语义挂上对应权限点（读接口挂 `:read`，写接口挂 `:write`）。
- 后端为权威校验点，前端隐藏仅为体验优化。

### 6.3 登录与账号接口

- `POST /api/auth/login`：改为校验 `bee_users`（bcrypt 比对），替换现有 config 单账号校验。保留登录限流（`LoginRateLimiter`）。
- `GET /api/me`：返回当前用户信息 + 解析后的权限列表，供前端驱动 UI。
- `POST /api/me/password`：当前用户改密（校验旧密码）。
- 用户管理接口（受 `users:manage`）：列表 / 创建 / 禁用启用 / 重置密码 / 分配角色（多选）。
- 角色管理接口（受 `roles:manage`）：列表 / 创建 / 编辑（含勾选权限点）/ 删除（`is_system` 角色禁止删除，super-admin 禁止降权）。
- `GET /api/permissions`：返回权限点目录与分组。

### 6.4 config 鉴权的处置

- `server.auth.username/password` 不再用于 Web 登录（**Web 登录一律走 DB 用户**）。
- 保留 `jwt_secret` / `access_token_ttl` / `refresh_token_ttl` 等 token 相关配置。
- `username/password` 字段标注为废弃（保留解析以兼容旧配置文件，不报错），文档与 CHANGELOG（英文）说明此变更。

## 7. 超管初始化 — 设置向导

- `GET /api/setup/status` → `{ "initialized": <bool> }`，库内存在任意用户即 `true`。该接口免鉴权。
- 未初始化时，前端将所有访问重定向到 `/setup` 向导页。
- `POST /api/setup`：创建第一个 super-admin（username + password + 可选 display_name）。
  - **一次性守卫**：仅当库内零用户时可用；已初始化后调用返回 409，杜绝重复初始化。
  - 创建成功后返回登录态（或要求随即登录）。
- **存量部署**：升级后 `bee_users` 为空 → 首次访问即进入设置向导创建超管；旧的 config 账号不再生效（一律走向导）。

## 8. 前端改造（`web/src`）

- 新增页面：
  - 设置向导页（`/setup`）：未初始化时强制进入。
  - 用户管理页：列表/建/禁用/重置/分配角色，入口受 `users:manage` 控制。
  - 角色管理页：建/编辑角色并勾选权限点（数据源 `GET /api/permissions`），入口受 `roles:manage` 控制。
- 鉴权封装（`web/src/lib/auth.ts` / `auth-guard.tsx`）：
  - 应用启动先查 `GET /api/setup/status`，未初始化跳向导。
  - 登录后拉 `GET /api/me`，缓存当前用户与权限；提供 `hasPermission(key)` 供页面/组件按权限隐藏入口与操作。
- 现有业务页：按权限隐藏导航项与写操作按钮（后端仍强校验）。
- 圆角遵守项目设计规范：最大 `sm`，仅允许 `rounded-none` / `rounded-sm` / `rounded-full`（真圆形/胶囊）。

## 9. 模块边界（设计为可独立测试的单元）

- `infra/store`：`user_store.go`（含用户↔角色绑定读写）、`role_store.go`（CRUD + 权限读写），纯 DB 层。
- `infra/auth`：权限目录与解析 `permissions.go`（`user → roles → permission set 并集`，含缓存）、`RequirePermission` 中间件、bcrypt 封装。
- `api`：`auth_handler`（登录/me/改密）、`user_handler`、`role_handler`、`setup_handler`。
- 各单元通过明确接口交互：handler 依赖 store 接口与 auth 解析接口，便于以 fake 替身做单测。

## 10. 不做（YAGNI，列为后续可选）

- SSO / OAuth、MFA。
- 操作审计日志。
- 按实例的细粒度 ACL（如「某人只能看某些 worker/部门」的部门级数据隔离）——本期为「角色 × 资源类型」级别分权。
- `ctl` CLI 的人类身份鉴权：仍走本地 RPC token，不变。

## 11. 测试计划

- store：用户/角色 CRUD、唯一约束、`is_system` 删除保护、级联删除权限、用户↔角色绑定与解绑、删用户/角色时 `bee_user_roles` 级联清理。
- auth：`user → roles → permissions（并集）` 解析与缓存失效、bcrypt 登录、超管 `*` 直通、`RequirePermission` 拦截（有权/无权/禁用用户）。
- setup：一次性守卫（零用户可建、已初始化返回 409）。
- 迁移：v47 建表与内置角色种子幂等。
- handler：登录、me、用户管理、角色管理的权限边界。

## 12. 迁移与发布注意

- 升级即出现设置向导，需在发布说明（CHANGELOG，英文）中明确：Web 登录改为多用户体系，旧 config 账号失效，首次访问请通过向导创建超管。
- super-admin 不可被删除或降权的约束需在 store/handler 双层保证。
