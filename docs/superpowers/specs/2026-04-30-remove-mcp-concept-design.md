# Remove MCP Concept — Design Spec

Date: 2026-04-30
Status: Approved (brainstorm)

## Background

The `mcp` (Model Context Protocol) naming is a historical artifact. The codebase no longer exposes a real MCP server — what exists is an internal HTTP-RPC endpoint used by the in-tree `openbee ctl` CLI and by spawned bee/worker processes to call back into the server. Keeping the `mcp` label causes confusion: new contributors and users see "MCP" and assume there is an externally-spoken Model Context Protocol surface.

The goal is to eliminate every `mcp`/`MCP` token from the source tree and replace it with `rpc`/`RPC`, which accurately describes the mechanism (a private RPC over HTTP between the server, ctl, and child processes).

## Goals

- Remove the `mcp` concept from package layout, HTTP routes, configuration, JWT plumbing, web proxy, i18n, and source comments.
- Land as a single mechanical-rename PR with no behavioural changes other than the new path/key names.
- Keep the JWT scheme, token TTL semantics, env vars, and tool name constants exactly as they are today.

## Non-goals

- No backward-compat shims for the old config keys, HTTP path, or context keys.
- No rewrite of `CHANGELOG.md` history. Historical entries that mention "MCP" stay verbatim — they describe the state of the project at the time they were written.
- No refactoring of unrelated code that happens to live in the renamed packages.
- No change to environment variable names (`OPENBEE_URL`, `OPENBEE_API_KEY` already carry no `mcp`).
- No change to tool name constants (`list_workers`, etc. — none carry `mcp`).

## Approach

A hard-cut rename. After the change, building the project against an old `config.yaml` whose only token-secret lives under `bee.mcp:` will silently fall through to a freshly generated secret, invalidating any in-flight bee/worker tokens. This is acceptable; users upgrading must re-run `openbee config init` or hand-edit the YAML to use `bee.rpc:`.

## Rename map

### Go package

- `internal/mcp/` → `internal/rpc/`
- Files inside the package keep their names (`server.go`, `tools.go`, `auth.go`, plus `*_test.go`); their `package mcp` declaration becomes `package rpc`.
- All importers update `github.com/theopenbee/openbee/internal/mcp` → `.../internal/rpc`.

### HTTP routes

- Path constant: `MCPBeeBasePath = "/mcp/bee"` → `RPCBeeBasePath = "/rpc/bee"`
- Endpoint: `POST /mcp/bee/call` → `POST /rpc/bee/call`
- Routes file: `internal/routes/mcp.go` → `internal/routes/rpc.go`
- Method: `(*Server).registerMCPRoutes` → `(*Server).registerRPCRoutes`
- `routes.Server` fields: `BeeMCP` → `BeeRPC`, `MCPAuthMiddleware` → `RPCAuthMiddleware`

### RPC server type

- Type: `MCPServer` → `RPCServer`
- Constructor: `NewBeeServer` keeps its name (no `mcp` token in it).
- All call sites in `internal/app/app.go`, tests, and elsewhere update their type references.

### Configuration

- YAML: `bee.mcp.token_secret` → `bee.rpc.token_secret`; `bee.mcp.token_ttl` → `bee.rpc.token_ttl`
- Go types: `MCPConfig` → `RPCConfig`; `BeeConfig.MCP` → `BeeConfig.RPC`
- Derived URL: `BeeConfig.MCPBaseURL` → `BeeConfig.RPCBaseURL`
- Config-wizard struct fields in `cmd/openbee/config.go`: `MCPTokenSecret` → `RPCTokenSecret`; `MCPTokenTTL` → `RPCTokenTTL`
- YAML template `internal/infra/config/config.yaml.tmpl`: the `mcp:` block under `bee:` becomes `rpc:`

### JWT auth

- Type: `auth.MCPClaims` → `auth.RPCClaims`
- Gin context keys in `internal/rpc/auth.go`:
  - `mcp.token.type` → `rpc.token.type`
  - `mcp.token.worker_id` → `rpc.token.worker_id`
  - `mcp.token.scopes` → `rpc.token.scopes`
- `auth.GenerateBeeToken`, `auth.GenerateWorkerToken`, `auth.ValidateToken` keep their names; only the claims-struct rename propagates.

### i18n

- `internal/infra/i18n/locales/zh.yaml` and `en.yaml`:
  - keys `prompt.mcp_token_regen_confirm`, `output.config.mcp_token_secret_generated`, `output.config.mcp_token_secret_regenerated` → `rpc_token_*`
  - parent block `mcp:` (line 224 in zh.yaml) → `rpc:`
  - text "MCP Token Secret …" → "RPC Token Secret …"
- `internal/infra/i18n/messages.go`: matching field names follow the key rename.

### Web

- `web/vite.config.ts`: dev-server proxy entry `'/mcp': 'http://localhost:8080'` → `'/rpc': 'http://localhost:8080'`

### Documentation and comments

- `CONTRIBUTING.md`: directory-tree line `mcp/   # Model Context Protocol` → `rpc/   # internal RPC for ctl/bee/worker callbacks`
- All Go doc comments mentioning "MCP tool", "MCP server", "MCP endpoint", "MCP JWT", etc. — rewritten to use "RPC".
- `internal/infra/utils/toolnames.go` package comment: "MCP tool name constants" → "RPC tool name constants".
- `internal/ctlclient/client.go` package and method comments: replace "MCP" mentions; the `unauthorized – check OPENBEE_API_KEY` error string is unaffected.

## Items intentionally untouched

- `CHANGELOG.md` historical entries.
- `skills-lock.json` (generated, not source).
- `OPENBEE_URL`, `OPENBEE_API_KEY` environment variables.
- JWT token format, signing algorithm, default TTL (2h).
- Tool name constants and their wire-format values.

## Breaking changes

1. `bee.mcp.*` keys in an existing `config.yaml` are silently ignored after upgrade. Users must rename to `bee.rpc.*` or re-run `openbee config init`. Failing to do so causes a fresh `token_secret` to be auto-generated, invalidating any live bee/worker tokens.
2. `POST /mcp/bee/call` returns 404. Anything that hardcoded `/mcp` (reverse proxies, custom scripts) must update.
3. The Vite dev proxy moves from `/mcp` to `/rpc`. Any in-flight web work depending on the old prefix updates to `/rpc`.

These are documented in the next CHANGELOG entry as a breaking change.

## Validation

- `go build ./...` and `go test ./...` pass with no `mcp` symbol references remaining (verified via `grep -rni 'mcp' --include='*.go' --include='*.ts' --include='*.yaml'` showing only acceptable hits: CHANGELOG history, skills-lock.json).
- `web/` build (`pnpm build` or equivalent) succeeds.
- Manual smoke test: start the server, run `openbee ctl message send` against a real session, spawn a bee task, verify a worker task hits `/rpc/bee/call` successfully.

## Out-of-scope follow-ups

- A future cleanup may merge `RPCConfig` into `BeeConfig` if the only fields remaining are `token_secret` and `token_ttl` — not in this PR.
- Refactoring `tools.go` (1299 lines) into smaller files is desirable but out of scope here.
