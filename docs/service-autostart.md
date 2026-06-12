# Autostart (`openbee service`)

The `openbee service` command group registers `openbee server` to start automatically when the current user logs in. All three operating systems use **user-level** mechanisms; no sudo or administrator rights are required.

## Quick start

```bash
openbee service install    # register and start
openbee service status
openbee service stop
openbee service start
openbee service uninstall  # remove registration
```

Pass `--config <path>` on `install` to point at a non-default config file. Pass `--no-start` to register without starting immediately. Pass `--force` to overwrite an existing registration.

## Platform details

### macOS

A LaunchAgent plist is written to `~/Library/LaunchAgents/com.theopenbee.openbee.plist` and registered via `launchctl bootstrap`. The process is auto-restarted on crash (`KeepAlive=true`, `ThrottleInterval=10`).

### Linux

A systemd user unit is written to `~/.config/systemd/user/openbee.service` and enabled via `systemctl --user enable --now`. The process is auto-restarted on failure (`Restart=on-failure`, `RestartSec=10`).

**Prerequisites:**
- A working systemd user session. Distributions without systemd (Alpine, certain WSL setups, minimal containers) are unsupported — use `openbee server` directly or write your own init script.
- If you want the service to survive logout, enable **user lingering**: `sudo loginctl enable-linger $USER`.

### Windows

A Scheduled Task named `OpenBee` is created with a logon trigger for the current user. Failure retry is set to 3 attempts spaced 1 minute apart. The task runs hidden — open `taskschd.msc` and navigate to **Task Scheduler Library > OpenBee** to inspect.

## Logs

All platforms redirect stdout/stderr to `~/.openbee/daemon.log` (same file as `openbee server`).

## Coexisting with `openbee server`

Once `service install` completes, the service manager owns the lifecycle. Running `openbee server` manually will refuse to start because the PID file is already taken by the auto-started instance. Use `openbee service stop` first.
