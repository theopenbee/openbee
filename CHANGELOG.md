# Changelog

## [Unreleased]

### Removed

- Remove the `recent_executions` field from `ctl system overview` / `get_system_overview`; use the `executions` array on `ctl task list` for per-task history.
- Remove the `openbee ctl execution` subcommand, the `list_executions` tool, and the `read:executions` scope; execution records are now returned inline by `ctl task list`.
- Remove the `openbee ctl system executions` subcommand and the `list_bee_executions` tool; bee execution history is no longer exposed via CLI or MCP.

### Changed

- `ctl task list` / `list_tasks` now returns each task with its execution records in an `executions` array, paginated with a per-task history limit.
- Executions reference their task via `task_id` (replacing `bee_tasks.execution_id`).

### Fixed

- Fix `/clear <worker>` leaving the worker's running tasks in `running` state; the per-worker clear now stops live executions and drains the worker's queue.
- Fix `/status` reporting tasks as `running` after they finished; a periodic reconciler now syncs task state with the underlying execution and sweeps orphaned rows.

## [0.0.38] - 2026-05-31

### Removed

- Remove `openbee claude download` and `openbee claude env` subcommands

## [0.0.37] - 2026-05-11

### Added
- Add `/list [keyword]` command

### Changed
- Enhance `/clear` command output with more detailed information

## [0.0.36] - 2026-05-10

### Changed
- Restructure the skill-loading instruction in session prompts into a multi-step prompt format to improve skill load rate

## [0.0.35] - 2026-05-06

### Breaking Changes
- Renamed the internal `mcp` concept to `rpc`

### Added
- Add Linear platform

## [0.0.34] - 2026-04-29

### Added
- Add `/status` command
- Add 404 Not Found page

### Changed
- Optimize `/engine` command: separate busy-check conditions for the scheduler (bee) and worker independently, so each can be evaluated and switched on its own criteria

### Fixed
- Fix execution records stuck in `running` state when the underlying process exits without a terminal signal (killed, crashed, or cancelled), which previously caused busy-checks to misreport workers as occupied
- Reset orphaned executions to `failed` at server startup so a crash or restart no longer leaves zombie `running` records behind

## [0.0.33] - 2026-04-28

### Added
- Add Xiaomi Mimo provider support

### Fixed
- Fix orphan processes left behind when a task is cancelled
- Fix bee working directory not being created on fresh installs, causing tasks to fail with a "chdir" error (especially on Windows)

## [0.0.32] - 2026-04-27

### Added
- Add worker name editing support
- Add agent token usage statistics
- Add engine args support at both global and worker levels, allowing different workers to be configured with different models, thinking depth, and other engine-specific options

### Fixed
- Fix a bug where cancelled tasks were still being executed

## [0.0.31] - 2026-04-22

### Added
- Add `/stop` command support
- Pass more IM platform message data to AI so that AI can leverage IM platform CLI to implement richer features
- Worker name must be globally unique and cannot conflict with any bot name

### Changed
- Messages starting with `@workerName` + space/newline or `workerName` + space/newline are now directly dispatched to the corresponding worker

### Fixed
- Fix DingTalk reaction 500 error caused by sending English emotion text ("🤔 Thinking...") instead of the required Chinese text ("🤔思考中"); also downgrade emoji failure log level from Error to Warn since it is non-blocking.

## [0.0.30] - 2026-04-21

### Added
- Add random worker name generation feature
- Stale messages are automatically skipped

### Changed
- Rename worker memory concept to worker constraints to avoid conflicts with the agent's built-in memory feature
- Rename bee coordinator memory management to constraint management to avoid conflicts with the agent's built-in memory feature
- Feishu and DingTalk reaction add/recall now retry up to 5 times with exponential backoff (500ms base delay) on network failures, improving resilience to transient connectivity issues.

### Fixed
- Fix agent execution error message not being delivered to IM
- Fix unstable DingTalk connection caused by redundant heartbeat supervisor

## [0.0.29] - 2026-04-20

### Added
- Add platform bot name configuration that automatically strips `@BotName` mentions from incoming messages, fixing a bug where group chat commands were not recognized

## [0.0.28] - 2026-04-20

### Added
- Add confirmation dialog before cancelling a scheduled task

### Fixed
- Fix bug where resetting a session caused scheduled tasks and countdown tasks to be cancelled

## [0.0.27] - 2026-04-20

### Fixed
- Fix WeChat Work (WeCom) image decryption failure caused by unpadded base64 AES key

## [0.0.26] - 2026-04-19

### Added
- Add Kimi agent
- Add `/engine` and `/clear` commands
- Add `engine` field to workers for specifying the AI agent per worker

## [0.0.25] - 2026-04-16

### Added
- Add worker copy feature
- Add `OPENBEE_LOG_LEVEL` environment variable to control log verbosity (supported levels: `debug`, `info`, `warn`, `error`; default: `info`)

### Fixed
- Fix memory loss bug when agent switches scheduler

## [0.0.24] - 2026-04-15

### Added
- Add environment variable configuration at global, worker, department, and scheduler levels

### Fixed
- Fix agent execution result recording

### Changed
- Improve error message handling for Pi Agent

## [0.0.23] - 2026-04-14

### Added
- Dashboard adds total message count, total working hours, and working hours trend chart

### Changed
- Optimize Skills installation logic
- Optimize scheduler error parsing on GLM and Kimi

### Fixed
- Fix session clear to only check immediate tasks for pending or running status
- Fix TypeScript type error in combined-trend-chart tooltip formatter

### Removed
- Remove scheduler retry mechanism

## [0.0.22] - 2026-04-13

### Added
- Add system data query permission and grant access to workers

### Changed
- Optimize session clearing logic to improve clearing efficiency
- Improve openbee ctl command output results
- Improve openbee ctl command filter and deletion

## [0.0.21] - 2026-04-12

### Added
- Plugin-based underlying engine, added support for Codex and Pi Agent

### Changed
- Optimized advanced configuration interaction
- Optimized ctl message sending, fixed a bug where message content would be escaped and executed

## [0.0.20] - 2026-04-09

### Added
- Add direct worker dispatch via @mention syntax, bypassing the scheduler

### Changed
- Improve MCP token generation interaction
- Improve display name resolution for Feishu @mentions
- Improve message list API with pagination support

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
