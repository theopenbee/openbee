# Dashboard 数据面板重构设计文档

**日期**: 2026-04-06  
**状态**: 已确认

---

## 背景

当前仪表盘（`web/src/pages/dashboard.tsx`）仅展示 Worker 卡片列表，没有任何数据统计能力。本次重构将其改造为真正的数据面板，提供系统运行状态的全面概览。

---

## 需求确认

| 指标 | 定义 |
|------|------|
| 部门数 | `departments` 表总记录数 |
| 员工数 | `workers` 表总记录数 |
| 活跃员工 | 当天在 `executions` 表有记录的 DISTINCT worker_id 数量 |
| 会话数量 | 今天首次出现的 `session_key`（MIN(received_at) 在今天的） |
| 消息数量（收） | `bee_platform_messages` 按 `received_at` 当天计数 |
| 消息数量（发） | `bee_outbound_messages` 按 `sent_at` 当天计数 |
| 执行数量 | `executions` 表按 `created_at` 当天计数，按 status 分组 |
| 定时任务数量 | `tasks` 表中 type IN ('countdown','scheduled') 且 status 为有效状态的总数 |
| 刷新策略 | 自动刷新，数字卡片每 30 秒轮询一次 |

---

## 方案选择

采用**方案B：分组接口 + 分区块布局**：
- 后端两个接口分别负责数字卡片数据和折线图数据
- 两者职责解耦，折线图切换天数不影响数字卡片刷新

---

## 后端设计

### 新增文件

- `internal/infra/store/stats_store.go` — 统计查询
- `internal/api/stats_handler.go` — HTTP handler
- 修改 `internal/api/router.go` — 注册路由

### 路由注册

```
GET /api/stats/overview   (JWT required)
GET /api/stats/trend      (JWT required, query param: days=7|15|30)
```

### GET /api/stats/overview

返回所有数字卡片所需数据：

```json
{
  "departments": 5,
  "workers": 12,
  "active_workers_today": 8,
  "active_workers_yesterday": 6,
  "active_workers_change": 0.333,
  "messages_received_today": 234,
  "messages_sent_today": 189,
  "sessions_new_today": 47,
  "executions_today": {
    "total": 89,
    "success": 82,
    "failed": 7
  },
  "scheduled_tasks": 15
}
```

**字段说明**：
- `active_workers_change`：`(today - yesterday) / yesterday`，yesterday 为 0 时返回 `null`
- `executions_today.total` = success + failed + 其他状态（running/pending 也计入）
- `scheduled_tasks`：type IN ('countdown','scheduled') 且 status NOT IN ('completed','cancelled')

### GET /api/stats/trend?days=7

支持 days = 7 / 15 / 30，非法值返回 400。

```json
{
  "days": 7,
  "data": [
    { "date": "2026-03-31", "active_workers": 6 },
    { "date": "2026-04-01", "active_workers": 9 },
    { "date": "2026-04-05", "active_workers": 7 },
    { "date": "2026-04-06", "active_workers": 8 }
  ]
}
```

**说明**：
- 返回最近 N 天（含今天）每天的活跃员工数
- 没有数据的日期也返回记录，active_workers 为 0（保证折线图连续）
- 日期格式：`YYYY-MM-DD`，使用服务端本地时区

### StatsStore 查询设计

```go
type StatsOverview struct {
    Departments             int      `json:"departments"`
    Workers                 int      `json:"workers"`
    ActiveWorkersToday      int      `json:"active_workers_today"`
    ActiveWorkersYesterday  int      `json:"active_workers_yesterday"`
    ActiveWorkersChange     *float64 `json:"active_workers_change"`
    MessagesReceivedToday   int      `json:"messages_received_today"`
    MessagesSentToday       int      `json:"messages_sent_today"`
    SessionsNewToday        int      `json:"sessions_new_today"`
    ExecutionsToday         ExecStats `json:"executions_today"`
    ScheduledTasks          int      `json:"scheduled_tasks"`
}

type ExecStats struct {
    Total   int `json:"total"`
    Success int `json:"success"`
    Failed  int `json:"failed"`
}

type TrendPoint struct {
    Date          string `json:"date"`
    ActiveWorkers int    `json:"active_workers"`
}
```

---

## 前端设计

### 新增/修改文件

