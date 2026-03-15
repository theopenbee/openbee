# WebSocket 心跳与断线重连 - 技术实现报告

> 源项目：dingtalk-openclaw-connector (TypeScript/Node.js)
> 目标：为 Golang 重新实现提供完整的技术规格

---

## 1. 架构概览

```
┌─────────────────────────────────────────────────────────────┐
│                     应用层 (plugin.ts)                       │
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │  心跳定时器   │  │  重连控制器   │  │  消息去重缓存    │  │
│  │  30s interval │  │  disconnect  │  │  Map<id,ts>     │  │
│  │  90s timeout  │  │  → connect   │  │  TTL=5min       │  │
│  └──────┬───────┘  └──────┬───────┘  └──────────────────┘  │
│         │                 │                                  │
│  ┌──────▼─────────────────▼──────────────────────────────┐  │
│  │              DWClient (dingtalk-stream SDK)            │  │
│  │  - autoReconnect: true (SDK 内建被动重连)              │  │
│  │  - keepAlive: false (禁用 SDK 的 8 秒激进心跳)         │  │
│  └──────────────────────┬────────────────────────────────┘  │
│                         │                                    │
│  ┌──────────────────────▼────────────────────────────────┐  │
│  │              WebSocket 连接 (ws)                       │  │
│  │  - ping/pong 帧 (RFC 6455 控制帧)                     │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
              ┌───────────────────────┐
              │  钉钉 Stream 服务端    │
              └───────────────────────┘
```

**双层保护机制：**
- **SDK 层**：`autoReconnect: true`，SDK 检测到连接断开时自动重连（被动）
- **应用层**：自定义 ping/pong 心跳检测，主动发现静默断连并触发重连（主动）

禁用 SDK 的 `keepAlive` 是因为其内建心跳仅 8 秒超时，过于激进，会在正常空闲期间误判连接断开。

---

## 2. 心跳机制详细设计

### 2.1 核心参数

| 参数 | 值 | 说明 |
|------|-----|------|
| `HEARTBEAT_INTERVAL` | 30 秒 | 每次心跳检测/发送 PING 的间隔 |
| `HEARTBEAT_TIMEOUT` | 90 秒 | 无 PONG 响应的最大容忍时间（≈3 个心跳周期） |
| 重连重试延迟 | 5 秒 | 首次重连失败后的固定重试间隔 |
| 消息去重 TTL | 5 分钟 | 消息 ID 缓存过期时间 |

### 2.2 状态变量

```go
// Golang 对应结构体字段
type HeartbeatManager struct {
    lastPongTime  time.Time    // 最后一次收到 PONG 的时间
    pendingPingID string       // 当前等待响应的 PING ID，空字符串表示无等待
    stopped       bool         // 连接是否已停止
    mu            sync.Mutex   // 保护并发访问
}
```

### 2.3 心跳流程 (每 30 秒执行一次)

```
heartbeatTick:
    ├── if stopped → 停止定时器，return
    │
    ├── elapsed = now - lastPongTime
    │
    ├── if elapsed > 90s (HEARTBEAT_TIMEOUT)
    │   ├── 日志: "心跳超时，触发重连"
    │   └── 执行重连流程 (见第3节)
    │   └── return
    │
    ├── if pendingPingID != "" (有 PING 等待中)
    │   ├── 日志: "等待 PONG 响应中"
    │   └── return (不发送新 PING)
    │
    └── 发送 PING
        ├── pingID = "ping_" + timestamp
        ├── pendingPingID = pingID
        ├── socket.ping({type:"PING", id:pingID, timestamp:now})
        └── if 发送失败 → 仅记录错误，不做特殊处理（等下一轮超时检测）
```

### 2.4 PONG 响应处理

```
onPong:
    ├── lastPongTime = now
    ├── pendingPingID = ""  (清除等待标记)
    └── 日志: "收到 PONG 响应"
```

### 2.5 PING 帧格式

使用 WebSocket 协议层的 ping 控制帧（RFC 6455），payload 为 JSON：

```json
{
  "type": "PING",
  "id": "ping_1710489600000",
  "timestamp": 1710489600000
}
```

