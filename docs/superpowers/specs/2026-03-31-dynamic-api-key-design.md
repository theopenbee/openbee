# Dynamic API Key Design

**Date:** 2026-03-31
**Branch:** feat/ctl-cli
**Status:** Approved

## Background

Currently, Bee and Worker both authenticate to the openbee MCP server using static keys set in the config file:

- `bee.mcp.api_key` — used by Bee to access `/mcp/bee` routes
- `bee.mcp.worker_api_key` — shared by all Workers to access `/mcp/worker` routes

This setup has two problems:

1. The server cannot identify whether a request comes from Bee or a Worker.
2. All Workers share a single key, so the server cannot identify which specific Worker is making a request.

## Goal

Replace static keys with dynamically generated JWT tokens that encode identity. Each token must allow the server to determine:

1. Whether the caller is Bee or a Worker.
2. If it is a Worker, which Worker (by Worker ID).

## Design

### Core Approach: JWT Tokens (HS256) with Configurable TTL

Tokens are signed JWTs using HMAC-SHA256. A dedicated `token_secret` in the config is used for signing — separate from `server.auth.jwt_secret` to allow independent rotation. Tokens are generated at process spawn time and injected as the `OPENBEE_API_KEY` environment variable.

**Token lifecycle ("use and replace"):** Each Bee or Worker process start generates a fresh token. The default TTL is 2 hours (configurable). There is no refresh mechanism; if a task runs beyond the TTL, subsequent `openbee ctl` calls will receive 401. This is acceptable for the initial implementation; longer-running tasks can increase the TTL via config.

---

### 1. Config Changes (`internal/config/config.go`)

**Remove** `APIKey` and `WorkerAPIKey` from `MCPConfig`.
**Add** `TokenSecret` and `TokenTTL`:

```go
type MCPConfig struct {
    TokenSecret string        `yaml:"token_secret"` // HMAC-SHA256 secret; empty = auto-generated on startup
    TokenTTL    time.Duration `yaml:"token_ttl"`    // token validity period; default 2h
}
```

`applyDefaults` behavior:

- If `TokenSecret == ""`: call `generateRandomKey()` to auto-generate (same pattern as the existing `WorkerAPIKey` auto-generation). The generated value is used in memory for the current process lifetime; it is not written back to the config file. If a stable secret across restarts is needed, the user should run `openbee config` or set it manually.
- If `TokenTTL == 0`: set to `2 * time.Hour`.

**Validation in `New()`** (`app.go`):

- Remove the check for `APIKey` and `WorkerAPIKey`.
- Add a check that `TokenSecret != ""` (will always be set after `applyDefaults`).

**Config template** (`internal/config/config.yaml.tmpl`): replace `api_key` and `worker_api_key` lines with `token_secret` and `token_ttl`.

---

### 2. New `internal/mcp/token.go`

JWT token generation and validation for MCP authentication. Follows the same style as `internal/auth/jwt.go`.

```go
package mcp

import (
    "fmt"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

const (
    TokenTypeBee    = "bee"
    TokenTypeWorker = "worker"
)

// MCPClaims are the JWT claims embedded in every MCP token.
type MCPClaims struct {
    Type     string `json:"type"`               // "bee" or "worker"
    WorkerID string `json:"worker_id,omitempty"` // set only for worker tokens
    jwt.RegisteredClaims
}

// GenerateBeeToken creates a signed JWT for the Bee process.
func GenerateBeeToken(secret string, ttl time.Duration) (string, error)

// GenerateWorkerToken creates a signed JWT for a specific Worker.
func GenerateWorkerToken(secret, workerID string, ttl time.Duration) (string, error)

// ValidateToken parses and validates a JWT, returning its claims.
func ValidateToken(tokenStr, secret string) (*MCPClaims, error)
```

All three functions use `jwt.SigningMethodHS256`. `ValidateToken` rejects tokens with unexpected signing methods and expired tokens.

---

### 3. Updated `internal/mcp/auth.go`

Replace `APIKeyMiddleware` and `AnyKeyMiddleware` with JWT-aware middleware. The token extraction location (header `X-API-Key` or query param `api_key`) is unchanged so no client-side changes are needed.

```go
// Context key constants
const CtxKeyTokenType = "mcp.token.type"
const CtxKeyWorkerID  = "mcp.token.worker_id"

// JWTAuthMiddleware validates the token and writes claims to gin.Context.
// Returns 401 on missing, invalid, or expired token.
func JWTAuthMiddleware(secret string) gin.HandlerFunc

// RequireBee aborts with 403 if the token type is not "bee".
func RequireBee() gin.HandlerFunc

// RequireWorker aborts with 403 if the token type is not "worker".
func RequireWorker() gin.HandlerFunc

// RequireBeeOrWorker accepts tokens of either type.
func RequireBeeOrWorker() gin.HandlerFunc
```

`JWTAuthMiddleware` must be applied before any `Require*` middleware.

---

### 4. Updated `internal/api/router.go`

**`ServerParams` changes:**

