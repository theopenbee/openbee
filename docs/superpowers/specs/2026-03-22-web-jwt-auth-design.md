# Web JWT 鉴权设计文档

**日期:** 2026-03-22
**状态:** 待用户审核

## 1. 问题描述

当前 OpenBee 的所有 Web API 接口 (`/api/*`) 和前端页面均无鉴权保护，任何人都可以直接访问和操作，存在以下安全隐患：

- Worker 可被任意创建、修改、删除
- 执行日志可被任意读取
- 本地聊天可被任意使用（可触发 AI 执行）
- `PUT /internal/log/level` 可被任意修改日志级别
- CORS 配置允许所有来源 (`*`)

**已有鉴权（不受本次改动影响）：**
- MCP 路由 (`/mcp/*`)：已有 API Key 中间件
- Telegram 平台：已有 auth code 机制

## 2. 设计方案：JWT Token 鉴权

### 2.1 整体架构

```
用户 ──POST /api/auth/login──▶ 后端验证密码（带速率限制）
                                  │
                              ◀── 返回 JWT access_token + refresh_token
                                  │
用户 ──携带 Authorization: Bearer <token>──▶ JWT 中间件验证
                                              │
                                          ◀── 通过 → 放行请求
                                          ◀── 失败 → 401 Unauthorized
```

### 2.2 认证流程

#### 登录
1. 用户访问前端，检测到无有效 token，跳转登录页
2. 用户输入用户名和密码，POST `/api/auth/login`
3. 后端验证用户名和密码（速率限制：每 IP 每分钟最多 5 次），签发 access_token（短期）和 refresh_token（长期）
4. 前端存储 tokens 到 localStorage

#### 请求鉴权
1. 前端每次请求在 Header 中携带 `Authorization: Bearer <access_token>`
2. JWT 中间件验证 token 有效性（签名、过期时间）
3. 验证通过则放行，失败返回 401

#### Token 刷新
1. access_token 过期后，前端用 refresh_token 调用 `POST /api/auth/refresh`
2. 后端验证 refresh_token 有效性，仅签发新的 access_token（refresh_token 不轮换，保持不变直到自身过期）
3. refresh_token 过期则需要重新登录

### 2.3 后端设计

#### 2.3.1 配置扩展

在 `config.go` 的 `ServerConfig` 中新增 `Auth AuthConfig`：

```go
type AuthConfig struct {
    Username        string        `yaml:"username"`         // 登录用户名，默认 "admin"
    Password        string        `yaml:"password"`         // 登录密码，空则跳过鉴权
    JWTSecret       string        `yaml:"jwt_secret"`       // JWT 签名密钥，为空时自动生成
    AccessTokenTTL  time.Duration `yaml:"access_token_ttl"` // access_token 有效期，默认 2h
    RefreshTokenTTL time.Duration `yaml:"refresh_token_ttl"` // refresh_token 有效期，默认 7d
}

type ServerConfig struct {
    Port  int        `yaml:"port"`
    Host  string     `yaml:"host"`
    Debug bool       `yaml:"debug"`
    Auth  AuthConfig `yaml:"auth"`
}
```

配置文件新增（在 `server` 段下）：
```yaml
server:
  port: 8080
  host: localhost
  debug: false
  auth:
    username: "admin"                # 登录用户名，默认 admin
    password: "your-password"        # 登录密码，留空则不启用鉴权
    jwt_secret: ""                   # JWT 密钥，留空则自动生成（重启后 token 失效）
    access_token_ttl: 2h
    refresh_token_ttl: 168h          # 7 天
```

**设计决策：**
- `password` 为空时，不启用鉴权（向后兼容，方便本地开发）
- `username` 默认为 `admin`，可自定义
- `jwt_secret` 为空时，启动时自动生成随机密钥（重启后所有 token 失效，安全但需重新登录）
- 单用户模型（账号+密码），无需用户表，凭证直接配置

#### 2.3.2 JWT 工具模块

新增 `internal/auth/jwt.go`，使用 `golang-jwt/jwt/v5` 库：

