# Changelog

## [Unreleased]

### Added
- Add direct worker dispatch via @mention syntax, bypassing the scheduler

### Changed
- Improve MCP token generation interaction
- Improve display name resolution for Feishu @mentions

### Removed
- Remove MCP registration endpoint

## [0.0.19] - 2026-04-08

### Added

- Added support for pasting images to upload in the chat input box
- Added support for Kimi Code
- Added collapsible support for long chat messages: messages exceeding a fixed height are clipped with a fade overlay and can be expanded or collapsed on demand

### Fixed

- Fixed execution error reporting to return more specific error messages
- Fixed chat message newline rendering issue
- Fixed chat message content overflow: long text, JSON, and code blocks no longer break out of message bubbles

## [0.0.18] - 2026-04-07

### Added

- Add statistics data to dashboard page
- Add department feature

### Changed

- Refactor web UI
- Complete message lifecycle closed-loop
- Refactor backend code

## [0.0.17] - 2026-04-02

### Added

- Add installation script support for China mainland CDN
- Add Claude Code download support for China mainland CDN

### Fixed

- Fix bug where brainstorming skill does not send messages

### Changed

- Default language set to English
- Optimize Feishu WebSocket proactive disconnection

## [0.0.16] - 2026-04-01

### Added

- `ctl`: CLI control interface equivalent to MCP — exposes the same capabilities as the MCP server via command-line subcommands

## [0.0.15] - 2026-03-29

### Added

- `config`: add default model mapping for Zhipu (GLM) provider — `ANTHROPIC_DEFAULT_HAIKU_MODEL` (glm-4.5-air), `ANTHROPIC_DEFAULT_SONNET_MODEL` (glm-5-turbo), `ANTHROPIC_DEFAULT_OPUS_MODEL` (glm-5.1)

## [0.0.14] - 2026-03-28

### Added

- Added backup and restore commands
- Added internationalization (i18n) support for the command-line interface. Currently supports English and Chinese.
- Added Tasks page

### Fixed

- Session detail page: show full message content with proper line breaks and paragraph formatting

## [0.0.13] - 2026-03-27

### Added

- Support canceling running tasks (`CancelTask`) to prevent tasks from being stuck indefinitely
- `bee`: parallel session dispatch — each session now runs in its own goroutine, bounded by a semaphore (`max_concurrent_bee`, default 5)

### Changed

- Improve task execution failure notifications: include worker name, retry count, and raw error in structured messages
- Scheduler computes `next_run_at` before claiming a task, eliminating the temporary 24-hour placeholder written to the database so `next_run_at` always reflects true scheduling intent

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
