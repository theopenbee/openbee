# 🐝 OpenBee — 数字员工（OPC）解决方案

[English](README.md)

**OpenBee** 是运行在私人电脑/服务器上的数字员工解决方案。你可以通过飞书/钉钉/企业微信与它沟通，指挥它创建员工，给它发布任务等等操作，请充分发挥你的想象力！

## 安装

**方式一：npm 安装（推荐）**

```bash
npm install -g @theopenbee/cli
```

安装时会自动下载当前平台对应的二进制文件，支持 Linux / macOS / Windows（amd64 & arm64）。

**方式二：手动下载二进制**

前往 [GitHub Releases](https://github.com/theopenbee/openbee/releases) 页面，下载对应平台的压缩包，解压后将 `openbee` 可执行文件放入 PATH 即可。

## 快速开始

**第一步：交互式生成配置文件**

```bash
openbee config
```

向导将引导你完成以下配置：

- 服务端口 / 主机
- Claude 可执行文件路径
- MCP API Key（可随机生成）
- 启用的 IM 平台（飞书 / 钉钉 / 企微）及对应凭证
- 高级选项（可跳过，使用默认值）

配置文件默认写入当前目录的 `config.yaml`，如需指定路径，使用 `-o` 参数：

```bash
openbee config -o /path/to/config.yaml
```

**第二步：启动服务**

```bash
openbee start
```

**第三步：开始使用**

- 打开 Web 控制台（默认 [http://localhost:8080](http://localhost:8080)）管理 Worker 和查看任务状态
- 在已配置的 IM 平台（飞书 / 钉钉 / 企微）中直接发送消息与 OpenBee 交互

## 工作原理

OpenBee 由四个核心层构成：

**1. 平台接入层**
连接飞书、钉钉、企微等 IM 平台，实时接收用户消息，并将结果回复到对话中。

**2. 消息摄取 & 任务调度**
对输入消息进行去抖、聚合后，路由给对应的 Worker 执行；同时支持定时任务，可按计划自动触发。

**3. Worker（数字员工）**
每个 Worker 是一个由 Claude AI 驱动的智能体，具备持久记忆、工具调用（MCP）和多步任务规划能力。Worker 可以被创建、配置、指派任务，像真实员工一样独立完成工作。

**4. Web 控制台**
提供 Worker 管理、任务执行历史、实时日志等可视化界面，方便监控和调试。

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=theopenbee/openbee&type=Date)](https://star-history.com/#theopenbee/openbee&Date)

## Community

- **问题反馈 / 功能建议**：[GitHub Issues](https://github.com/theopenbee/openbee/issues)
- **贡献代码**：请阅读 [贡献指南](CONTRIBUTING.md)，提交前需同意 [贡献者许可协议（CLA）](CLA.zh.md)
