# Local Messages Pagination Design

**Date:** 2026-04-08
**Topic:** `/api/local/messages` 大数据量分页优化
**Status:** Approved

---

## Background

当前 `GET /api/local/messages` 接口无任何分页或限制逻辑：

- `MessageStore.ListBySessionKey` 无 LIMIT 子句，全量返回所有非 merged 入站消息
- `OutboundMessageStore.ListBySessionKey` 被以 `limit=0`（即无限制）调用，注释中明确警告"use with caution on long-lived sessions"
- Handler 将两者全部加载到内存、合并、排序后一次性返回 JSON

随着本地 session 长期使用，消息量增长到几百至几千条时，将导致响应变慢、内存占用升高、前端渲染卡顿。

---

## Scope

中等使用场景（单 session 消息量预计达到几百到几千条），采用游标（keyset）分页方案。

---

## API Contract

### 请求

```
GET /api/local/messages?before=<unix_ms>&limit=<n>
```

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| before | int64 (unix ms) | 无（不传 = 最新） | 返回时间戳严格小于此值的消息 |
| limit | int | 50 | 每页条数，服务端强制最大 100 |

### 响应

响应格式从裸数组改为对象：

```json
{
  "messages": [
    { "role": "user", "content": "...", "ts": 1712345678000 },
    { "role": "bee",  "content": "...", "ts": 1712345679000 }
  ],
  "has_more": true
}
```

### 行为

- 不传 `before`：返回最新的 limit 条，按时间升序排列
- 传入 `before`：返回时间戳 < before 的最近 limit 条，按时间升序排列
- `has_more=true` 表示还有更早的消息可以继续加载

---

## Store Layer

### MessageStore.ListBySessionKey

```go
// Before
func (s *MessageStore) ListBySessionKey(ctx context.Context, sessionKey string) ([]InboundMessage, error)

// After
func (s *MessageStore) ListBySessionKey(ctx context.Context, sessionKey string, before int64, limit int) ([]InboundMessage, error)
```

SQL 逻辑：

```sql
-- before=0（不传）：取最新 limit+1 条
SELECT id, content, received_at FROM bee_platform_messages
WHERE session_key = ? AND status != 'merged'
ORDER BY received_at DESC
LIMIT ?

-- before>0：取时间戳 < before 的最新 limit+1 条
SELECT id, content, received_at FROM bee_platform_messages
WHERE session_key = ? AND status != 'merged' AND received_at < ?
ORDER BY received_at DESC
LIMIT ?
```

查询用 DESC 取「最近的 limit+1 条」；handler 层合并后再反转为 ASC 供前端展示。

### OutboundMessageStore.ListBySessionKey

```go
// Before
func (s *OutboundMessageStore) ListBySessionKey(ctx context.Context, sessionKey string, limit int) ([]OutboundMessage, error)

// After
func (s *OutboundMessageStore) ListBySessionKey(ctx context.Context, sessionKey string, before int64, limit int) ([]OutboundMessage, error)
```

SQL 逻辑同上，过滤条件改为 `sent_at < ?`。

### has_more 判断

两个 store 各多查 1 条（`LIMIT limit+1`），合并排序后：

- 若总数 > limit，则 `has_more = true`，截取前 limit 条返回
- 无需额外 COUNT 查询

---

## Handler Layer

`getMessages` 改动：

1. 从 query string 解析 `before`（int64）和 `limit`（int，默认 50，上限 100）
2. 并行调用两个 store，各传入 `before` 和 `limit+1`
3. 合并、按时间升序排序
4. 判断 `has_more`，截断为 limit 条
5. 返回 `{ messages, has_more }` 对象

---

## Frontend

### api.ts

```ts
getMessages(before?: number, limit = 50): Promise<{ messages: ChatMessage[], has_more: boolean }>
```

### 聊天组件逻辑

- **初始加载**：调用 `getMessages()`，记录 `has_more` 和最早消息的 `ts`
- **加载更早消息**：用户滚动到顶且 `has_more=true` 时，调用 `getMessages(earliestTs)`，将结果前置插入消息列表
- **滚动位置保持**：加载完成后恢复到加载前的滚动位置，避免内容跳动
- **Loading 状态**：加载中在顶部显示 loading 指示器

---

## Files Affected

| 文件 | 改动类型 |
|------|--------|
| `internal/infra/store/message_store.go` | `ListBySessionKey` 加 `before`/`limit` 参数，SQL 加 WHERE + LIMIT |
| `internal/infra/store/outbound_message_store.go` | `ListBySessionKey` 加 `before` 参数，SQL 加 `sent_at < ?` |
| `internal/api/local_chat_handler.go` | 解析 query 参数，调整 store 调用，返回新响应格式 |
| `web/src/lib/api.ts` | 更新 `getMessages` 签名和返回类型 |
| 前端聊天组件 | 实现加载更多逻辑 + 滚动位置保持 |

---

## Out of Scope

- 消息归档 / 历史数据清理（当前规模不需要）
- 关键词搜索 / 跳转（当前无此需求）
- 多 session 支持（本地只有 `local:default`）
