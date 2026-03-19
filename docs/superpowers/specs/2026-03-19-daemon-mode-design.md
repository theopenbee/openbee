# Daemon Mode Design

**Date:** 2026-03-19
**Status:** Reviewed

## Overview

Add daemon (background) run mode to the `openbee` CLI, with companion `stop`, `restart`, and `status` commands. The daemon runs the existing server in the background, detached from the terminal, writing logs to a file and recording its PID for lifecycle management.

## Requirements

- `openbee server --daemon` / `-d` starts the server as a background process
- `openbee stop` gracefully stops a running daemon
- `openbee restart [-c config]` stops a running daemon and starts a new one
- `openbee status` reports whether a daemon is running, its PID, and how long it has been up
- Cross-platform: macOS, Linux, and Windows

## Confirmed Design Decisions

| Decision | Choice |
|---|---|
| Start command | `openbee server --daemon` / `-d` flag on existing `server` command |
| Log file | `~/.openbee/openbee.log` (fixed path, stdout+stderr redirected) |
| PID file | `~/.openbee/openbee.pid` |
| Daemon mechanism | Re-exec (parent re-launches itself as child, then exits) |

## Architecture

### Daemon Lifecycle (Re-exec Model)

When `openbee server --daemon` is invoked:

1. **Parent process** detects `--daemon` flag and absence of `OPENBEE_DAEMON=1` env var
2. Parent checks for a live PID file; if found, prints "already running (PID: X)" and exits 1
3. Parent resolves its own executable path via `os.Executable()` + `filepath.EvalSymlinks()` (never `os.Args[0]`)
4. Parent spawns a child: re-executes the resolved binary with identical arguments (`server [-c ...]`), plus `OPENBEE_DAEMON=1` in the environment, with platform-specific detach attributes (see Cross-Platform Strategy)
5. Parent polls for child liveness for up to 2 s (100 ms intervals); if the child exits during this window, reports the error and exits 1
6. Parent writes child PID + Unix start timestamp (seconds) to `~/.openbee/openbee.pid`, prints `Daemon started (PID: <pid>)`, then exits 0
7. **Child process** (running with `OPENBEE_DAEMON=1`): opens `~/.openbee/openbee.log` in append mode and redirects both OS-level file descriptors 1 and 2 to that file **before** calling `logger.Init` — ensuring zap's `os.Stderr` sink captures the log file
8. Child proceeds with the normal `server` startup (`logger.Init` → `BuildApp` → `Run`)

When the daemon shuts down (SIGTERM / SIGINT / error), it removes the PID file on exit.

### Cross-Platform Strategy

Platform-specific behaviour is isolated in build-tagged files. Shared logic (PID file R/W, env detection) lives in a single `daemon.go`.

#### Unix (`//go:build !windows` — macOS, Linux)

- Spawn child with `cmd.SysProcAttr{Setsid: true}` — creates a new session, detaches from terminal
- Stop: `syscall.Kill(pid, syscall.SIGTERM)`, then poll for process exit (max 15 s), then `SIGKILL`
- Process liveness check (status): `syscall.Kill(pid, 0)` — error means process is gone

#### Windows (`//go:build windows`)

- Spawn child with `syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}` — the struct type comes from the standard `syscall` package; the constants come from `golang.org/x/sys/windows`. `CREATE_NO_WINDOW` prevents a new console window from appearing; `CREATE_NEW_PROCESS_GROUP` is required for `GenerateConsoleCtrlEvent` targeting. Note: `DETACHED_PROCESS` is intentionally avoided — it interacts poorly with `CREATE_NEW_PROCESS_GROUP` and does not reliably suppress console windows.
- Stop: send `CTRL_BREAK_EVENT` via `windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(pid))` for graceful shutdown (app.go already handles `syscall.SIGTERM` and `syscall.SIGINT`); fall back to `Process.Kill()` after 15 s timeout
- Process liveness check: `windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))` — error means process is gone
- `golang.org/x/sys/windows` must be imported directly in `daemon_windows.go`; run `go mod tidy` after adding to promote it from indirect to direct dependency

### File Layout

```
cmd/openbee/
  daemon.go          # shared: PID file R/W, env detection, daemonize() entry point
  daemon_unix.go     # //go:build !windows  — spawnDaemon(), isAlive(), stopProcess()
  daemon_windows.go  # //go:build windows   — spawnDaemon(), isAlive(), stopProcess()
  server.go          # add --daemon flag; detect OPENBEE_DAEMON env to skip re-exec
  stop.go            # stopCmd
  restart.go         # restartCmd
  status.go          # statusCmd
```