> **Golang 实现注意**：`gorilla/websocket` 使用 `conn.WriteControl(websocket.PingMessage, payload, deadline)` 发送 ping 帧，并通过 `conn.SetPongHandler()` 监听 pong。

---

## 3. 断线重连机制详细设计

### 3.1 重连触发条件

唯一触发条件：**心跳超时** — 90 秒内未收到任何 PONG 响应。

### 3.2 重连流程

```
reconnect:
    ├── 第一次尝试 (立即执行)
    │   ├── Step 1: client.disconnect()  // 先断开旧连接
    │   ├── Step 2: client.connect()     // 重新建立连接
    │   ├── Step 3: 重置状态
    │   │   ├── lastPongTime = now
    │   │   └── pendingPingID = ""
    │   └── 日志: "重连成功"
    │
    └── if 第一次失败
        ├── 日志: "重连失败，5秒后重试"
        └── 等待 5 秒后第二次尝试
            ├── client.connect()
            ├── 重置状态 (同上)
            └── if 仍然失败
                └── 仅记录错误，等待下一个心跳周期再次触发
```

### 3.3 重连策略特点

| 特性 | 当前实现 | Golang 建议改进 |
|------|---------|----------------|
| 退避策略 | 固定 5 秒 | 可改为指数退避 (1s → 2s → 4s → 8s → 16s → 30s max) |
| 最大重试次数 | 无限制（每个心跳周期都会重试） | 建议加上限，超过后通知上层 |
| 重连前断开 | 是 (disconnect → connect) | 必须保持，防止连接泄漏 |
| 状态重置 | 重置 lastPongTime 和 pendingPingID | 必须保持，防止立即再次触发 |
| 并发保护 | 无（JS 单线程） | **Golang 必须加 mutex** |

### 3.4 状态机

```
            ┌─────────────┐
            │  CONNECTED  │◄──────────────────────┐
            └──────┬──────┘                       │
                   │                              │
          心跳超时(90s)                    重连成功
                   │                              │
            ┌──────▼──────┐                       │
            │ RECONNECTING│───────────────────────┘
            └──────┬──────┘
                   │
             重连失败
                   │
            ┌──────▼──────┐
            │  RETRY_WAIT │──── 5秒后 ────► RECONNECTING
            └──────┬──────┘
                   │
             重试也失败
                   │
            ┌──────▼──────┐
            │   DEGRADED  │──── 下一个心跳周期 ──► RECONNECTING
            └─────────────┘
```

---

## 4. 连接生命周期管理

### 4.1 启动流程

```
startup:
    1. 创建 DWClient (clientId, clientSecret, autoReconnect=true, keepAlive=false)
    2. 注册消息回调监听器 (TOPIC_ROBOT)
    3. client.connect()  // 建立 WebSocket 连接
    4. 初始化状态变量 (lastPongTime=now, pendingPingID="", stopped=false)
    5. 注册 PONG 监听器 (socket.on('pong', handler))
    6. 启动心跳定时器 (setInterval, 30s)
    7. 返回 pending Promise，等待 abortSignal
```

### 4.2 停止流程 (doStop)

```go
// Golang 伪代码
func (m *ConnectionManager) Stop(reason string) {
    m.mu.Lock()
    defer m.mu.Unlock()

    if m.stopped {
        return  // 防止重复停止
    }
    m.stopped = true

    // 1. 停止心跳定时器
    m.heartbeatTicker.Stop()

    // 2. 关闭 WebSocket 连接
    if err := m.client.Disconnect(); err != nil {
        log.Warn("断开连接时出错:", err)
    }

    // 3. 记录活动状态
    m.recordActivity("stop")
}
```

**关键设计点：**
- 使用 `stopped` 布尔标志防止重复停止
- 统一的 `doStop()` 函数处理所有停止场景（abort signal / 手动停止）
- 先清理定时器，再断开连接

### 4.3 健康检查

```go
func (m *ConnectionManager) IsHealthy() bool {
    return !m.stopped
}
```

---

## 5. 消息回调与去重机制

### 5.1 回调处理流程