```go
package auth

import "github.com/golang-jwt/jwt/v5"

type TokenPair struct {
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
    ExpiresIn    int64  `json:"expires_in"` // access_token 剩余秒数
}

type Claims struct {
    Type string `json:"type"` // "access" 或 "refresh"
    jwt.RegisteredClaims
}

type JWTService struct {
    secret          []byte
    accessTokenTTL  time.Duration
    refreshTokenTTL time.Duration
}

func NewJWTService(secret string, accessTTL, refreshTTL time.Duration) *JWTService
func (s *JWTService) GenerateTokenPair() (*TokenPair, error)
func (s *JWTService) GenerateAccessToken() (string, int64, error)
func (s *JWTService) ValidateAccessToken(tokenStr string) error
func (s *JWTService) ValidateRefreshToken(tokenStr string) error
```

**选择 `golang-jwt/jwt/v5` 的理由：**
- JWT 有 base64url 编码、header 格式等细节，手动实现容易引入安全隐患
- 该库是 Go 生态最广泛使用的 JWT 库，经过充分审计
- 项目已有 20+ 依赖，增加一个轻量库不增加负担

#### 2.3.3 认证中间件

新增 `internal/auth/middleware.go`：

```go
// JWTMiddleware 验证 access token，支持两种传递方式：
// 1. Authorization: Bearer <token> header
// 2. token=<token> query parameter（用于 SSE 等不支持自定义 header 的场景）
func JWTMiddleware(jwtSvc *JWTService) gin.HandlerFunc {
    return func(c *gin.Context) {
        token := extractBearerToken(c)
        if token == "" {
            token = c.Query("token")
        }
        if err := jwtSvc.ValidateAccessToken(token); err != nil {
            c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
            return
        }
        c.Next()
    }
}
```

#### 2.3.4 登录速率限制

在 `internal/auth/rate_limit.go` 中实现简单的内存级速率限制：

```go
// LoginRateLimiter 限制每个 IP 每分钟最多 maxAttempts 次登录尝试
type LoginRateLimiter struct {
    maxAttempts int
    window      time.Duration
    attempts    map[string][]time.Time
    mu          sync.Mutex
}

func NewLoginRateLimiter(maxAttempts int, window time.Duration) *LoginRateLimiter
func (l *LoginRateLimiter) Allow(ip string) bool
```

#### 2.3.5 认证路由

新增 `internal/auth/handler.go`：

```go
type AuthHandler struct {
    username    string
    password    string
    jwtSvc      *JWTService
    rateLimiter *LoginRateLimiter
}

// POST /api/auth/login
// Body: {"username": "admin", "password": "xxx"}
// Response: {"access_token": "...", "refresh_token": "...", "expires_in": 7200}
// 用户名或密码错误返回 401，速率限制返回 429
func (h *AuthHandler) Login(c *gin.Context)

// POST /api/auth/refresh
// Body: {"refresh_token": "xxx"}
// Response: {"access_token": "...", "expires_in": 7200}
// 注意：不返回新的 refresh_token，避免无状态下的轮换复杂性
func (h *AuthHandler) Refresh(c *gin.Context)

// GET /api/auth/status
// 无需 token，返回是否需要登录
// Response: {"auth_required": true}
func (h *AuthHandler) Status(c *gin.Context)
```

#### 2.3.6 路由注册变更

`router.go` 改动（完整 setupRoutes）：

