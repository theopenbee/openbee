# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.0.8] - 2026-03-22

### Added

- `web`: JWT authentication for web API and frontend
- `web`: login/refresh/status auth endpoints with Bearer token and query param support
- `web`: login rate limiter (5 attempts/min per IP)
- `web`: React auth guard and login page with i18n support
- `config`: auth configuration (username/password/jwt_secret/ttl) in config wizard

### Changed

- `web`: CORS updated to support credentials when auth is enabled
- `web`: all API, SSE stream, and internal endpoints protected by JWT middleware

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
