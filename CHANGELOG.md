# Changelog

## [Unreleased]

### Added

- `bee`: parallel session dispatch — each session now runs in its own goroutine, bounded by a semaphore (`max_concurrent_bee`, default 5)
- `store`: `ClaimBatch` selects at most one message per session key and skips sessions already in `feeding` status (FIFO within session)
- `config`: `max_concurrent_bee` field in `FeederConfig`; exposed in config template and interactive wizard

### Changed

- `store`: add `inPlaceholders` / `nullInt64Ptr` helpers; unify scan loops in execution/task stores
- `mcp`: `toolGetWorkerStatus` uses targeted `GetRunningByWorkerID` query instead of full history scan

### Fixed

- `api`: extract `localSessionKey` helper to deduplicate session key construction across local chat handlers
- `api`: sanitize upload filename with `filepath.Base` to prevent path traversal
- `api`: remove redundant "what" comments; annotate suppressed `errcheck` with reason

## [0.0.12] - 2026-03-24

### Added

- `config`: add missing model env vars (`ANTHROPIC_DEFAULT_*_MODEL`, `CLAUDE_CODE_SUBAGENT_MODEL`, `ENABLE_TOOL_SEARCH`) for Kimi (Moonshot) provider

## [0.0.11] - 2026-03-24

### Added

- `claudemd`: non-interactive mode rules for bee — tool substitution patterns and `send_message`-only communication
- `web`: light/dark theme switcher (system-aware, persisted to localStorage)
- `web`: server-side pagination for executions and worker detail session lists
- `mcp`: split into Bee server (19 tools) and Worker server (5 tools) with separate API keys and routes (`/mcp/bee`, `/mcp/worker`)
- `config`: Worker API Key prompt in config wizard

### Changed

- `web`: remove on-demand badge from worker cards
- `mcp`: worker tools derived from bee tools via allowlist; legacy routes removed; constructor replaced with `ServerParams` struct

### Fixed

- `config`: clean stale provider env keys when switching providers; generate `worker_api_key` when unset
- `api`: allow clearing worker description via pointer types
- `web`: suppress browser translate prompt; sync HTML `lang` attribute on language change

## [0.0.9] - 2026-03-23

### Fixed

- `bee`: enforce `SendMessage` for all user-facing replies

## [0.0.8] - 2026-03-22

### Added

- `web`: JWT authentication — login/refresh endpoints, rate limiter, React auth guard and login page
- `config`: auth configuration in wizard (username/password/jwt_secret/ttl)

## [0.0.7] - 2026-03-22

### Added

- `telegram`: user authorization via `/auth <code>`, bot menu commands, rate-limited replies for unauthorized users

## [0.0.6] - 2026-03-22

### Added

- `weixin`: WeChat platform support — receiver/sender, QR login, media upload, voice transcoding

### Changed

- `frontend`: redesign with DM Sans/JetBrains Mono; extract reusable components; advanced config section

### Fixed

- `weixin`: skip QR login when token exists
- `bee`: include merged message content in trigger_input
- `logger`: defer `logger.With()` so component loggers work after `Init()`

## [0.0.5] - 2026-03-21

### Added

- `telegram`: Telegram platform support
- `worker`: `ActiveLogRegistry` for live log streaming in bee sessions

### Fixed

- `bee`: fix worker name misrouting on address patterns
- `frontend`: session detail loading state and empty state

## [0.0.4] - 2026-03-20

### Added

- Execution logs stored on filesystem (`~/.openbee/logs/`) instead of database

### Removed

- `get_execution_logs` MCP tool; WebSocket log streaming endpoint

### Fixed

- `bee`: explicit worker name takes priority over whitelist routing
- `upgrade`: surface errors to stderr

## [0.0.3] - 2026-03-19

### Changed

- `store`: rename `schema_migrations` to `bee_migrations`

## [0.0.2] - 2026-03-20

### Added

- `list_session_contexts` and `clear_worker_session` MCP tools
- `list_tasks` filtering by `worker_id`; `force` param for `clear_session`
- Retry limit and failure notification for message processing

### Changed

- Task and worker config format changed to YAML frontmatter
- Bee strengthened as delegation-first coordinator
- Claude binary download via GitHub Releases

## [0.0.1] - 2026-03-19

### Added

- Daemon mode (`--daemon/-d`) with `restart`/`status`/`stop` commands and PID management
- All database tables prefixed with `bee_`
- Homebrew and Scoop installation