```go
func NewServer(
    ws *store.WorkerStore,
    es *store.ExecutionStore,
    mgr *worker.Manager,
    logRegistry *worker.ActiveLogRegistry,
    mcpSrv *mcp.MCPServer,
    mcpAPIKey string,
    staticFS fs.FS,
    localChat *LocalChatHandler,
    authHandler *auth.AuthHandler,  // 新增，nil 表示不启用鉴权
    jwtMiddleware gin.HandlerFunc,  // 新增，nil 表示不启用鉴权
) *Server

func (s *Server) setupRoutes() {
    // 认证路由 — 始终注册，不需要 JWT
    if s.authHandler != nil {
        authGroup := s.router.Group("/api/auth")
        {
            authGroup.GET("/status", s.authHandler.Status)
            authGroup.POST("/login", s.authHandler.Login)
            authGroup.POST("/refresh", s.authHandler.Refresh)
        }
    }

    // API 路由 — 需要 JWT（仅在启用鉴权时）
    api := s.router.Group("/api")
    if s.jwtMiddleware != nil {
        api.Use(s.jwtMiddleware)
    }
    {
        // Workers, Executions, Sessions, Local chat — 路由定义不变
        // ...
    }

    // SSE 流 — 注册在 gzip 外，但仍需 JWT 保护
    if s.localChatHandler != nil {
        sseHandler := s.localChatHandler.StreamReplies
        if s.jwtMiddleware != nil {
            // 手动应用 JWT 中间件（因为此路由不在 /api group 内）
            s.router.GET("/api/local/sessions/:id/stream",
                s.jwtMiddleware, sseHandler)
        } else {
            s.router.GET("/api/local/sessions/:id/stream", sseHandler)
        }
    }

    // Internal log level — 同样受 JWT 保护
    if s.jwtMiddleware != nil {
        s.router.PUT("/internal/log/level",
            s.jwtMiddleware, gin.WrapH(logger.LevelHandler()))
    } else {
        s.router.PUT("/internal/log/level", gin.WrapH(logger.LevelHandler()))
    }

    // MCP — 保持原有 API Key 鉴权
    // ...

    // 静态文件 — 不变
    // ...
}
```

**关键点：**
- SSE 端点 `/api/local/sessions/:id/stream` 虽然注册在 router 根级别（绕过 gzip），但显式挂载了 JWT 中间件
- `/internal/log/level` 也受 JWT 保护
- `authHandler` 和 `jwtMiddleware` 为 nil 时完全跳过鉴权，向后兼容

#### 2.3.7 应用层集成

`app.go` 中 `buildAPIServer` 更新：

```go
func buildAPIServer(cfg config.ServerConfig, mcpCfg config.MCPConfig, s appStores, ...) *api.Server {
    var authHandler *auth.AuthHandler
    var jwtMiddleware gin.HandlerFunc

    if cfg.Auth.Password != "" {
        username := cfg.Auth.Username
        if username == "" {
            username = "admin"
        }
        secret := cfg.Auth.JWTSecret
        if secret == "" {
            // 自动生成随机密钥
            b := make([]byte, 32)
            rand.Read(b)
            secret = hex.EncodeToString(b)
            logger.Info("JWT secret auto-generated (tokens will expire on restart)")
        }
        jwtSvc := auth.NewJWTService(secret, cfg.Auth.AccessTokenTTL, cfg.Auth.RefreshTokenTTL)
        rateLimiter := auth.NewLoginRateLimiter(5, time.Minute)
        authHandler = auth.NewAuthHandler(username, cfg.Auth.Password, jwtSvc, rateLimiter)
        jwtMiddleware = auth.JWTMiddleware(jwtSvc)
    }

    return api.NewServer(..., authHandler, jwtMiddleware)
}
```

#### 2.3.8 CORS 配置调整

当启用鉴权时（`password != ""`），CORS 配置变更：

```go
corsConfig := cors.Config{
    AllowMethods:  []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
    AllowHeaders:  []string{"Origin", "Content-Type", "Authorization", "Accept-Language", "X-API-Key"},
    ExposeHeaders: []string{"Content-Length"},
}

if authEnabled {
    // AllowOrigins: * 与 AllowCredentials: true 互斥
    // 使用 AllowOriginFunc 允许所有来源但兼容 credentials
    corsConfig.AllowOriginFunc = func(origin string) bool { return true }
    corsConfig.AllowCredentials = true
} else {
    corsConfig.AllowOrigins = []string{"*"}
    corsConfig.AllowCredentials = false
}
```

### 2.4 前端设计

#### 2.4.1 新增文件

| 文件 | 用途 |
|------|------|
| `web/src/lib/auth.ts` | Token 存储/读取/刷新逻辑（含并发刷新去重） |
| `web/src/pages/login.tsx` | 登录页面 |
| `web/src/components/auth-guard.tsx` | 路由守卫组件 |