```
onMessage(messageId, data):
    ├── Step 1: 立即确认 (socketCallBackResponse)
    │   └── 必须在 60 秒内响应，否则钉钉服务器重发
    │
    ├── Step 2: 去重检查
    │   ├── if processedMessages.has(messageId) → 跳过
    │   └── else → markMessageProcessed(messageId)
    │
    └── Step 3: 异步处理消息
        └── 解析 JSON → handleDingTalkMessage()
            └── 处理异常仅记录，不影响回调确认
```

### 5.2 消息去重实现

```go
// Golang 实现建议
type MessageDedup struct {
    cache map[string]time.Time  // messageId → 处理时间
    ttl   time.Duration         // 5 分钟
    mu    sync.RWMutex
}

func (d *MessageDedup) IsProcessed(msgID string) bool {
    d.mu.RLock()
    defer d.mu.RUnlock()
    _, exists := d.cache[msgID]
    return exists
}

func (d *MessageDedup) Mark(msgID string) {
    d.mu.Lock()
    defer d.mu.Unlock()
    d.cache[msgID] = time.Now()
}

// 定期清理过期条目（建议用独立 goroutine + ticker）
func (d *MessageDedup) Cleanup() {
    d.mu.Lock()
    defer d.mu.Unlock()
    now := time.Now()
    for id, ts := range d.cache {
        if now.Sub(ts) > d.ttl {
            delete(d.cache, id)
        }
    }
}
```

---

## 6. Golang 实现建议架构

### 6.1 推荐包结构

```
pkg/
├── websocket/
│   ├── client.go          // WebSocket 连接管理
│   ├── heartbeat.go       // 心跳检测器
│   ├── reconnect.go       // 重连控制器
│   └── dedup.go           // 消息去重
├── dingtalk/
│   ├── stream.go          // 钉钉 Stream 协议实现
│   └── message.go         // 消息处理
└── config/
    └── config.go          // 配置管理
```

### 6.2 核心结构体设计

```go
type StreamClient struct {
    // 连接配置
    clientID     string
    clientSecret string

    // WebSocket 连接
    conn         *websocket.Conn
    connMu       sync.Mutex

    // 心跳管理
    heartbeat    *HeartbeatManager

    // 消息去重
    dedup        *MessageDedup

    // 生命周期控制
    ctx          context.Context
    cancel       context.CancelFunc
    stopped      atomic.Bool

    // 日志
    logger       *slog.Logger
}

type HeartbeatManager struct {
    interval     time.Duration   // 30s
    timeout      time.Duration   // 90s
    lastPongTime atomic.Value    // time.Time
    pendingPing  atomic.Value    // string

    ticker       *time.Ticker
    onTimeout    func()          // 超时回调 → 触发重连
}
```

### 6.3 Golang 关键差异点

| 方面 | TypeScript 原实现 | Golang 注意事项 |
|------|-------------------|----------------|
| 并发模型 | 单线程事件循环，无竞态 | **必须用 mutex/atomic 保护共享状态** |
| 定时器 | `setInterval` / `setTimeout` | `time.Ticker` / `time.AfterFunc` |
| WebSocket ping | `ws.ping(payload)` | `conn.WriteControl(PingMessage, payload, deadline)` |
| WebSocket pong | `ws.on('pong', handler)` | `conn.SetPongHandler(handler)` |
| 异步处理 | `async/await` + Promise | `goroutine` + `channel` |
| 停止信号 | `abortSignal` + `addEventListener` | `context.Context` + `ctx.Done()` |
| 连接生命周期 | pending Promise 保持存活 | 主 goroutine `select` 阻塞等待 ctx 取消 |
| 重连时锁定 | 不需要（单线程） | **必须在重连期间锁定连接，防止并发读写** |

### 6.4 心跳 Goroutine 伪代码

