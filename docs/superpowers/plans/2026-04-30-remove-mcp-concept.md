# Remove MCP Concept Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the historical `mcp` / `MCP` naming from the openbee source tree, replacing it with `rpc` / `RPC` to accurately describe the private HTTP-RPC mechanism used by `openbee ctl`, bee, and worker processes.

**Architecture:** Single hard-cut PR. Mechanical rename only — no behavior change, no compat shims, no rewrites of the renamed packages. The work proceeds as a sequence of focused commits, each leaving the tree compilable and all tests green. Spec at `docs/superpowers/specs/2026-04-30-remove-mcp-concept-design.md`.

**Tech Stack:** Go (server, ctl, domain), TypeScript (web — Vite config only), YAML (config + i18n locales).

---

## Conventions used in this plan

- Every step shows the exact `old → new` substitution. When a file has many sites, the step lists every site by line number and old token; do them all in one pass.
- After each task: `go build ./...`, `go test ./...`, then commit.
- The smoke test in Task 9 is the final gate; do not skip it.
- Run all commands from the repo root: `/Users/tengyongzhi/work/bot-workspaces/openbee`.

---

## Task 1: Rename `internal/mcp/` package to `internal/rpc/`

**Goal:** Move the package on disk, rename `package mcp` to `package rpc`, update every import path. No type/function/method renames yet — keep the diff small and reviewable.

**Files:**
- Move: `internal/mcp/auth.go` → `internal/rpc/auth.go`
- Move: `internal/mcp/auth_test.go` → `internal/rpc/auth_test.go`
- Move: `internal/mcp/server.go` → `internal/rpc/server.go`
- Move: `internal/mcp/server_test.go` → `internal/rpc/server_test.go`
- Move: `internal/mcp/tools.go` → `internal/rpc/tools.go`
- Move: `internal/mcp/tools_test.go` → `internal/rpc/tools_test.go`
- Modify imports in: `internal/app/app.go`, `internal/routes/server.go`, `internal/routes/mcp.go`

- [ ] **Step 1: Move the directory**

```bash
git mv internal/mcp internal/rpc
```

- [ ] **Step 2: Change package declaration in every moved Go file**

Each of `internal/rpc/auth.go`, `auth_test.go`, `server.go`, `server_test.go`, `tools.go`, `tools_test.go` starts with `package mcp`. Change all six to `package rpc`.

- [ ] **Step 3: Update import paths in all importers**

Search and replace across the repo:

```bash
grep -rl '"github.com/theopenbee/openbee/internal/mcp"' --include='*.go'
```

In each file (currently `internal/app/app.go`, `internal/routes/server.go`, `internal/routes/mcp.go`):

- `"github.com/theopenbee/openbee/internal/mcp"` → `"github.com/theopenbee/openbee/internal/rpc"`
- Every qualified identifier `mcp.X` → `rpc.X` (e.g. `mcp.NewBeeServer`, `mcp.MCPServer`, `mcp.JWTAuthMiddleware`, `mcp.RequireBeeOrWorker`, `mcp.CtxKeyWorkerID`, etc.)

- [ ] **Step 4: Build**

Run: `go build ./...`
Expected: PASS (no symbol renames yet, only package path moved).

- [ ] **Step 5: Run tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: move internal/mcp to internal/rpc"
```

---

## Task 2: Rename HTTP path and routes file

**Goal:** Move the public-looking HTTP endpoint from `/mcp/bee/call` to `/rpc/bee/call`, rename the path constant, and rename the routes file. Update the one test that asserts the old path.

**Files:**
- Modify: `internal/infra/config/config.go` (lines 16-18)
- Move: `internal/routes/mcp.go` → `internal/routes/rpc.go`
- Modify: the new `internal/routes/rpc.go` (function name, path constant reference)
- Modify: `internal/routes/server.go` (call site of `registerMCPRoutes`)
- Modify: `internal/ctlclient/client.go` (uses `config.MCPBeeBasePath`)
- Modify: `internal/ctlclient/client_test.go:18` (asserts `/mcp/bee/call`)

- [ ] **Step 1: Rename the path constant in `internal/infra/config/config.go`**

Around lines 16-18:

```go
// MCP endpoint path prefixes.
const (
    MCPBeeBasePath = "/mcp/bee"
)
```

becomes

```go
// RPC endpoint path prefixes.
const (
    RPCBeeBasePath = "/rpc/bee"
)
```

- [ ] **Step 2: Move the routes file**

```bash
git mv internal/routes/mcp.go internal/routes/rpc.go
```

- [ ] **Step 3: Edit `internal/routes/rpc.go`**

Apply these edits to the moved file:

- Function name: `func (s *Server) registerMCPRoutes()` → `func (s *Server) registerRPCRoutes()`
- Body: `config.MCPBeeBasePath+"/call"` → `config.RPCBeeBasePath+"/call"`

(The middleware reference `s.MCPAuthMiddleware` and the field `s.BeeMCP` stay for now — they get renamed in Task 3.)

- [ ] **Step 4: Edit `internal/routes/server.go`**

- Call site: `s.registerMCPRoutes()` → `s.registerRPCRoutes()`

- [ ] **Step 5: Edit `internal/ctlclient/client.go`**

Around line 81:

- `c.BaseURL+config.MCPBeeBasePath+"/call"` → `c.BaseURL+config.RPCBeeBasePath+"/call"`

- [ ] **Step 6: Edit `internal/ctlclient/client_test.go`**

Line 18:

- `assert.Equal(t, "/mcp/bee/call", r.URL.Path)` → `assert.Equal(t, "/rpc/bee/call", r.URL.Path)`

- [ ] **Step 7: Build and test**

```bash
go build ./...
go test ./...
```

Expected: both PASS.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "refactor: rename MCP HTTP path and route registration to RPC"
```