#### 2.4.2 Token 管理 (`auth.ts`)

```typescript
// Token 存储
function saveTokens(accessToken: string, refreshToken: string): void
function getAccessToken(): string | null
function getRefreshToken(): string | null
function clearTokens(): void

// Token 刷新（带并发去重，多个 401 只触发一次 refresh）
let refreshPromise: Promise<string | null> | null = null
async function refreshAccessToken(): Promise<string | null> {
    if (refreshPromise) return refreshPromise
    refreshPromise = doRefresh().finally(() => { refreshPromise = null })
    return refreshPromise
}

// 认证状态检查
async function checkAuthRequired(): Promise<boolean>
```

#### 2.4.3 API 层改造 (`api.ts`)

修改 `fetchAPI` 函数：
1. 自动在请求头中注入 `Authorization: Bearer <token>`
2. 收到 401 响应时，尝试用 refresh_token 刷新
3. 刷新成功后自动重试原请求
4. 刷新失败则跳转登录页

```typescript
async function fetchAPI<T>(path: string, options?: RequestInit): Promise<T> {
    const token = getAccessToken()
    const headers: Record<string, string> = {
        "Content-Type": "application/json",
        "Accept-Language": i18n.language || "en",
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...(options?.headers as Record<string, string> ?? {}),
    }

    let res = await fetch(`${API_BASE}${path}`, { ...options, headers })

    if (res.status === 401 && getRefreshToken()) {
        const newToken = await refreshAccessToken()
        if (newToken) {
            headers.Authorization = `Bearer ${newToken}`
            res = await fetch(`${API_BASE}${path}`, { ...options, headers })
        } else {
            clearTokens()
            window.location.hash = "#/login"
            throw new Error("unauthorized")
        }
    }
    // ... 原有错误处理
}
```

同理改造 `executions.logs`（直接 fetch）和 `localChat.uploadMedia`（FormData 请求）中的 token 注入。

SSE 连接使用 query parameter：
```typescript
const url = `${API_BASE}/local/sessions/${id}/stream?token=${getAccessToken()}`
const eventSource = new EventSource(url)
```

#### 2.4.4 路由守卫 (`auth-guard.tsx`)

```tsx
function AuthGuard({ children }: { children: ReactNode }) {
    const [state, setState] = useState<"loading" | "authed" | "login">("loading")

    useEffect(() => {
        checkAuthRequired().then(required => {
            if (!required) { setState("authed"); return }
            if (getAccessToken()) { setState("authed"); return }
            setState("login")
        })
    }, [])

    if (state === "loading") return null
    if (state === "login") { /* redirect to /login */ }
    return children
}
```

#### 2.4.5 登录页面 (`login.tsx`)

- 简洁的用户名+密码输入表单（居中卡片样式）
- 调用 `POST /api/auth/login` 提交 username + password
- 成功后存储 tokens 并跳转到首页
- 失败显示错误信息（401 用户名或密码错误，429 尝试过于频繁）

#### 2.4.6 App.tsx 路由改造

```tsx
<Routes>
    <Route path="/login" element={<Login />} />
    <Route element={<AuthGuard><Layout /></AuthGuard>}>
        {/* ... 现有路由不变 */}
    </Route>
</Routes>
```

### 2.5 安全考虑

1. **密码验证** — 使用 `crypto/subtle.ConstantTimeCompare` 防止时序攻击
2. **JWT 签名** — 使用 `golang-jwt/jwt/v5` 库实现 HMAC-SHA256 (HS256)，避免手动实现的安全隐患
3. **Token 过期** — access_token 短期（2h），refresh_token 长期（7d），不做 refresh_token 轮换
4. **登录速率限制** — 每 IP 每分钟最多 5 次，防止暴力破解
5. **向后兼容** — password 为空时不启用鉴权，现有部署无感知
6. **SSE token** — 通过 query parameter 传递，已知风险：token 会出现在服务器访问日志中。缓解：access_token 有效期短（2h），且 Gin 日志在生产模式下可关闭
7. **前端 token 存储** — 使用 localStorage，已知 XSS 风险。可接受的权衡：openbee 为单用户工具，前端无用户输入的 HTML 渲染场景。如未来需要更高安全性，可将 refresh_token 改为 httpOnly cookie
8. **并发刷新去重** — 前端使用 promise 去重，避免多个 401 触发多次 refresh 请求
9. **所有端点保护** — `/api/*`、`/internal/log/level`、SSE 流均受 JWT 保护