| 文件 | 说明 |
|------|------|
| `web/src/pages/dashboard.tsx` | 全量重写为数据面板 |
| `web/src/hooks/use-stats.ts` | 新增，包含 useStatsOverview 和 useStatsTrend |
| `web/src/components/stat-card.tsx` | 新增，通用单指标卡片 |
| `web/src/components/active-workers-card.tsx` | 新增，活跃员工对比卡 |
| `web/src/components/messages-card.tsx` | 新增，消息收/发卡 |
| `web/src/components/executions-card.tsx` | 新增，执行情况卡 |
| `web/src/components/activity-trend-chart.tsx` | 新增，折线图组件 |
| `web/package.json` | 新增 recharts 依赖 |

### 页面布局

```
┌─────────────────────────────────────────────────────┐
│  数据面板                                            │
├──────────┬──────────┬──────────┬────────────────────┤
│  部门数  │  员工数  │ 定时任务 │   今日会话数        │
├──────────┴──────────┴──────────┴────────────────────┤
│  活跃员工：今日 8 / 昨日 6 / 同比 +33%  ↑           │
├────────────────────┬────────────────────────────────┤
│ 消息：收 234/发 189│ 执行：总89  成功82  失败7       │
├────────────────────┴────────────────────────────────┤
│  员工活跃趋势          [7天] [15天] [30天]           │
│  ┌──────────────────────────────────────────────┐   │
│  │              折线图                          │   │
│  └──────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────┘
```

### 组件说明

**StatCard**（通用）
- Props: `title`, `value`, `icon?`, `loading?`
- 用于：部门数、员工数、定时任务数、今日会话数

**ActiveWorkersCard**
- 展示今日/昨日活跃员工数及同比变化率
- 同比为正时显示绿色上箭头，为负时显示红色下箭头，为 null 时显示「-」

**MessagesCard**
- 并排展示「收到」和「发送」两个数字

**ExecutionsCard**
- 并排展示「总计」、「成功」（绿）、「失败」（红）三个数字

**ActivityTrendChart**
- 顶部 tab 切换 7/15/30 天，切换时调用 useStatsTrend(days)
- X 轴为日期，Y 轴为活跃员工数
- 使用 Recharts LineChart，样式跟随 CSS 变量主题色

### 数据层 Hooks

```typescript
// useStatsOverview: 30秒自动刷新
export function useStatsOverview() {
  return useQuery({
    queryKey: ["stats", "overview"],
    queryFn: () => api.stats.overview(),
    refetchInterval: 30_000,
  })
}

// useStatsTrend: 按 days 参数查询，切换时重新请求
export function useStatsTrend(days: 7 | 15 | 30) {
  return useQuery({
    queryKey: ["stats", "trend", days],
    queryFn: () => api.stats.trend(days),
  })
}
```

### API 客户端扩展

在 `web/src/lib/api.ts` 中新增 `stats` 命名空间：

```typescript
stats: {
  overview: () => get<StatsOverview>("/api/stats/overview"),
  trend: (days: number) => get<StatsTrend>(`/api/stats/trend?days=${days}`),
}
```

---

## 国际化

在现有 i18n 配置中新增 `dashboard.*` 键值：

| Key | 中文 | 英文 |
|-----|------|------|
| dashboard.departments | 部门数 | Departments |
| dashboard.workers | 员工数 | Workers |
| dashboard.scheduledTasks | 定时任务 | Scheduled Tasks |
| dashboard.sessionsToday | 今日会话 | Sessions Today |
| dashboard.activeWorkers | 活跃员工 | Active Workers |
| dashboard.today | 今日 | Today |
| dashboard.yesterday | 昨日 | Yesterday |
| dashboard.change | 同比 | Change |
| dashboard.messagesReceived | 收到消息 | Messages Received |
| dashboard.messagesSent | 发送消息 | Messages Sent |
| dashboard.executions | 执行数 | Executions |
| dashboard.executionsSuccess | 成功 | Success |
| dashboard.executionsFailed | 失败 | Failed |
| dashboard.activityTrend | 员工活跃趋势 | Worker Activity Trend |

---

## 依赖变更

```bash
# 新增图表库（recharts v2+ 内置 TypeScript 类型，无需额外 @types 包）
npm install recharts
```

---

## 不在本次范围内

- 折线图以外的其他图表类型（饼图、柱状图）
- 数据导出功能
- 自定义刷新间隔
- 历史数据对比（超过30天）
