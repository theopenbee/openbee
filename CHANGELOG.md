# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `config`: add missing model env vars and subagent/tool-search settings to Kimi (Moonshot) provider — `ANTHROPIC_DEFAULT_OPUS_MODEL`, `ANTHROPIC_DEFAULT_SONNET_MODEL`, `ANTHROPIC_DEFAULT_HAIKU_MODEL`, `CLAUDE_CODE_SUBAGENT_MODEL`, and `ENABLE_TOOL_SEARCH` are now included in `moonshotEnv` and registered in `providerEnvKeys` so they are cleaned up when switching providers

## [0.0.11] - 2026-03-24

### Added

- `claudemd`: add non-interactive mode rules for bee — new `beeNonInteractiveRules()` section at highest priority, defines tool substitution patterns (`AskUserQuestion` → `send_message`, `EnterPlanMode` → inline thinking) and enforces `send_message`-only communication for bee coordinator
- `claudemd`: simplify bee notification rules — remove redundant "禁止直接输出文本" block (now covered by non-interactive mode section), replace with concise message format requirement

- `web`: light theme with theme switcher — adds a system-aware light/dark toggle in the top nav, persisted to localStorage; UI adapts fully across all pages
- `web`: pagination for executions page — server-side session-level pagination with page/page_size query params, frontend pagination controls with previous/next navigation and i18n support (zh/en)
- `web`: pagination for worker detail sessions — server-side pagination for the sessions list on the worker detail page, consistent with executions page pattern
- `mcp`: tool permission isolation — split single MCP server into Bee server (full 19-tool access) and Worker server (restricted to 5 tools: send_message, mark_task_complete, save/get/delete_memory), each with its own API key and route group (`/mcp/bee`, `/mcp/worker`)
- `config`: add MCP Worker API Key prompt in config subcommand advanced settings (generate randomly / enter manually)

### Changed

- `web`: remove on-demand label from worker cards — Clock icon and "On-demand" badge removed from both dashboard and workers list pages; unused i18n key `common.onDemand` removed
- `web`: simplify theme switcher — remove redundant wrapper component, inline toggle directly in nav; deduplicate StatusBadge styles using a shared variant map
- `api`: extract `parsePagination`/`paginatedResponse` helpers in Go API handler — shared by executions and worker-detail routes
- `web`: extract `PaginationControls` React component shared by executions and worker-detail pages
- `mcp`: deduplicate tool schemas and dispatch logic — worker tools derived from bee tools via allowlist filter instead of duplication
- `mcp`: remove legacy MCP routes (`/mcp/sse`, `/mcp/messages`) — all callers now use `/mcp/bee` and `/mcp/worker` paths
- `mcp`: replace NewServer 12-positional-param constructor with self-documenting ServerParams struct

### Fixed

- `config`: clean stale provider env keys when switching model provider — previously, switching from MiniMax to GLM left MiniMax's model name in settings.json, causing the new provider to fail
- `config`: generate worker_api_key in config subcommand when not explicitly set
- `api`: allow clearing worker description — use pointer types in updateWorker handler so empty string values are properly saved instead of silently skipped
- `web`: prevent browser translate prompt — add `translate="no"` attribute and `<meta name="google" content="notranslate" />` to suppress browser translation prompts
- `web`: dynamically update HTML `lang` attribute on i18n language change to correctly reflect the current page language

## [0.0.9] - 2026-03-23

### Fixed

- `bee`: enforce SendMessage tool for all user-facing replies — users cannot see direct text output, so all responses must go through SendMessage

## [0.0.8] - 2026-03-22

### Added

- `web`: JWT authentication for web API and frontend
- `web`: login/refresh/status auth endpoints with Bearer token and query param support
- `web`: login rate limiter (5 attempts/min per IP)
- `web`: React auth guard and login page with i18n support
- `web`: all API, SSE stream, and internal endpoints protected by JWT middleware
- `config`: auth configuration (username/password/jwt_secret/ttl) in config wizard

## [0.0.7] - 2026-03-22

### Added

- `telegram`: user authorization via `/auth <code>` command with persistent auth store
- `telegram`: bot menu commands setup (`/start`, `/auth`)
- `telegram`: `auth_code` config option with auto-generation in config wizard
- `telegram`: rate-limited replies for unauthorized users (one per sender per 60s)

## [0.0.6] - 2026-03-22

### Added

- `weixin`: WeChat platform support — WeixinPlatform receiver and sender, QR code login flow, media upload with CDN encryption, SILK to WAV voice transcoding, AES-128-ECB crypto, config wizard integration

### Changed

- `frontend`: redesign UI with DM Sans and JetBrains Mono typography
- `frontend`: extract reusable components — EmptyState, FadeIn, PageHeader, SkeletonLoader, StatusBadge
- `frontend`: refine all pages with consistent layout, loading states, and styling
- `config`: move port/host/debug/dbpath/mcpkey into advanced configuration section

### Fixed

- `weixin`: allow skipping QR login when token already exists
- `bee`: include merged message content in trigger_input
- `logger`: defer logger.With() resolution so component loggers work after Init()

## [0.0.5] - 2026-03-21

### Added

- `telegram`: Telegram platform support — TelegramPlatform receiver and sender, TelegramConfig, config template section, and config wizard integration
- `worker`: ActiveLogRegistry for shared live log access across workers
- `logs`: real-time log streaming for bee session executions via ActiveLogRegistry

### Fixed

- `bee`: recognize address pattern to prevent worker name misrouting
- `frontend`: session detail page loading state; add noExecutions empty state
- `api`: only set Cache-Control header when log content is non-empty

## [0.0.4] - 2026-03-20

### Added

- Execution logs now stored on filesystem (`~/.openbee/logs/`) instead of database, enabling AI to read log files directly

### Removed

- `get_execution_logs` MCP tool (AI now reads log files directly from filesystem)
- WebSocket execution logs streaming endpoint

### Fixed

- `bee`: ensure explicit worker name takes absolute priority over whitelist routing rules
- `upgrade`: surface errors to stderr and print target binary path on completion

## [0.0.3] - 2026-03-19

### Changed

- `store`: renamed `schema_migrations` table to `bee_migrations` for namespace clarity

## [0.0.2] - 2026-03-20

### Added

- `list_session_contexts` MCP tool for querying active session contexts
- `clear_worker_session` MCP tool for clearing individual worker sessions
- `list_tasks` now supports filtering by `worker_id` for cross-session queries
- Force confirmation parameter to `clear_session` MCP tool
- Retry limit and failure notification for bee message processing

### Changed

- Task metadata format changed to YAML frontmatter
- Worker config changed to YAML frontmatter format
- Bee role strengthened as coordinator with delegation-first dispatch
- Claude binary download now uses GitHub Releases instead of CDN service
- Bee notification rules rewritten with coordinator semantics

### Fixed

- Removed redundant bee name from role preamble

## [0.0.1] - 2026-03-19

### Added

- Daemon mode with `--daemon/-d` flag for background server operation
- `restart`, `status`, `stop` commands for daemon management
- Cross-platform daemon support (Unix and Windows)
- Daemon PID file management and liveness checking
- All database tables prefixed with `bee_` namespace
- CLI strings translated to English
- Homebrew and Scoop installation methods
