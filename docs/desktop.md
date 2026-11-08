# Desktop app

<sub><a href="desktop.zh.md">中文</a></sub>

A native OpenBee app for macOS, Windows and Linux, built with [MyGo](https://github.com/egoist/mygo). The standard build includes the OpenBee server, so it is all you need on one computer.

## Download

Install with a package manager, or download the file for your platform from [GitHub Releases](https://github.com/theopenbee/openbee/releases).

| Platform | Package manager | Release file |
| --- | --- | --- |
| macOS, Apple silicon | `brew install --cask theopenbee/tap/openbee-desktop` | `openbee-desktop-<version>-darwin-arm64.dmg` or `.zip` |
| macOS, Intel | same as above | `openbee-desktop-<version>-darwin-amd64.dmg` or `.zip` |
| Windows | `scoop bucket add theopenbee https://github.com/theopenbee/scoop-bucket`<br>`scoop install theopenbee/openbee-desktop` | `openbee-desktop-<version>-windows-<arch>-setup.exe` |
| Linux | — | `openbee-desktop-<version>-linux-<arch>.deb` or `.tar.gz` |

## First launch

Release builds are not code signed yet, so the system asks you to approve the app once:

- **macOS:** right-click the app and choose Open. On macOS 15 and later, use Open Anyway in System Settings → Privacy & Security. Or run `xattr -dr com.apple.quarantine /Applications/OpenBee.app`.
- **Windows:** when SmartScreen says "Windows protected your PC", click More info, then Run anyway.

Then create the admin account. Later launches sign in automatically.

## The built-in server

- It starts on `http://localhost:19876` with `~/.openbee/config.yaml`, the same files `openbee server` uses. The web console works at that address too.
- It runs while the app runs and stops when you quit. On macOS and Windows, closing the window keeps the app in the menu bar or tray. On Linux, closing the window quits.
- If an OpenBee server already answers on that port, the app uses it instead of starting its own.
- Logs go to `~/Library/Logs/OpenBee/server.log` on macOS, and to the `logs` folder of the app data directory elsewhere.

## Updates

The app checks GitHub Releases for a new version 10 seconds after launch and then once a day. When one is out, it shows the release notes and offers to install it, remind you later or skip that version. Tick "Automatically download and install updates in the future" to stop being asked. **Check for Updates…** checks right away: it is in the app menu on macOS and in the File menu on Windows and Linux.

An update downloads in the background and takes effect when the app restarts. With the built-in server, restarting the app restarts the server too.

The app only updates itself where it was installed from a release file or Homebrew. Scoop, the `.deb` package and the single-file `openbee-desktop-<version>-windows-<arch>.exe` are updated by downloading the new version.

## Use another server

To connect to a remote server, choose **Change Server** in the app menu.

If you never need the built-in server, Releases also carry a client-only build, `openbee-desktop-client-<version>-…`. Homebrew and Scoop always install the build with the server.

## Build from source

Install `jq` first. Go 1.27 is downloaded automatically.

```bash
make desktop              # current platform, with the server; output in desktop/build/
make desktop-all          # macOS, Windows and Linux, with the server
make desktop-client       # current platform, client only
make desktop-client-all   # macOS, Windows and Linux, client only
```

`make desktop` and `make desktop-all` build the web UI with pnpm first, because the server embeds it.