### 2.6 不在本次范围内

- 多用户支持（仅单账号模式）
- RBAC 权限控制
- OAuth / 第三方登录
- 密码修改 API（直接改配置文件）
- HTTPS（应由反向代理处理）
- Token 黑名单/注销（无状态 JWT，登出仅清除前端 token）

## 3. 涉及文件清单

### 后端新增
| 文件 | 说明 |
|------|------|
| `internal/auth/jwt.go` | JWT 签发与验证（基于 golang-jwt/jwt/v5） |
| `internal/auth/middleware.go` | Gin JWT 中间件（支持 header 和 query param） |
| `internal/auth/handler.go` | 登录/刷新/状态 API handler |
| `internal/auth/rate_limit.go` | 登录速率限制器 |
| `internal/auth/jwt_test.go` | JWT 签发/验证单元测试 |
| `internal/auth/middleware_test.go` | 中间件行为测试 |
| `internal/auth/handler_test.go` | 认证接口测试 |

### 后端修改
| 文件 | 改动 |
|------|------|
| `go.mod` / `go.sum` | 新增 `github.com/golang-jwt/jwt/v5` 依赖 |
| `internal/config/config.go` | `ServerConfig` 新增 `Auth AuthConfig`，`applyDefaults` 新增默认值 |
| `internal/config/config.yaml.tmpl` | `server` 段下新增 `auth` 配置 |
| `internal/api/router.go` | `NewServer` 签名变更（增加 authHandler, jwtMiddleware），`setupRoutes` 挂载认证路由和中间件，SSE/internal 路由增加 JWT 保护 |
| `internal/app/app.go` | `buildAPIServer` 初始化 JWTService/AuthHandler 并传入 Server |
| `cmd/openbee/config.go` | `configValues` 新增 auth 字段，交互式配置新增 auth 问答 |

### 前端新增
| 文件 | 说明 |
|------|------|
| `web/src/lib/auth.ts` | Token 管理工具（含并发刷新去重） |
| `web/src/pages/login.tsx` | 登录页面 |
| `web/src/components/auth-guard.tsx` | 路由守卫 |

### 前端修改
| 文件 | 改动 |
|------|------|
| `web/src/lib/api.ts` | fetchAPI 注入 token、401 自动刷新重试，SSE/upload 也注入 token |
| `web/src/App.tsx` | 添加 /login 路由和 AuthGuard 包裹 |

## 4. 实施步骤概要

1. **后端：依赖引入** — `go get github.com/golang-jwt/jwt/v5`
2. **后端：配置层** — ServerConfig 新增 AuthConfig，applyDefaults 设置默认 TTL
3. **后端：JWT 核心** — 实现 JWTService（签发/验证 access_token 和 refresh_token）
4. **后端：速率限制** — 实现 LoginRateLimiter
5. **后端：中间件** — 实现 JWTMiddleware（支持 header + query param）
6. **后端：认证 Handler** — 实现 login/refresh/status 接口
7. **后端：路由集成** — 修改 NewServer 签名，setupRoutes 挂载中间件
8. **后端：应用层集成** — app.go 初始化 auth 组件
9. **后端：配置模板** — 更新 config.yaml.tmpl 和 config.go 交互式配置
10. **后端：单元测试** — jwt_test.go, middleware_test.go, handler_test.go
11. **前端：auth 工具** — Token 存储/刷新逻辑
12. **前端：登录页面** — 密码输入表单
13. **前端：路由守卫** — AuthGuard 组件
14. **前端：API 层改造** — fetchAPI 注入 token，SSE/upload 适配
15. **前端：路由改造** — App.tsx 集成
16. **集成测试与验证** — 端到端测试鉴权流程