```go
// Remove:
BeeAPIKey    string
WorkerAPIKey string

// Add:
TokenSecret string
```

**`registerMCPRoutes` changes:**

```go
beeGroup := s.router.Group(config.MCPBeeBasePath)
beeGroup.Use(mcp.JWTAuthMiddleware(s.TokenSecret), mcp.RequireBee())
// sse and messages handlers unchanged

s.router.POST(config.MCPBeeBasePath+"/call",
    mcp.JWTAuthMiddleware(s.TokenSecret),
    mcp.RequireBeeOrWorker(),
    s.BeeMCPServer.HandleCall,
)

workerGroup := s.router.Group(config.MCPWorkerBasePath)
workerGroup.Use(mcp.JWTAuthMiddleware(s.TokenSecret), mcp.RequireWorker())
// sse and messages handlers unchanged
```

---

### 5. Updated `internal/claude/invoker.go`

Remove the static `apiKey` field. Accept a per-invocation key in `Run()`.

```go
// Before
func NewInvoker(binary, openbeeURL, apiKey string) *Invoker
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath string) (*Process, <-chan Output, error)

// After
func NewInvoker(binary, openbeeURL string) *Invoker
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath, apiKey string) (*Process, <-chan Output, error)
```

`OPENBEE_API_KEY` is still injected into the subprocess environment; only the value changes from a static string to a freshly generated JWT.

---

### 6. Updated `internal/worker/manager.go`

Generate a Worker JWT immediately before spawning each Claude process:

```go
token, err := mcp.GenerateWorkerToken(
    mgr.beeCfg.MCP.TokenSecret,
    worker.ID,
    mgr.beeCfg.MCP.TokenTTL,
)
if err != nil {
    return fmt.Errorf("generate worker token: %w", err)
}
proc, ch, err := mgr.invoker.Run(ctx, workDir, prompt, opts, logPath, token)
```

`NewManager` no longer receives an `apiKey` argument (and `NewInvoker` no longer takes one either).

---

### 7. Updated `internal/bee/bee_process.go`

Generate a Bee JWT immediately before spawning the Bee process:

```go
token, err := mcp.GenerateBeeToken(cfg.MCP.TokenSecret, cfg.MCP.TokenTTL)
if err != nil {
    return fmt.Errorf("generate bee token: %w", err)
}
proc, ch, err := inv.Run(ctx, workDir, prompt, opts, logPath, token)
```

`NewInvoker` is called without an `apiKey` argument.

---

### 8. Updated `cmd/openbee/config.go`

**`configValues` struct:** remove `MCPAPIKey` and `WorkerAPIKey`; add `MCPTokenSecret string`.

**`loadExistingConfig`:** map `cfg.Bee.MCP.TokenSecret` → `MCPTokenSecret` (remove old key mappings).

**Interactive wizard:** remove the two survey steps for MCP API key and Worker API key. Add a single step for `token_secret` that mirrors the `jwt_secret` pattern: if empty, auto-generate and display; allow manual entry.

---

## Data Flow Summary

```
Server startup
  └─ applyDefaults: generate TokenSecret if empty, set TokenTTL default

Task dispatch (Worker)
  └─ manager.go: GenerateWorkerToken(secret, workerID, ttl)
       └─ inject as OPENBEE_API_KEY into claude subprocess

Bee spawn
  └─ bee_process.go: GenerateBeeToken(secret, ttl)
       └─ inject as OPENBEE_API_KEY into claude subprocess

Request arrives at /mcp/bee or /mcp/worker
  └─ JWTAuthMiddleware: extract X-API-Key, validate JWT
       └─ RequireBee / RequireWorker / RequireBeeOrWorker: check token type
            └─ handler: optionally read CtxKeyWorkerID from context
```

## Out of Scope

Manual invocation of `openbee ctl` from outside a Bee/Worker process (e.g., from a developer shell). In this design, such usage requires manually setting `OPENBEE_API_KEY` to a valid token. A future `openbee token` subcommand can address this use case.

## Files Changed

| File | Change |
|------|--------|
| `internal/config/config.go` | Replace `MCPConfig` fields; update `applyDefaults` |
| `internal/config/config.yaml.tmpl` | Replace `api_key`/`worker_api_key` with `token_secret`/`token_ttl` |
| `internal/mcp/token.go` | **New** — JWT generation and validation |
| `internal/mcp/auth.go` | Replace static-key middleware with JWT middleware |
| `internal/api/router.go` | Update `ServerParams` and `registerMCPRoutes` |
| `internal/app/app.go` | Update `buildAPIServer` call; remove old key validation |
| `internal/claude/invoker.go` | Remove static `apiKey`; add per-invocation `apiKey` to `Run()` |
| `internal/worker/manager.go` | Generate Worker JWT at dispatch time |
| `internal/bee/bee_process.go` | Generate Bee JWT at spawn time |
| `cmd/openbee/config.go` | Update `configValues`, `loadExistingConfig`, wizard steps |
| `internal/config/config_bee_test.go` | Update test fixtures that reference old `api_key`/`worker_api_key` fields |