### PID File

- Path: `~/.openbee/openbee.pid`
- Format: two lines — line 1: ASCII decimal PID; line 2: Unix start timestamp (seconds since epoch)
- Written by parent **after** the readiness poll confirms the child is alive (not before)
- Removed by the daemon on clean shutdown (SIGTERM/SIGINT); abnormal termination (SIGKILL, panic, OOM) leaves a stale file — this is expected and handled by the stale-detection logic in all lifecycle commands
- Also removed by any command that detects a stale entry
- `stop` / `restart` / `status` treat a missing PID file as "not running"
- A stale PID file (process no longer exists) is detected via liveness check, cleaned up, and treated as "not running"

### Log File

- Path: `~/.openbee/openbee.log`
- Mode: append (`O_APPEND | O_CREATE | O_WRONLY`, `0644`)
- The daemon child redirects OS file descriptors 1 and 2 to this file **before** `logger.Init` is called, so that zap's `os.Stderr` sink writes to the log file from the moment it is initialised
- Log rotation is out of scope — users should manage rotation externally (logrotate, etc.)

### Uptime Calculation

`openbee status` computes uptime as `now − start_timestamp`, where `start_timestamp` is the Unix epoch value stored on line 2 of the PID file. No OS-level process introspection (`/proc`, `ps`) is required.

## Command Reference

### `openbee server [-c config.yaml] [--daemon | -d]`

Existing command extended with one new flag. Behaviour unchanged when `--daemon` is absent.

| Flag | Description |
|---|---|
| `-c`, `--config` | Path to config file (default: `config.yaml`) |
| `-d`, `--daemon` | Start as background daemon |

Exit codes when `--daemon` is used: `0` on successful spawn, `1` on error.

### `openbee stop`

Reads `~/.openbee/openbee.pid`, sends stop signal, waits up to 15 s for the process to exit, then force-kills if still alive. Prints status to stdout. Exit `0` if stopped (or was not running), `1` on error.

### `openbee restart [-c config.yaml]`

1. Runs stop sequence (tolerates "not running")
2. Waits for old process to fully exit (stop already handles the 15 s wait)
3. Runs daemon spawn sequence: re-execs binary as `server --daemon -c <config>`, where `<config>` is the value passed to `restart -c` (defaults to `config.yaml` if not provided)

The config path is always provided explicitly to the new child; it is not inferred from the old process.

### `openbee status`

Reads PID file, checks liveness, prints one of:

```
● openbee is running   (PID: 12345, uptime: 3h 42m)
○ openbee is not running
```

Exit code `0` = running, `1` = not running or error (consistent with systemd convention).

## Error Handling

| Scenario | Behaviour |
|---|---|
| Daemon already stopped (`stop` called again) | Reports already stopped, exit 0 |
| `start` while daemon already running | Print error "already running (PID: X)", exit 1 |
| Log file not writable | Print error to stderr, exit 1 |
| PID file directory does not exist | Create `~/.openbee/` automatically |
| Stale PID file | Detect via liveness check, remove stale file, treat as "not running" |
| Child exits immediately after spawn | Parent polls for child liveness (100 ms intervals, up to 2 s); if child has exited during this window, parent reports failure and exits 1. Note: a healthy child that takes >2 s to start is not flagged — the poll only catches immediate exits, not slow startups |

## Dependencies

- No new external dependencies on Unix
- Windows: `golang.org/x/sys/windows` — currently an indirect dependency; importing it directly in `daemon_windows.go` and running `go mod tidy` will promote it to a direct dependency in `go.mod`

## Files Changed

| File | Change |
|---|---|
| `cmd/openbee/server.go` | Add `--daemon` / `-d` flag; skip re-exec when `OPENBEE_DAEMON=1` |
| `cmd/openbee/daemon.go` | New — shared PID file helpers, `daemonize()` entry point |
| `cmd/openbee/daemon_unix.go` | New — Unix spawn/signal/liveness |
| `cmd/openbee/daemon_windows.go` | New — Windows spawn/signal/liveness |
| `cmd/openbee/stop.go` | New — `stopCmd` |
| `cmd/openbee/restart.go` | New — `restartCmd` |
| `cmd/openbee/status.go` | New — `statusCmd` |

## Out of Scope

- Windows Service (`svc.Run`) integration — not required; background process is sufficient
- Log rotation — left to the user (logrotate, etc.)
- Multiple concurrent daemon instances — not supported; enforced via PID file check
