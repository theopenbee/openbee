---
name: openbee-testenv
description: Use when an openbee change needs acceptance on a real running instance — real IM bots (Telegram, Discord, Feishu, DingTalk, WeCom, WeChat, Linear), bee/worker/task flows, plugin install or setup, or the web UI against a live server — and whenever asked to bring up, check, restart or tear down the openbee test environment.
---

# OpenBee test environment

One script owns the whole lifecycle of a test instance. Do not hand-roll configs, launchers, health checks or kill commands. If the script lacks something, extend the script instead of working around it.

```bash
T=<this skill's base directory>/scripts/testenv.py
```

The script runs from any cwd; use its absolute path, because a worktree of an older branch may not contain the skill.

## Rules

- **Only `$T` creates test instances.** No ad-hoc `/tmp` dirs, no fork launchers, no heredoc configs, no `openbee config` wizard.
- **Leave other instances alone.** The developer's real openbee (its port, its `~/.openbee`, the `openbee` binary on PATH) is never touched. Never run bare `openbee` against a test env; use `$T mcp`, `$T js` and `$T api`. If you run inside an openbee worker, `OPENBEE_URL`/`OPENBEE_API_KEY` point at the real instance; the script strips them.
- **Engine is always pi + DeepSeek inside a mock HOME.** The script installs the pi engine plugin and does this; don't switch to Claude (it is not logged in under a mock HOME) or a fake engine.
- **Secrets live only in the secrets dir** (below). When the user sends new credentials in chat, write them there (mode 0600) first, then use the script. Never put them inline in commands or configs.
- **Build from a worktree of the branch under test** (`.worktrees/<branch>`); don't switch branches in the main checkout for a test.
- **Always tear down.** Run `down` as soon as acceptance is over (the user confirms the results or says stop), and also after a failed or abandoned run. Report it in the final message. If a task ends while the user is still testing, the env may stay up, but the message must say so.

## Secrets dir

`~/.openbee-e2e/` (override with `OPENBEE_E2E_HOME`), mode 0700, never committed:

| File | Content |
|---|---|
| `deepseek-key` | DeepSeek API key for pi (required); stored as the global env var `DEEPSEEK_API_KEY` |
| `<channel>.json` | that plugin's config keys, plus optional `bot_name` |

Channel files: `telegram.json` `{"token"}`, `discord.json` `{"token"}`, `feishu.json` `{"app_id", "app_secret"}`, `dingtalk.json` `{"client_id", "client_secret"}`, `wecom.json` `{"bot_id", "secret"}`, `linear.json` `{"api_key", "projects", "states"}` (lists as comma-separated strings). Any other plugin key in the file is passed through. WeChat needs no file; it logs in by QR.

Node 24 and `pi` are taken from the newest nvm v24, else from PATH; override with `OPENBEE_E2E_NODE_BIN=<dir with node and pi>`.

## Lifecycle

### 0. Check

`$T status` lists test envs and every `OTHER openbee server` process. Those are either the developer's real instance or leftovers from old ad-hoc runs; ask before stopping any of them.

### 1. Up

```bash
$T up --src <worktree> [--engine-src <engine-pi checkout>] [--channel telegram --channel feishu ...]
```

Builds web (`pnpm install --frozen-lockfile && pnpm build`) and server from `--src` (default: the current checkout). Then it creates the mock HOME, writes a config with random 64-hex secrets (engines are not in the config), checks pi, and starts the built-in daemon (`openbee server -d`). Then it creates the admin via `/api/setup`, stores `DEEPSEEK_API_KEY` as a global env var, installs the pi engine plugin, points its `path` setting at the pi binary and waits until its state is `ready` (60s; on `failed` the plugin error is printed). Finally it creates a pi worker named `tester` and prints a JSON summary with the web URL, admin password, installed engines, DB and log paths.

Options:
- `--name` (default `e2e`; use another name for a second env)
- `--port` (default: first free in 12360-12379)
- `--skip-web` (only when `web/` is untouched since the last build)
- `--worker ""` (no worker)
- `--model deepseek-v4-pro`
- `--engine-src <dir>` installs the pi plugin from a built checkout (`openbee.plugin.json` and `dist/` present). Without it the script looks `pi` up in the plugin catalog and stops with a clear error if the catalog does not list it (engines may not be published yet).
- `--engine NAME[=DIR]` (repeatable) installs another engine plugin the same way, e.g. `--engine claude=<engine-claude checkout>`; without `=DIR` it comes from the catalog. If the plugin has a `path` setting and `NAME` is on PATH, the script sets it. pi stays the default and the only engine that can run under the mock HOME.
- `--env KEY=VALUE` (extra server env, e.g. `OPENBEE_PLUGIN_CATALOG=<catalog.json>`, or a `PATH` override for runtime tests)