---

## Task 3: Rename `MCPServer` type and `routes.Server` fields

**Goal:** Rename the in-package server type `MCPServer` → `RPCServer`. Rename the two fields on `routes.Server` that carry the `MCP` token. The constructor `NewBeeServer` keeps its name (it carries no `mcp` token).

**Files:**
- Modify: `internal/rpc/server.go`
- Modify: `internal/rpc/tools.go` (every `*MCPServer` method receiver)
- Modify: `internal/rpc/server_test.go`, `tools_test.go` (any `*MCPServer` references)
- Modify: `internal/routes/server.go` (struct fields)
- Modify: `internal/routes/rpc.go` (uses `s.BeeMCP`, `s.MCPAuthMiddleware`)
- Modify: `internal/app/app.go` (variable `beeMCPSrv`, parameter type, struct-literal field names)

- [ ] **Step 1: Rename the type in `internal/rpc/server.go`**

- `type MCPServer struct {` → `type RPCServer struct {`
- `func NewBeeServer(...) *MCPServer {` → `func NewBeeServer(...) *RPCServer {`
- `return &MCPServer{` → `return &RPCServer{`
- `func (s *MCPServer) workerIDContext(...)` → `func (s *RPCServer) workerIDContext(...)`
- `func (s *MCPServer) HandleCall(...)` → `func (s *RPCServer) HandleCall(...)`
- Update the doc comment `// MCPServer dispatches tool calls.` → `// RPCServer dispatches tool calls.`
- Update doc comment `// NewBeeServer creates a Bee MCP Server with all tools.` → `// NewBeeServer creates a Bee RPC Server with all tools.`

- [ ] **Step 2: Update method receivers in `internal/rpc/tools.go`**

Every receiver `func (s *MCPServer) ...` → `func (s *RPCServer) ...`. Run:

```bash
grep -n "func (s \*MCPServer)" internal/rpc/tools.go
```

Replace each match. (Same for any helper functions taking `*MCPServer` as a parameter.)

- [ ] **Step 3: Update `*MCPServer` references in `internal/rpc/server_test.go` and `internal/rpc/tools_test.go`**

```bash
grep -n "MCPServer" internal/rpc/*.go
```

Replace each occurrence with `RPCServer`.

- [ ] **Step 4: Rename `routes.Server` fields in `internal/routes/server.go`**

Around lines 27-28:

- `BeeMCP            *mcp.MCPServer` → `BeeRPC            *rpc.RPCServer`
- `MCPAuthMiddleware gin.HandlerFunc` → `RPCAuthMiddleware gin.HandlerFunc`

