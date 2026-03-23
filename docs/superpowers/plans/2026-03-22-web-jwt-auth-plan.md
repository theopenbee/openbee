# Web JWT 鉴权实施计划

**设计文档:** `docs/superpowers/specs/2026-03-22-web-jwt-auth-design.md`
**日期:** 2026-03-22

---

## Phase 1: 后端基础设施（独立可测试）

### Step 1.1: 引入依赖 + 配置层
**文件:**
- `go.mod` — 新增 `golang-jwt/jwt/v5`
- `internal/config/config.go` — `ServerConfig` 新增 `Auth AuthConfig`，`applyDefaults` 添加 TTL 默认值（access: 2h, refresh: 168h）

**验证:** `go build ./...` 通过

### Step 1.2: JWT 核心模块
**文件:**
- `internal/auth/jwt.go` — `JWTService` 实现：
  - `NewJWTService(secret, accessTTL, refreshTTL)`
  - `GenerateTokenPair()` → access_token + refresh_token
  - `GenerateAccessToken()` → 仅 access_token（refresh 时用）
  - `ValidateAccessToken(token)` → error
  - `ValidateRefreshToken(token)` → error
  - Claims 包含 `type` (access/refresh), `exp`, `iat`, `jti`(UUID)

**验证:** `internal/auth/jwt_test.go`
- 签发 token pair 并验证
- access token 过期后验证失败
- refresh token 不能当 access token 用
- 无效签名被拒绝

### Step 1.3: 登录速率限制
**文件:**
- `internal/auth/rate_limit.go` — `LoginRateLimiter`：
  - 基于 IP 的内存计数器
  - 每个 window（1min）最多 maxAttempts（5）次
  - 过期条目自动清理

**验证:** 单元测试确认限流行为

### Step 1.4: JWT 中间件
**文件:**
- `internal/auth/middleware.go` — `JWTMiddleware(jwtSvc)`：
  - 优先从 `Authorization: Bearer <token>` 取 token
  - 回退到 `token` query parameter
  - 验证失败返回 `401 {"error": "unauthorized"}`

**验证:** `internal/auth/middleware_test.go`
- 有效 token 放行
- 无 token 返回 401
- 过期 token 返回 401
- query param token 放行

### Step 1.5: 认证 Handler
**文件:**
- `internal/auth/handler.go` — `AuthHandler`：
  - `POST /login`: 验证密码（ConstantTimeCompare），检查速率限制，成功返回 token pair
  - `POST /refresh`: 验证 refresh_token，返回新 access_token（不含 refresh_token）
  - `GET /status`: 返回 `{"auth_required": bool}`

**验证:** `internal/auth/handler_test.go`
- 正确密码登录成功
- 错误密码返回 401
- 速率限制返回 429
- refresh 正常工作
- status 返回正确状态

---

## Phase 2: 后端集成

### Step 2.1: 路由集成
**文件:**
- `internal/api/router.go` — 改动：
  - `Server` 结构体新增 `authHandler *auth.AuthHandler`, `jwtMiddleware gin.HandlerFunc`
  - `NewServer` 签名增加两个参数
  - `setupRoutes()`:
    - `/api/auth/*` 路由注册（不受 JWT 保护）
    - `/api/*` group 挂载 `jwtMiddleware`（如非 nil）
    - SSE 路由 `/api/local/sessions/:id/stream` 显式挂载 `jwtMiddleware`
    - `/internal/log/level` 显式挂载 `jwtMiddleware`
  - CORS 配置：auth 启用时使用 `AllowOriginFunc` 替代 `AllowOrigins: *`

**验证:** `go build ./...` 通过

### Step 2.2: 应用层初始化
**文件:**
- `internal/app/app.go` — `buildAPIServer` 改动：
  - 如果 `cfg.Server.Auth.Password != ""`：创建 JWTService, RateLimiter, AuthHandler
  - `jwt_secret` 为空时自动生成并 log
  - 传入 NewServer

**验证:** `go build ./...` 通过，手动启动 server 测试

### Step 2.3: 配置模板更新
**文件:**
- `internal/config/config.yaml.tmpl` — server 段新增 auth 配置
- `cmd/openbee/config.go` — `configValues` 新增 `AuthPassword`, `AuthJWTSecret` 等字段，交互式流程增加 auth 配置问答

**验证:** `openbee config` 命令生成包含 auth 段的 yaml

---

## Phase 3: 前端适配

### Step 3.1: Auth 工具库
**文件:**
- `web/src/lib/auth.ts`:
  - `saveTokens/getAccessToken/getRefreshToken/clearTokens` — localStorage 操作
  - `refreshAccessToken` — 带 promise 去重的刷新逻辑
  - `checkAuthRequired` — 调用 `GET /api/auth/status`

**验证:** 手动测试 token 存取

### Step 3.2: API 层改造
**文件:**
- `web/src/lib/api.ts`:
  - `fetchAPI` 注入 Authorization header
  - 401 → 自动 refresh → 重试或跳转 login
  - `executions.logs` 和 `localChat.uploadMedia` 同样注入 token
  - SSE URL 追加 `?token=xxx`

**验证:** 浏览器 network 面板确认请求携带 Authorization header

### Step 3.3: 登录页面
**文件:**
- `web/src/pages/login.tsx`:
  - 居中卡片布局
  - 密码输入 + 提交按钮
  - 错误展示（401 密码错误, 429 频率限制）
  - 成功后 saveTokens + navigate to "/"

**验证:** 手动测试登录流程

### Step 3.4: 路由守卫 + 路由改造
**文件:**
- `web/src/components/auth-guard.tsx`:
  - 加载状态 → 检查 auth status → 判断 token → 渲染或跳转 login
- `web/src/App.tsx`:
  - 新增 `/login` 路由
  - 现有路由用 `<AuthGuard>` 包裹

**验证:** 端到端测试：未登录访问 → 跳转登录 → 登录成功 → 正常使用 → token 过期 → 自动刷新

---

## Phase 4: 测试与收尾

### Step 4.1: 集成测试
- 启动 server（auth enabled）→ 访问 API 返回 401
- 登录 → 获取 token → 正常访问 API
- access token 过期 → refresh 成功 → 继续访问
- refresh token 过期 → 需要重新登录
- 无 password 配置 → 所有 API 无需鉴权（向后兼容）

### Step 4.2: 收尾
- 确认所有现有测试通过
- CHANGELOG 更新
