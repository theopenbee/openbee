# Daemon Mode Design

**Date:** 2026-03-19
**Status:** Draft

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
2. Parent spawns a child: re-executes `os.Args[0]` with identical arguments, plus `OPENBEE_DAEMON=1` in the environment
3. Child process opens `~/.openbee/openbee.log` and redirects its stdout+stderr to that file
4. Child detaches from the controlling terminal (platform-specific, see below)
5. Parent writes child PID to `~/.openbee/openbee.pid`, prints `Daemon started (PID: <pid>)`, then exits 0
6. Child proceeds with the normal `server` startup (`BuildApp` + `Run`)

When the daemon shuts down (SIGTERM / SIGINT / error), it removes the PID file on exit.

### Cross-Platform Strategy

Platform-specific behaviour is isolated in build-tagged files. Shared logic (PID file R/W, env detection) lives in a single `daemon.go`.

#### Unix (`//go:build !windows` — macOS, Linux)

- Spawn child with `cmd.SysProcAttr{Setsid: true}` — creates a new session, detaches from terminal
- Stop: `syscall.Kill(pid, syscall.SIGTERM)`, then poll for process exit (max 15 s), then `SIGKILL`
- Process liveness check (status): `syscall.Kill(pid, 0)` — error means process is gone

#### Windows (`//go:build windows`)

- Spawn child with `syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008}` (`DETACHED_PROCESS`) — detaches from console
- Stop: send `CTRL_BREAK_EVENT` via `GenerateConsoleCtrlEvent` for graceful shutdown; fall back to `Process.Kill()` after 15 s timeout
- Process liveness check: `windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))` — error means process is gone

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
- Format: ASCII decimal PID followed by newline
- Written by parent after child starts; removed by daemon on clean exit
- `stop` / `restart` / `status` treat a missing PID file as "not running"
- A stale PID file (process no longer exists) is detected and cleaned up automatically

### Log File

- Path: `~/.openbee/openbee.log`
- Mode: append (`O_APPEND | O_CREATE | O_WRONLY`, `0644`)
- The daemon's `os.Stdout` and `os.Stderr` are both redirected to this file before `BuildApp` runs
- Existing zap JSON logging is unaffected — it writes to stdout, which is now the log file

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
2. Waits for old process to fully exit
3. Runs daemon spawn sequence with the given config path

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
| Daemon already running (`stop` called again) | Reports already stopped, exit 0 |
| `start` while daemon already running | Print error "already running (PID: X)", exit 1 |
| Log file not writable | Print error to stderr, exit 1 |
| PID file directory does not exist | Create `~/.openbee/` automatically |
| Stale PID file | Detect via liveness check, remove stale file, treat as "not running" |
| Child exits immediately after spawn | Parent detects via a brief readiness wait (100 ms poll, up to 2 s); reports error if child is gone |

## Dependencies

- No new external dependencies
- Windows: uses `golang.org/x/sys/windows` (already an indirect dependency in most Go projects; add if not present)

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