### 2. Engines

```bash
$T engine NAME[=DIR]
```

Installs an engine plugin into a running env with the same steps as `up`: install, set `path`, wait for `ready`. Use it to add an engine later, or to reinstall pi after a failed `up`. Engine registrations live inside the env and go away with `down`.

### 3. Connect channels

```bash
$T channel <telegram|discord|feishu|dingtalk|wecom|linear|weixin> [--src <plugin checkout>] [--spec <npm spec>] [--set key=value]
```

- Default source is the published npm package. To test unpublished plugin code, build it (`npm ci && npm run build`) and pass `--src <plugin checkout>`.
- On builds without the plugin system, the script writes `bee.platforms.<id>` instead and restarts.
- The script refuses a channel that another running env already uses: one bot cannot serve two instances.
- Linear defaults to label `openbee-e2e` and refuses `openbee`, the label a real instance watches.

**WeChat (QR login).** Ask the user to confirm they are ready to scan first; codes expire in about 2 minutes and only 3 are issued. Then run `$T weixin-login` in the background (or under Monitor). For each `QR_PNG <path>` line, send the PNG to the user. If the setup asks for a verification code, relay it with `$T weixin-input <code>`. On success the plugin is enabled automatically.

### 4. Test

1. Send the user the checklist for each connected channel from `references/checklists.md`, in the user's language. Tell them they don't need to reply when done.
2. Watch with the Monitor tool: `$T watch`. It prints new or changed inbound messages, tasks and outbound messages, plus warn/error log lines, and exits when the server stops. Don't poll the DB faster than its default 5s.
3. Inspect with:
   - `$T db "<sql>"` for read-only queries (tables `bee_platform_messages`, `bee_tasks`, `bee_outbound_messages`, `bee_executions`, `bee_workers`)
   - `$T mcp <tool> '<json args>'` to call the MCP server as bee, e.g. `$T mcp send_message '{"session_key": "<key>", "content": "hi"}'`
   - `$T js '<code>'` (or `$T js -` to read stdin) to run an `execute` script as bee, e.g. `$T js 'return await openbee.list_workers()'`
   - `$T api <METHOD> <path> [--data '<json>']` for authenticated API calls as admin
   - a chat message to the worker: `$T api POST /api/local/conversations/worker:<worker_id>/messages --data '{"content": "..."}'`, then read the reply with `$T api GET /api/local/conversations/worker:<worker_id>/messages` (`worker_id` is in the `up` summary)
   - `$T logs [--lines N] [-f]` for the server log
4. Drive Linear and web-UI checks yourself; don't ask the user to do them.

### 5. Change code mid-test

`$T restart --rebuild [--skip-web]` rebuilds from the same `--src` and restarts, keeping DB and plugins (installed engines included). Add `--src <checkout>` to rebuild from another checkout, e.g. to upgrade an env started from an older build and test its migrations. It refuses while a message or task is in flight, because restarting then breaks the reply the user is waiting for. Wait, or pass `--force` only if the user agrees.

### 6. Down

```bash
$T down            # or: $T down --all
```

This stops the daemon, kills plugin and worker child processes, verifies the port is free, and deletes the env dir. `--keep` stops but keeps files, for post-mortems only. Afterwards:

- stop any Monitor tasks
- clean external state the test created (Linear issues and the `openbee-e2e` label; see `references/gotchas.md`)
- run `$T status` and confirm nothing is left

## Layout

```
~/.openbee-e2e/
  deepseek-key <channel>.json      secrets
  tools/                           qrcode npm package for WeChat QR PNGs
  envs/<name>/                     one per env, deleted by down
    meta.json config.yaml bin/openbee tmp/ weixin-qr-N.png
    home/                          mock HOME
      .openbee/{openbee.db,openbee.log,openbee.pid,plugins/,worker/}
      .pi/agent/settings.json
      .claude/skills .agents/skills
```

The server's `TMPDIR` points to `envs/<name>/tmp`, so worker scratch files go away with the env.

## References

- `references/checklists.md`: user test scripts per channel
- `references/gotchas.md`: platform quirks, upgrade-compat testing, DB notes