(The `mcp.MCPServer` reference is now `rpc.RPCServer` after Task 1's import rewrite — confirm no stragglers by `grep -n "mcp\." internal/routes/server.go`; should be empty.)

- [ ] **Step 5: Update field accesses in `internal/routes/rpc.go`**

- `s.MCPAuthMiddleware` → `s.RPCAuthMiddleware`
- `s.BeeMCP.HandleCall` → `s.BeeRPC.HandleCall`
- The middleware function `mcp.RequireBeeOrWorker()` was renamed to `rpc.RequireBeeOrWorker()` in Task 1 — confirm via grep.

- [ ] **Step 6: Update `internal/app/app.go`**

Around line 174:

- `beeMCPSrv := mcp.NewBeeServer(...)` → `beeRPCSrv := rpc.NewBeeServer(...)`

Around line 214:

- `srv, err := buildAPIServer(cfg.Server, cfg.Bee.MCP, s, mgr, beeMCPSrv, ...)` → `srv, err := buildAPIServer(cfg.Server, cfg.Bee.MCP, s, mgr, beeRPCSrv, ...)`

(The `cfg.Bee.MCP` reference is intentionally untouched here — it gets renamed in Task 5.)

Around line 333:

- `func buildAPIServer(serverCfg config.ServerConfig, mcpCfg config.MCPConfig, s appStores, mgr *worker.Manager, beeMCPSrv *mcp.MCPServer, ...)` →
  `func buildAPIServer(serverCfg config.ServerConfig, mcpCfg config.MCPConfig, s appStores, mgr *worker.Manager, beeRPCSrv *rpc.RPCServer, ...)`

(Note: `mcpCfg`, `config.MCPConfig` stay until Task 5.)

Around line 339:

- `mcpAuthMiddleware := mcp.JWTAuthMiddleware(mcpCfg.TokenSecret)` is already `rpc.JWTAuthMiddleware(mcpCfg.TokenSecret)` after Task 1. Local var `mcpAuthMiddleware` stays for now.

Around lines 353-354 (struct literal building `routes.Server`):

- `BeeMCP:            beeMCPSrv,` → `BeeRPC:            beeRPCSrv,`
- `MCPAuthMiddleware: mcpAuthMiddleware,` → `RPCAuthMiddleware: mcpAuthMiddleware,`

- [ ] **Step 7: Build and test**

```bash
go build ./...
go test ./...
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "refactor: rename MCPServer to RPCServer and routes.Server fields"
```

---

## Task 4: Rename JWT claims type and context keys

**Goal:** Rename `auth.MCPClaims` → `auth.RPCClaims`. Move the three Gin context key string values from `mcp.token.*` to `rpc.token.*`. Update the doc comment that calls the JWT "MCP JWT".

**Files:**
- Modify: `internal/infra/auth/token.go` (lines 15-60: type + constructors)
- Modify: `internal/rpc/auth.go` (constants `CtxKeyTokenType/WorkerID/Scopes`, doc comment)
- Modify: any file referencing `auth.MCPClaims` (grep first; expect just the auth package and tests)

- [ ] **Step 1: Edit `internal/infra/auth/token.go`**

- Line 15 doc comment: `// MCPClaims are the JWT claims embedded in every MCP token.` → `// RPCClaims are the JWT claims embedded in every RPC token.`
- Line 16: `type MCPClaims struct {` → `type RPCClaims struct {`
- Line 24 (inside `GenerateBeeToken`): `return signToken(MCPClaims{` → `return signToken(RPCClaims{`
- Line 33 (inside `GenerateWorkerToken`): `return signToken(MCPClaims{` → `return signToken(RPCClaims{`
- Line 43: `func ValidateToken(tokenStr, secret string) (*MCPClaims, error) {` → `func ValidateToken(tokenStr, secret string) (*RPCClaims, error) {`
- Line 44 (inside `ValidateToken`): `token, err := jwt.ParseWithClaims(tokenStr, &MCPClaims{}, ...)` → `..., &RPCClaims{}, ...`
- Line 53: `claims, ok := token.Claims.(*MCPClaims)` → `claims, ok := token.Claims.(*RPCClaims)`
- Line 60 (helper): `func signToken(claims MCPClaims, secret string) (string, error) {` → `func signToken(claims RPCClaims, secret string) (string, error) {`

- [ ] **Step 2: Edit `internal/rpc/auth.go`**

Around lines 11-13 — change the three constant string values (the constant names stay; only the values change to keep the wire-side cleaner):

```go
CtxKeyTokenType = "rpc.token.type"
CtxKeyWorkerID  = "rpc.token.worker_id"
CtxKeyScopes    = "rpc.token.scopes"
```

Line 16: `// JWTAuthMiddleware validates the MCP JWT and writes claims to gin.Context.` → `// JWTAuthMiddleware validates the RPC JWT and writes claims to gin.Context.`

- [ ] **Step 3: Sweep for any other `MCPClaims` references**

```bash
grep -rn "MCPClaims" --include='*.go'
```

Expected after Steps 1-2: no matches. (Tests in `internal/infra/auth/token_test.go` use only the public functions and don't name the claims type — verify.)

- [ ] **Step 4: Build and test**

```bash
go build ./...
go test ./...
```

Expected: PASS. Token format is unchanged; only the Go type name changed.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: rename MCPClaims to RPCClaims and switch context keys to rpc.token.*"
```

---

## Task 5: Rename config types, YAML keys, and base URL field

**Goal:** Move the configuration block from `bee.mcp:` to `bee.rpc:`. Rename `MCPConfig` → `RPCConfig`, `BeeConfig.MCP` → `BeeConfig.RPC`, `MCPBaseURL` → `RPCBaseURL`, in source, template, and tests. Update every reader (`worker.Manager`, `bee.Process`, `ctlclient`, `app.go`, etc.).

**Files:**
- Modify: `internal/infra/config/config.go` (lines 122, 126, 229-..., 277, 341-348)
- Modify: `internal/infra/config/config.yaml.tmpl` (lines 37-39)
- Modify: `internal/infra/config/config_bee_test.go` (lines 27-28)
- Modify: `internal/domain/worker/manager.go` (lines 60-61)
- Modify: `internal/domain/worker/manager_test.go` (lines 79-80)
- Modify: `internal/domain/bee/bee_process.go` (lines 37-38)
- Modify: `internal/ctlclient/client.go` (lines 41, 44)
- Modify: `internal/app/app.go` (lines 214, 333, 339; rename local var `mcpCfg`)
- Modify: `config.yaml` at the repo root if present (rename the `bee.mcp:` block to `bee.rpc:`)

- [ ] **Step 1: Edit `internal/infra/config/config.go`**

- Line 122 (inside `BeeConfig`): `MCP             MCPConfig          \`yaml:"mcp"\`` → `RPC             RPCConfig          \`yaml:"rpc"\``
- Line 126: `MCPBaseURL string \`yaml:"-"\`` → `RPCBaseURL string \`yaml:"-"\`` (keep the comment, just rename the field)
- Line 229: `type MCPConfig struct {` → `type RPCConfig struct {`
- Line 277 (inside `Load` defaults): `cfg.Bee.MCPBaseURL = fmt.Sprintf(...)` → `cfg.Bee.RPCBaseURL = fmt.Sprintf(...)`
- Lines 341-348 (defaults block):
  - `cfg.Bee.MCP.TokenSecret == ""` → `cfg.Bee.RPC.TokenSecret == ""`
  - `cfg.Bee.MCP.TokenSecret = config.GenerateRandomSecret()` → `cfg.Bee.RPC.TokenSecret = ...`
  - `cfg.Bee.MCP.TokenTTL == 0` → `cfg.Bee.RPC.TokenTTL == 0`
  - `cfg.Bee.MCP.TokenTTL = 2 * time.Hour` → `cfg.Bee.RPC.TokenTTL = ...`

- [ ] **Step 2: Edit `internal/infra/config/config.yaml.tmpl`**

Lines 37-39:

```
  mcp:
    token_secret: "{{.MCPTokenSecret}}"
    token_ttl: {{.MCPTokenTTL}}
```

becomes

```
  rpc:
    token_secret: "{{.RPCTokenSecret}}"
    token_ttl: {{.RPCTokenTTL}}
```

(The `{{.RPCTokenSecret}}` / `{{.RPCTokenTTL}}` placeholders are renamed in Task 6.)

- [ ] **Step 3: Edit `internal/infra/config/config_bee_test.go`**

Lines 27-28:

- `if cfg.Bee.MCPBaseURL != "http://localhost:8080" {` → `if cfg.Bee.RPCBaseURL != "http://localhost:8080" {`
- `t.Errorf("MCPBaseURL: want http://localhost:8080 got %q", cfg.Bee.MCPBaseURL)` → `t.Errorf("RPCBaseURL: want http://localhost:8080 got %q", cfg.Bee.RPCBaseURL)`

- [ ] **Step 4: Edit `internal/domain/worker/manager.go`**

Lines 60-61 (inside the constructor that builds a `Manager` from `BeeConfig`):

- `tokenSecret:     bc.MCP.TokenSecret,` → `tokenSecret:     bc.RPC.TokenSecret,`
- `tokenTTL:        bc.MCP.TokenTTL,` → `tokenTTL:        bc.RPC.TokenTTL,`

- [ ] **Step 5: Edit `internal/domain/worker/manager_test.go`**

Lines 79-80: identical substitution to Step 4.

- [ ] **Step 6: Edit `internal/domain/bee/bee_process.go`**

Lines 37-38:

- `tokenSecret:    cfg.MCP.TokenSecret,` → `tokenSecret:    cfg.RPC.TokenSecret,`
- `tokenTTL:       cfg.MCP.TokenTTL,` → `tokenTTL:       cfg.RPC.TokenTTL,`

- [ ] **Step 7: Edit `internal/ctlclient/client.go`**

Around lines 41, 44 (inside `NewClient`):

- `baseURL = cfg.Bee.MCPBaseURL` → `baseURL = cfg.Bee.RPCBaseURL`
- `if apiKey == "" && cfg.Bee.MCP.TokenSecret != "" {` → `if apiKey == "" && cfg.Bee.RPC.TokenSecret != "" {`
- `token, err := auth.GenerateBeeToken(cfg.Bee.MCP.TokenSecret, cfg.Bee.MCP.TokenTTL)` → `token, err := auth.GenerateBeeToken(cfg.Bee.RPC.TokenSecret, cfg.Bee.RPC.TokenTTL)`

Also update the package doc comment at the top of the file:

- `// Package ctlclient provides an HTTP client for the openbee /mcp/bee/call endpoint,` → `// Package ctlclient provides an HTTP client for the openbee /rpc/bee/call endpoint,`
- `// Client calls the openbee /mcp/bee/call endpoint.` → `// Client calls the openbee /rpc/bee/call endpoint.`
- The comment block listing config-loading priority that mentions "MCP" — none uses the literal word `MCP` for the env var (`OPENBEE_API_KEY` is unchanged), so there is nothing else to change here. Confirm with `grep -n "MCP\|mcp" internal/ctlclient/client.go`.

- [ ] **Step 8: Edit `internal/app/app.go`**

- Line 214: change `cfg.Bee.MCP` → `cfg.Bee.RPC` in the call to `buildAPIServer`.
- Line 263: `os.Setenv("OPENBEE_URL", cfg.MCPBaseURL)` → `os.Setenv("OPENBEE_URL", cfg.RPCBaseURL)`
- Line 333: parameter `mcpCfg config.MCPConfig` → `rpcCfg config.RPCConfig`
- Line 339: `mcpAuthMiddleware := rpc.JWTAuthMiddleware(mcpCfg.TokenSecret)` → `rpcAuthMiddleware := rpc.JWTAuthMiddleware(rpcCfg.TokenSecret)`
- Line 353-ish: `RPCAuthMiddleware: mcpAuthMiddleware,` → `RPCAuthMiddleware: rpcAuthMiddleware,`
- Sweep the rest of `app.go` for any leftover `mcp` / `MCP` token: `grep -n "mcp\|MCP" internal/app/app.go` should be empty.

- [ ] **Step 9: Update the user-side `config.yaml` at the repo root**

```bash
test -f config.yaml && grep -n "^  mcp:" config.yaml
```

If the file exists with `bee.mcp:`, edit it: rename the block from `mcp:` to `rpc:`. (This is a sample/dev config; production users handle their own YAML.)

- [ ] **Step 10: Build and test**

```bash
go build ./...
go test ./...
```

Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add -A
git commit -m "refactor: rename bee.mcp config block to bee.rpc"
```

---

## Task 6: Rename config-wizard variables in `cmd/openbee/config.go`

**Goal:** Rename the wizard struct fields `MCPTokenSecret` / `MCPTokenTTL` → `RPCTokenSecret` / `RPCTokenTTL` and update each reference. The i18n messages those references read are renamed in Task 7.

**Files:**
- Modify: `cmd/openbee/config.go` (lines 50-51, 138-139, 199, 649-672)

- [ ] **Step 1: Rename the struct fields**

Around lines 50-51 (the struct that holds wizard answers):

- `MCPTokenSecret   string` → `RPCTokenSecret   string`
- `MCPTokenTTL      string` → `RPCTokenTTL      string`

- [ ] **Step 2: Update prefilled values from existing config**

Around lines 138-139:

- `MCPTokenSecret:        cfg.Bee.MCP.TokenSecret,` → `RPCTokenSecret:        cfg.Bee.RPC.TokenSecret,`
- `MCPTokenTTL:           cfg.Bee.MCP.TokenTTL.String(),` → `RPCTokenTTL:           cfg.Bee.RPC.TokenTTL.String(),`

(After Task 5, `cfg.Bee.MCP` no longer exists — these references already need to be `RPC`. If `go build` failed at the end of Task 5, it was here. Treat this step as the corresponding Task-5 fix-up.)

- [ ] **Step 3: Update default-values map**

Line 199: `MCPTokenTTL:            "2h",` → `RPCTokenTTL:            "2h",`

- [ ] **Step 4: Update wizard prompt logic**

Lines 649-672 region:

- `vals.MCPTokenSecret != ""` → `vals.RPCTokenSecret != ""`
- `Message: i18n.M.Prompt.MCPTokenRegenConfirm,` → `Message: i18n.M.Prompt.RPCTokenRegenConfirm,` (the i18n field is renamed in Task 7; this step assumes that rename — see ordering note below)
- `vals.MCPTokenSecret = config.GenerateRandomSecret()` → `vals.RPCTokenSecret = config.GenerateRandomSecret()` (every occurrence in this block)
- `fmt.Println(i18n.M.Output.Config.MCPTokenSecretRegenerated)` → `fmt.Println(i18n.M.Output.Config.RPCTokenSecretRegenerated)`
- `fmt.Printf(i18n.M.Output.Config.MCPTokenSecretGenerated+"\n", vals.MCPTokenSecret)` → `fmt.Printf(i18n.M.Output.Config.RPCTokenSecretGenerated+"\n", vals.RPCTokenSecret)`
- `if vals.MCPTokenSecret == "" {` → `if vals.RPCTokenSecret == "" {`
- final assignment `vals.MCPTokenSecret = config.GenerateRandomSecret()` → `vals.RPCTokenSecret = config.GenerateRandomSecret()`

**Ordering note:** Steps 1-4 reference `i18n.M.Prompt.RPCTokenRegenConfirm` etc. before those fields exist. **Do Task 7 first, then come back and finish this Task 6 — or merge the two tasks into one commit.** Recommended: combine Tasks 6 + 7 into one commit because the references are circular.

- [ ] **Step 5: Build and test (after also doing Task 7)**

```bash
go build ./...
go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit (combined with Task 7)**

```bash
git add -A
git commit -m "refactor: rename config-wizard MCPToken* fields and i18n keys to RPC"
```

---

## Task 7: Rename i18n keys, struct fields, and YAML content

**Goal:** Rename the i18n message struct fields and YAML keys/blocks that carry `mcp`. The displayed text changes from "MCP Token Secret" to "RPC Token Secret".

**Files:**
- Modify: `internal/infra/i18n/messages.go` (lines 90, 230, 231, 285, 366, 367)
- Modify: `internal/infra/i18n/locales/en.yaml` (lines 79, 151, 152, 224)
- Modify: `internal/infra/i18n/locales/zh.yaml` (lines 79, 151, 152, 224)

(See Task 6 ordering note: do this task in the same commit as Task 6.)

- [ ] **Step 1: Edit `internal/infra/i18n/messages.go`**

- Line 90: `MCPTokenRegenConfirm     string \`yaml:"mcp_token_regen_confirm"\`` → `RPCTokenRegenConfirm     string \`yaml:"rpc_token_regen_confirm"\``
- Line 230: `MCPTokenSecretGenerated    string \`yaml:"mcp_token_secret_generated"\`    // contains %s` → `RPCTokenSecretGenerated    string \`yaml:"rpc_token_secret_generated"\`    // contains %s`
- Line 231: `MCPTokenSecretRegenerated  string \`yaml:"mcp_token_secret_regenerated"\`` → `RPCTokenSecretRegenerated  string \`yaml:"rpc_token_secret_regenerated"\``
- Line 285: `MCP             MCPRuntimeMessages        \`yaml:"mcp"\`` → `RPC             RPCRuntimeMessages        \`yaml:"rpc"\``
- Line 366: `// MCPRuntimeMessages holds MCP tool runtime text sent back to the bee agent.` → `// RPCRuntimeMessages holds RPC tool runtime text sent back to the bee agent.`
- Line 367: `type MCPRuntimeMessages struct {` → `type RPCRuntimeMessages struct {`

- [ ] **Step 2: Find any consumers of `i18n.M.MCP.*` (the runtime block)**

```bash
grep -rn "i18n\.M\.MCP\b" --include='*.go'
```

Replace each `i18n.M.MCP` with `i18n.M.RPC`.

- [ ] **Step 3: Edit `internal/infra/i18n/locales/en.yaml`**

- Line 79: `mcp_token_regen_confirm: "MCP Token Secret already exists. Regenerate?"` → `rpc_token_regen_confirm: "RPC Token Secret already exists. Regenerate?"`
- Line 151: `mcp_token_secret_generated: "Generated MCP Token Secret: %s"` → `rpc_token_secret_generated: "Generated RPC Token Secret: %s"`
- Line 152: `mcp_token_secret_regenerated: "MCP Token Secret regenerated."` → `rpc_token_secret_regenerated: "RPC Token Secret regenerated."`
- Line 224: `  mcp:` → `  rpc:`

- [ ] **Step 4: Edit `internal/infra/i18n/locales/zh.yaml`**

- Line 79: `mcp_token_regen_confirm: "MCP Token Secret 已存在，是否重新生成？"` → `rpc_token_regen_confirm: "RPC Token Secret 已存在，是否重新生成？"`
- Line 151: `mcp_token_secret_generated: "已生成 MCP Token Secret：%s"` → `rpc_token_secret_generated: "已生成 RPC Token Secret：%s"`
- Line 152: `mcp_token_secret_regenerated: "MCP Token Secret 已重新生成。"` → `rpc_token_secret_regenerated: "RPC Token Secret 已重新生成。"`
- Line 224: `  mcp:` → `  rpc:`

- [ ] **Step 5: Build and test (combined with Task 6)**

```bash
go build ./...
go test ./...
```

Expected: PASS.

(Commit happens with Task 6.)

---

## Task 8: Update web proxy, CONTRIBUTING.md, and remaining doc comments

**Goal:** Catch the non-Go surfaces and any leftover doc comments that mention MCP.

**Files:**
- Modify: `web/vite.config.ts`
- Modify: `CONTRIBUTING.md`
- Modify: `internal/infra/utils/toolnames.go` (package comment)
- Modify: `internal/domain/task/dispatcher.go` (line 252 comment)
- Sweep all `*.go` files for residual `MCP` / `mcp` tokens in comments

- [ ] **Step 1: Edit `web/vite.config.ts`**

Around line 15, in the dev-server `proxy` config:

- `'/mcp': "http://localhost:8080",` → `'/rpc': "http://localhost:8080",`

(If the web build calls into the API directly — search for `'/mcp'` in `web/`: `grep -rn "/mcp" web/src` — replace any direct call sites too.)

- [ ] **Step 2: Edit `CONTRIBUTING.md`**

Around line 102 in the directory tree:

- `│   ├── mcp/           # Model Context Protocol` → `│   ├── rpc/           # internal RPC for ctl/bee/worker callbacks`

- [ ] **Step 3: Edit `internal/infra/utils/toolnames.go`**

Line 1: `// Package toolnames defines MCP tool name constants as the single source of truth.` → `// Package toolnames defines RPC tool name constants as the single source of truth.`

- [ ] **Step 4: Edit `internal/domain/task/dispatcher.go`**

Line 252 (the comment): `// can call mark_task_success and send_message via MCP.` → `// can call mark_task_success and send_message via RPC.`

- [ ] **Step 5: Sweep for residual `MCP` / `mcp` in code/comments**

```bash
grep -rn "MCP\|Mcp\|mcp" --include='*.go' --include='*.ts' --include='*.tsx' --include='*.yaml' --include='*.yml' --include='*.md' \
  | grep -v CHANGELOG.md \
  | grep -v skills-lock.json \
  | grep -v docs/superpowers/specs/2026-04-30-remove-mcp-concept-design.md \
  | grep -v docs/superpowers/plans/2026-04-30-remove-mcp-concept.md
```

Expected: empty. Any hits are real and must be fixed before commit. Common stragglers: stale comments referencing "MCP server" or "MCP tool" inside `internal/rpc/tools.go`. Rename them.

- [ ] **Step 6: Build and test**

```bash
go build ./...
go test ./...
```

Expected: PASS.

- [ ] **Step 7: (Web) Build the frontend**

```bash
cd web && pnpm install --frozen-lockfile && pnpm build && cd ..
```

(If the project uses npm or yarn, substitute. Goal is simply: web compiles.) Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "refactor: scrub remaining MCP references in comments, web proxy, and docs"
```

---

## Task 9: Final verification and changelog entry

**Goal:** Confirm the rename is complete, add a CHANGELOG entry documenting the breaking change, and run a manual smoke test.

**Files:**
- Modify: `CHANGELOG.md` (new entry at the top under the Unreleased / next-version section)

- [ ] **Step 1: Verify no MCP tokens remain outside allowed locations**

```bash
grep -rn "MCP\|Mcp\|mcp" --include='*.go' --include='*.ts' --include='*.tsx' --include='*.yaml' --include='*.yml' --include='*.md' \
  | grep -v CHANGELOG.md \
  | grep -v skills-lock.json \
  | grep -v docs/superpowers/specs/2026-04-30-remove-mcp-concept-design.md \
  | grep -v docs/superpowers/plans/2026-04-30-remove-mcp-concept.md
```

Expected: no output.

- [ ] **Step 2: Add CHANGELOG entry**

Open `CHANGELOG.md` and add to the Unreleased / next-version section (matching whatever heading style the file already uses):

```
### Breaking Changes
- Renamed the internal `mcp` concept to `rpc`. Config users must rename `bee.mcp:` to `bee.rpc:` in their `config.yaml`, or re-run `openbee config init`. The HTTP endpoint moved from `/mcp/bee/call` to `/rpc/bee/call`. Any reverse proxy or external script hardcoding `/mcp` must be updated.
```

(Match the file's existing English-only convention — see the prior changelog entries for tone.)

- [ ] **Step 3: Build and test (full)**

```bash
go build ./...
go test ./...
cd web && pnpm build && cd ..
```

Expected: all PASS.

- [ ] **Step 4: Manual smoke test**

Start the server in one terminal:

```bash
go run ./cmd/openbee server
```

In another terminal, exercise the renamed surface:

```bash
# 1. ctl talks to the new /rpc/bee/call endpoint
openbee ctl worker list

# 2. The HTTP path is correct (the test in client_test.go covers this, but verify)
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/mcp/bee/call
# Expected: 404

curl -s -X POST -H "Content-Type: application/json" -H "X-API-Key: <bee-token>" \
  -d '{"name":"list_workers","arguments":{}}' \
  http://localhost:8080/rpc/bee/call
# Expected: 200 with {"result": ...}
```

Spawn a real bee task end-to-end via whatever platform the dev environment has wired up (e.g., Feishu) and verify a worker lifecycle completes (create_task → worker runs → send_message → mark_task_success).

- [ ] **Step 5: Commit changelog**

```bash
git add CHANGELOG.md
git commit -m "docs: changelog entry for mcp -> rpc rename"
```

- [ ] **Step 6: Push and open PR**

```bash
git push -u origin <branch>
gh pr create --title "refactor: remove mcp concept, rename to rpc" --body "$(cat <<'EOF'
## Summary
- Renames the historical `mcp` concept to `rpc` across the source tree (Go packages, HTTP routes, config keys, JWT plumbing, i18n, web proxy, docs).
- Hard cut — no compat shims. See `docs/superpowers/specs/2026-04-30-remove-mcp-concept-design.md` for the full rename map.

## Breaking changes
- `bee.mcp:` config block must be renamed to `bee.rpc:` (or re-run `openbee config init`).
- `POST /mcp/bee/call` is gone; the endpoint is now `POST /rpc/bee/call`.

## Test plan
- [x] `go build ./...`
- [x] `go test ./...`
- [x] `pnpm build` (web)
- [x] Smoke: `openbee ctl worker list` round-trips via `/rpc/bee/call`
- [x] Smoke: full bee → worker lifecycle on dev platform

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-review (run after writing this plan)

- [x] Spec coverage — every section of `2026-04-30-remove-mcp-concept-design.md` is implemented:
  - Go package rename → Task 1
  - HTTP routes → Task 2
  - `MCPServer` type + `routes.Server` fields → Task 3
  - JWT claims + context keys → Task 4
  - Config types/YAML/`MCPBaseURL` → Task 5
  - Config-wizard struct + i18n → Tasks 6 + 7
  - Web proxy + docs + comments → Task 8
  - Validation + CHANGELOG entry → Task 9
- [x] No placeholders. Every step has the exact `old → new` substitution or shell command.
- [x] Type consistency. `RPCServer`, `RPCConfig`, `RPCClaims`, `RPCBaseURL`, `RPCTokenSecret/RPCTokenTTL`, `BeeRPC`, `RPCAuthMiddleware`, `RPCBeeBasePath`, `registerRPCRoutes`, `i18n.M.RPC`, `RPCRuntimeMessages` are all consistent across tasks. Constructor `NewBeeServer` and helpers `GenerateBeeToken`, `GenerateWorkerToken`, `ValidateToken` keep their names.
- [x] Tasks 6 and 7 have a noted ordering dependency and are committed together.

---

## Execution choice

Plan complete and saved to `docs/superpowers/plans/2026-04-30-remove-mcp-concept.md`. Two execution options:

**1. Subagent-Driven (recommended)** — fresh subagent per task, review between tasks, fast iteration.
**2. Inline Execution** — execute tasks in this session using `executing-plans`, batch with checkpoints for review.

Which approach?
