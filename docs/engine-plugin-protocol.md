# Engine plugin protocol (v1)

An engine plugin lets openbee run tasks with an AI coding CLI (Claude Code, Codex, pi, ...). openbee ships no engines: install the CLI and log in to it yourself, then install the plugin that drives it. Plugins can be written in any language.

Unlike a [channel plugin](channel-plugin-protocol.md), an engine plugin does not stay running. Every time openbee needs the engine it starts the plugin command, writes one JSON request to its stdin, reads one JSON response from its stdout, and the process exits. The CLI runs as a child of that plugin process.

## Package layout

An engine plugin uses the same `openbee.plugin.json` manifest as a channel plugin, with `"type": "engine"`:

```json
{
  "protocol_version": 1,
  "type": "engine",
  "id": "claude",
  "name": { "en": "Claude Code", "zh": "Claude Code" },
  "description": { "en": "Runs tasks with Claude Code", "zh": "用 Claude Code 执行任务" },
  "command": ["node", "dist/index.js"],
  "version": "0.1.0",
  "config": [
    { "key": "path", "type": "string", "label": { "en": "Claude Code path" }, "help": { "en": "Leave empty to use claude on PATH" } }
  ]
}
```

| Field | Rules |
|---|---|
| `protocol_version` | Must be `1`. The engine protocol is versioned separately from the channel protocol. |
| `type` | Must be `engine`. Without `type` the manifest describes a channel plugin; any other value is rejected. |
| `id` | Engine id, used by workers and by the default engine setting. Matches `^[a-z][a-z0-9_-]{1,31}$`. Cannot be `local`. Channel and engine plugins share one id space. |
| `name`, `description` | As for channel plugins. `name` is shown wherever engines are listed. |
| `command` | As for channel plugins, including the [runtime](channel-plugin-protocol.md#runtime) openbee picks for `node`. Run with the plugin directory as working directory. |
| `version` | Optional. `package.json` `version` wins when present. |
| `config[]` | As for channel plugins: `type` is `string`, `secret`, `bool`, `int` or `list`; `required`, `default`, `random_default`, `label` and `help` are optional. `enabled` and `bot_name` are reserved keys. |
| `setup` | Not allowed; a manifest that declares it is rejected. Logging in to the CLI is up to the user. |

One plugin provides exactly one engine.

## Installation

Engine plugins are installed, upgraded and removed like channel plugins (see [Installation](channel-plugin-protocol.md#installation) and [Runtime](channel-plugin-protocol.md#runtime)):

```sh
openbee plugin install claude          # catalog id; npm:<name>, an npm spec or a directory also work
openbee plugin configure claude --set path=/opt/bin/claude
openbee plugin list
openbee plugin enable|disable|restart <id>
openbee plugin update [id...]
openbee plugin remove <id> [--yes]
```

`openbee plugin configure` never asks an engine for a bot name. Engines can also be installed and managed from the web console's plugin page, which uses the HTTP API described in the channel protocol's [Configuration](channel-plugin-protocol.md#configuration).

## Lifecycle

### States

`GET /api/plugins` and `GET /api/plugins/<id>` report a `type` (`channel` or `engine`) for every plugin. An engine plugin is in one of these states:

| State | Meaning |
|---|---|
| `not_configured` | No settings are saved yet, or a required setting is missing. |
| `disabled` | Configured but turned off. Workers cannot use it. |
| `starting` | Enabled; the first `check` is running. |
| `ready` | The latest `check` passed. |
| `failed` | The latest `check` failed (the error is in `error`), or the plugin could not be loaded. |

An enabled engine is usable as soon as it is registered, including while it is `starting` or `failed`. The state only tells the user whether the CLI looked usable; the CLI may be installed later. openbee does not retry a failed `check` on a timer.

openbee runs `check` for every enabled engine when it starts, and again whenever an engine is installed, upgraded, enabled, restarted or reconfigured.

### Install, upgrade, remove

- **First install enables the engine.** When no settings are saved for the id (a first install, or a reinstall after a removal), openbee saves the manifest defaults as the plugin's settings, enables it and starts the first `check`. If a required setting has no default, the plugin stays `not_configured` until the user configures it. Upgrading or reinstalling over an existing install keeps the saved settings and the enabled or disabled choice.
- **Upgrade and settings changes take effect on the next call.** There is no process to restart. A call that is already running keeps the process it started with; disabling, upgrading or removing the plugin does not interrupt it.
- **Remove** stops the engine and removes the plugin, its settings and `data_dir`. It never deletes `state_dir`. Workers that name the removed engine keep their setting and fail on their next run (see [Using an engine](#using-an-engine)).
- `PUT /api/plugins/<id>/settings` rejects a non-empty `bot_name` for an engine with HTTP 400 and code `plugin_bot_name_not_supported`.

### `data_dir` and `state_dir`

| Directory | Location | Use | Removed on uninstall |
|---|---|---|---|
| `data_dir` | `~/.openbee/plugins/data/<id>/` | The plugin's own caches and files. | Yes |
| `state_dir` | `~/.openbee/.<id>/` | Session state: session id mappings, session files. | No |

openbee creates both directories if they are missing before it starts the plugin. Keep session state in `state_dir`, so sessions continue after the plugin is reinstalled or installed from another source. Several calls can run at the same time, one process each; write files atomically.

### Using an engine

The engine for a run is the worker's own engine if it has one, otherwise the default engine. The default is decided on every run, from the engines that are enabled at that moment:

1. The default engine stored in the database (set in the web console), if that engine is enabled.
2. Otherwise `bee.engine.default` from `config.yaml`, if that engine is enabled.
3. Otherwise the enabled engine with the lowest id.

The stored value is never rewritten, so a user with only one engine installed does not have to change it. `bee.engines` in `config.yaml` is no longer used: a leftover section is ignored and logged as a warning. openbee starts with no engine installed; install one with `openbee plugin install` or from the web console.

A run fails with the usual failure notice when no engine can be used:

| Error | When |
|---|---|
| `engine "<id>" is not installed or enabled` | The worker's own engine is not installed or is disabled. Other workers keep running. |
| `no engine is installed or enabled` | The run needs the default engine and no engine is enabled. |
| `<cli> not found on PATH` | By convention, the message a plugin reports when its CLI cannot be found, for example `claude not found on PATH`. It shows up as the `failed` error and as the run error. |

`GET /api/config` lists the enabled engines on every request: `enabled_engines` as an array of ids, and `engines` as `[{"id": "...", "name": {"en": "..."}}]`. An empty list means no engine is enabled.

## Process rules

- **One request on stdin, one response on stdout.** openbee writes the request as a single line (UTF-8 JSON, ending in `\n`) and closes stdin. A plugin may read until the first newline or until EOF.
- **stdout carries the response only.** Every stdout line that is not a JSON object with a `result` or `error` member is logged as a warning (component `engine:<id>`) and ignored, so stray output from the plugin or its dependencies does not break the call. If several lines are valid responses, the last one wins. Keep the CLI's own stdout out of it: pipe it or write it to `log_path`.
- **stderr is for logs.** Each non-empty stderr line is copied to the openbee log (clipped to 4 KiB) with component `engine:<id>`. If the plugin exits without a response, the last stderr line becomes the error message.
- **Lines are limited to 16 MiB** on stdout and stderr; longer lines are dropped and logged. A response must therefore fit in 16 MiB.
- **The exit code does not discard a response.** A valid response counts even when the plugin then exits with a non-zero code. A plugin killed by a signal fails the call even if it had already written its response. A plugin that exits without a response fails the call with `engine plugin exited without a response: <last stderr line>`, or with the exit status when stderr was empty; a plugin killed by a signal reports the signal.
- **Child processes that keep stdout or stderr open** after the plugin exits delay the call by at most 3 seconds; openbee then continues with the response it has.
- **Keep the CLI in the plugin's process group.** On Linux and macOS openbee starts the plugin as the leader of a new process group. Spawn the CLI the default way: do not detach it, start a new session or double fork, or it escapes the kill below. openbee kills the whole group with `SIGKILL` (no chance to clean up) when the user stops a task, when a task times out or is cancelled, when `check` or `usage` times out, and after any failed call, to clear leftovers. On Windows openbee ends the plugin's process tree with `taskkill /T` when a task is stopped.
- **Working directory.** The plugin runs in its own directory; it sets the CLI's working directory from `work_dir`.
- **Environment.** For `run`, the environment is openbee's own (with its executable directory prepended to `PATH`), the user's Env settings, `OPENBEE_API_KEY`, and `NO_PROXY` and `no_proxy` extended with the host of the MCP URL when there is one. `check` and `usage` get openbee's environment without the user's Env settings and without `OPENBEE_API_KEY`. Pass the environment on to the CLI unchanged. `OPENBEE_API_KEY` is only ever in `run`'s environment; it is not in the request or in any argument.
- **Timeouts.** `check` 30 seconds, `usage` 60 seconds. `run` has no timeout of its own: it ends when the plugin does or when the task is stopped or times out (`bee.engine.timeout`).

## Messages

Both sides ignore unknown fields, so new optional fields can be added within version 1.

### Request

```json
{
  "protocol_version": 1,
  "method": "run",
  "openbee_version": "0.0.47",
  "engine_id": "claude",
  "language": "zh",
  "config": { "path": "" },
  "data_dir": "/home/u/.openbee/plugins/data/claude",
  "state_dir": "/home/u/.openbee/.claude",
  "params": {}
}
```

| Field | Rules |
|---|---|
| `protocol_version` | `1`. |
| `method` | `run`, `check` or `usage`. |
| `openbee_version` | The openbee version. |
| `engine_id` | The plugin's `id`. |
| `language` | The user's language, `en` or `zh`. |
| `config` | The plugin's settings with manifest defaults applied and validated, under the same rules as a channel plugin's `initialize.config`. Always an object; `secret` values arrive in plain text. |
| `data_dir`, `state_dir` | See [`data_dir` and `state_dir`](#data_dir-and-state_dir). |
| `params` | Depends on `method`; `{}` for `check`. |

### Response

A single line, either of:

```json
{ "result": { } }
{ "error": { "message": "claude not found on PATH" } }
```

`error.message` is shown to the user. The shape of `result` depends on the method.

### `run`

Runs one task. The call lasts as long as the task.

```json
"params": {
  "role": "bee",
  "work_dir": "/home/u/.openbee/bee",
  "prompt": "...",
  "session_id": "6f1c5a52-0000-4000-8000-000000000000",
  "resume": true,
  "extra_args": ["--model", "opus"],
  "log_path": "/home/u/.openbee/logs/2026-10-29/exec-123.log",
  "mcp": { "name": "openbee", "url": "http://127.0.0.1:12350/mcp", "api_key_env": "OPENBEE_API_KEY" }
}
```

| Field | Rules |
|---|---|
| `role` | `bee` or `worker`. |
| `work_dir` | The directory the CLI must run in. |
| `prompt` | The full prompt, passed through byte for byte. It can be several MB and contain newlines, quotes and emoji. |
| `session_id` | openbee's session id. The plugin keeps the mapping to its CLI's own session (for example a thread id) in `state_dir`. |
| `resume` | `true` when the session has run before and the plugin should continue the CLI conversation instead of starting one. |
| `extra_args` | The merged global and worker engine arguments; always an array, possibly empty. Append them after the plugin's own arguments. |
| `log_path` | A file in an existing directory. The plugin writes the CLI's output to it; the format is up to the plugin. |
| `mcp` | Omitted when openbee has no external address. Otherwise the plugin registers an MCP server called `name` at `url` with the CLI, in whatever way that CLI supports. `api_key_env` is the name of the environment variable that holds the API key; refer to the key by that name and never write its value into arguments or files. |

Result:

```json
{ "result": { "text": "final reply", "error": "" } }
```

| Response | openbee records |
|---|---|
| `text` set, `error` empty | The task succeeded and `text` is the final reply. |
| `error` non-empty | The task failed with that message. `text` may still carry the reply produced so far, and is kept. Use it for a non-zero CLI exit, `is_error` results, `stopReason=error` and the like. |
| `{"error": {"message": "..."}}` | The task failed with that message. |
| No response | The task failed (see [Process rules](#process-rules)). |

When a task is stopped or times out, openbee kills the process group (see [Process rules](#process-rules)); the plugin gets no chance to answer.

### `check`

Reports whether the CLI can be used. `params` is `{}`.

```json
{ "result": { "version": "2.1.314", "path": "/usr/local/bin/claude" } }
```

or `{"error": {"message": "claude not found on PATH"}}`. A plugin finds its CLI from the `path` setting when it has one, otherwise on `PATH`. The call times out after 30 seconds.

### `usage`

Reads token usage for a batch of sessions. openbee starts one `usage` process per engine in each token statistics round, and does not call it with an empty list.

```json
"params": { "session_ids": ["6f1c...", "a2b9..."] }
```

```json
{
  "result": {
    "usage": {
      "6f1c...": [
        { "model": "claude-opus", "input_tokens": 1, "output_tokens": 2, "cache_creation_tokens": 0, "cache_read_tokens": 0 }
      ]
    }
  }
}
```

Sessions that cannot be found are left out of `usage`. The call times out after 60 seconds.