```go
func (c *StreamClient) heartbeatLoop() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-c.ctx.Done():
            return
        case <-ticker.C:
            if c.stopped.Load() {
                return
            }

            lastPong := c.heartbeat.lastPongTime.Load().(time.Time)
            elapsed := time.Since(lastPong)

            // 超时检测
            if elapsed > 90*time.Second {
                c.logger.Warn("心跳超时，触发重连", "elapsed", elapsed)
                c.reconnect()
                continue
            }

            // 有等待中的 PING，跳过
            if pending := c.heartbeat.pendingPing.Load().(string); pending != "" {
                continue
            }

            // 发送 PING
            pingID := fmt.Sprintf("ping_%d", time.Now().UnixMilli())
            c.heartbeat.pendingPing.Store(pingID)

            payload, _ := json.Marshal(map[string]interface{}{
                "type":      "PING",
                "id":        pingID,
                "timestamp": time.Now().UnixMilli(),
            })

            c.connMu.Lock()
            err := c.conn.WriteControl(
                websocket.PingMessage,
                payload,
                time.Now().Add(5*time.Second),
            )
            c.connMu.Unlock()

            if err != nil {
                c.logger.Error("发送 PING 失败", "error", err)
            }
        }
    }
}
```

### 6.5 重连 Goroutine 伪代码

```go
func (c *StreamClient) reconnect() {
    c.connMu.Lock()
    defer c.connMu.Unlock()

    // 第一次尝试
    if err := c.disconnect(); err != nil {
        c.logger.Warn("断开旧连接失败", "error", err)
    }

    if err := c.connect(); err != nil {
        c.logger.Error("重连失败", "error", err)

        // 5秒后重试一次
        time.AfterFunc(5*time.Second, func() {
            c.connMu.Lock()
            defer c.connMu.Unlock()
            if err := c.connect(); err != nil {
                c.logger.Error("重试重连失败", "error", err)
                return
            }
            c.resetHeartbeatState()
            c.logger.Info("重试重连成功")
        })
        return
    }

    c.resetHeartbeatState()
    c.logger.Info("重连成功")
}

func (c *StreamClient) resetHeartbeatState() {
    c.heartbeat.lastPongTime.Store(time.Now())
    c.heartbeat.pendingPing.Store("")
}
```

---

## 7. 时序图

```
Client                          DingTalk Server
  │                                    │
  │──── connect() ────────────────────►│
  │◄─── WebSocket established ────────│
  │                                    │
  │    [每30秒心跳循环]                 │
  │──── PING (WebSocket控制帧) ──────►│
  │◄─── PONG ─────────────────────────│
  │     lastPongTime = now             │
  │     pendingPingID = ""             │
  │                                    │
  │──── PING ─────────────────────────►│
  │◄─── PONG ─────────────────────────│
  │                                    │
  │──── PING ─────────────────────────►│
  │         (网络中断, 无PONG)          │
  │     ...30s...                      │
  │──── PING ─────────────────────────►│ (pendingPing≠""，跳过)
  │     ...30s...                      │
  │──── 检测: elapsed > 90s            │
  │                                    │
  │──── disconnect() ─────────────────►│
  │──── connect() ────────────────────►│ (新连接)
  │◄─── WebSocket established ────────│
  │     lastPongTime = now             │
  │     pendingPingID = ""             │
  │                                    │
  │     [恢复心跳循环]                  │
  │──── PING ─────────────────────────►│
  │◄─── PONG ─────────────────────────│
```

---

## 8. 总结：Golang 实现清单

- [ ] WebSocket 连接管理（gorilla/websocket 或 nhooyr/websocket）
- [ ] 心跳定时器：30 秒 `time.Ticker`，发送 WebSocket PING 控制帧
- [ ] PONG 处理器：`conn.SetPongHandler()` 更新 `lastPongTime`
- [ ] 超时检测：90 秒无 PONG → 触发重连
- [ ] 重连逻辑：disconnect → connect → 重置状态，失败 5 秒后重试
- [ ] 并发安全：所有共享状态用 `sync.Mutex` 或 `atomic` 保护
- [ ] 生命周期：`context.Context` 控制优雅关闭
- [ ] 消息去重：`map[string]time.Time` + 5 分钟 TTL + 定期清理 goroutine
- [ ] 消息立即确认：收到回调后先 ACK（60 秒内），再异步处理
- [ ] 统一停止函数：清理 ticker → 关闭连接 → 标记 stopped
