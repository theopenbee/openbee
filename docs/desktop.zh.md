# 桌面版

<sub><a href="desktop.md">English</a></sub>

基于 [MyGo](https://github.com/egoist/mygo) 构建的 OpenBee 原生应用，支持 macOS、Windows 和 Linux。标准版内置 OpenBee 服务端，一台电脑上装它就够了。

## 下载

用包管理器安装，或从 [GitHub Releases](https://github.com/theopenbee/openbee/releases) 下载对应平台的文件。

| 平台 | 包管理器 | 发布文件 |
| --- | --- | --- |
| macOS，M 系列芯片 | `brew install --cask theopenbee/tap/openbee-desktop` | `openbee-desktop-<version>-darwin-arm64.dmg` 或 `.zip` |
| macOS，Intel 芯片 | 同上 | `openbee-desktop-<version>-darwin-amd64.dmg` 或 `.zip` |
| Windows | `scoop bucket add theopenbee https://github.com/theopenbee/scoop-bucket`<br>`scoop install theopenbee/openbee-desktop` | `openbee-desktop-<version>-windows-<arch>-setup.exe` |
| Linux | — | `openbee-desktop-<version>-linux-<arch>.deb` 或 `.tar.gz` |

## 首次打开

发布包暂未做代码签名，系统会要求你手动放行一次：

- **macOS：** 右键点击应用，选择“打开”。macOS 15 及以上请在“系统设置 → 隐私与安全性”中点击“仍要打开”。也可以运行 `xattr -dr com.apple.quarantine /Applications/OpenBee.app`。
- **Windows：** SmartScreen 提示“Windows 已保护你的电脑”时，点击“更多信息”，再点击“仍要运行”。

然后创建管理员账号，之后打开会自动登录。

## 内置服务端

- 它使用 `~/.openbee/config.yaml` 在 `http://localhost:19876` 启动，与 `openbee server` 用的是同一套文件。这个地址同样可以打开 Web 控制台。
- 应用运行时它一直运行，退出应用后停止。在 macOS 和 Windows 上关闭窗口，应用仍留在菜单栏或托盘；在 Linux 上关闭窗口即退出。
- 如果该端口上已有 OpenBee 服务在运行，应用会直接使用它，不再自己启动。
- 日志在 macOS 上位于 `~/Library/Logs/OpenBee/server.log`，其他平台位于应用数据目录的 `logs` 文件夹。

## 连接其他服务端

要连接远程服务端，在应用菜单中选择 **更换服务器**。

如果完全用不到内置服务端，发布页还提供纯客户端版 `openbee-desktop-client-<version>-…`。Homebrew 和 Scoop 安装的始终是内置服务端的版本。

## 从源码构建

先安装 `jq`，Go 1.27 会自动下载。

```bash
make desktop              # 当前平台，内置服务端，产物在 desktop/build/
make desktop-all          # macOS、Windows 和 Linux，内置服务端
make desktop-client       # 当前平台，纯客户端
make desktop-client-all   # macOS、Windows 和 Linux，纯客户端
```

`make desktop` 和 `make desktop-all` 会先用 pnpm 构建 Web 界面，因为服务端内嵌了它。
