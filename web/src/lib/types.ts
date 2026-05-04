export type WorkerStatus = "idle" | "working" | "error"
export type ExecutionStatus = "pending" | "running" | "completed" | "failed"

export interface Worker {
  id: string
  name: string
  description: string
  constraints: string
  work_dir: string
  engine: Engine
  engine_args?: Record<string, string>
  permission_scopes?: string
  status: WorkerStatus
  departments?: DepartmentBrief[]
  created_at: number
  updated_at: number
}

export interface Department {
  id: string
  name: string
  parent_id: string | null
  sort_order: number
  created_at: number
  updated_at: number
}

export interface DepartmentTree extends Department {
  children: DepartmentTree[]
}

export interface DepartmentBrief {
  id: string
  name: string
}

export interface WorkerExecution {
  id: string
  worker_id?: string
  worker_name?: string
  session_id: string
  trigger_input: string
  status: ExecutionStatus
  result: string
  logs: string | null
  ai_process_pid: number
  started_at: number | null
  completed_at: number | null
}

export interface ModelTokenStats {
  model: string
  total_tokens: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
}

export interface SessionTokenStats {
  total_tokens: number
  by_model: ModelTokenStats[]
}

export interface SessionDetail {
  executions: WorkerExecution[]
  token_stats: SessionTokenStats | null
}

export interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  token_stats?: Record<string, SessionTokenStats | null>
}

export interface ChatMessage {
  role: "user" | "bee"
  content: string
  media_paths?: string[]
  ts: number
}

export interface LocalMessagesResponse {
  messages: ChatMessage[]
  has_more: boolean
}

export type TaskType = "immediate" | "countdown" | "scheduled"
export type TaskStatus = "pending" | "running" | "completed" | "failed" | "cancelled"

export interface Task {
  id: string
  worker_id: string
  worker_name: string
  instruction: string
  type: TaskType
  status: TaskStatus
  scheduled_at: number | null
  cron_expr: string
  next_run_at: number | null
  execution_id: string
  created_at: number
  updated_at: number
}

export interface StatsOverview {
  departments: number
  workers: number
  active_workers_today: number
  active_workers_yesterday: number
  active_workers_change: number | null
  messages_total_today: number
  messages_total_yesterday: number
  messages_change: number | null
  messages_total_global: number
  executions_today: number
  executions_yesterday: number
  executions_change: number | null
  exec_duration_today_ms: number
  exec_duration_yesterday_ms: number
  exec_duration_total_ms: number
  scheduled_tasks: number
  tokens_total: number
  tokens_today_total: number
  tokens_yesterday_total: number
}

export interface TrendPoint {
  date: string
  active_workers: number
}

export interface StatsTrend {
  days: number
  data: TrendPoint[]
}

export interface ExecDurationTrendPoint {
  date: string
  total_duration_ms: number
}

export interface ExecDurationTrend {
  days: number
  data: ExecDurationTrendPoint[]
}

export interface TokenTrendPoint {
  date: string
  total_tokens: number
}

export interface TokenTrend {
  days: number
  data: TokenTrendPoint[]
}

export type EnvScope = "global" | "bee" | "department" | "worker"

// Matches the backend constant defaultBeeID in internal/domain/bee/bee_process.go.
export const DEFAULT_BEE_ID = "default"

// Mirror model.SystemConfigKey* in Go — keep in sync.
export const SYSTEM_CONFIG_KEY_DEFAULT_ENGINE = "default_engine"
export const SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL = "engine_args_global"
export const SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE = "engine_args_bee"
export const SYSTEM_CONFIG_KEY_LINEAR_PROJECTS = "linear_projects"

// Mirrors ai.AllEngines in Go — keep in sync.
export const ENGINES = ["claude", "codex", "pi", "kimi"] as const
export type Engine = (typeof ENGINES)[number]
export const DEFAULT_ENGINE: Engine = ENGINES[0]

// pickDefaultEngine returns the most appropriate engine to seed a form with:
// the worker's current engine if set, else the first server-enabled engine,
// else the global default. Shared by create/edit worker sheets so the rule
// stays in one place.
export function pickDefaultEngine(current: Engine | undefined, enabled: readonly Engine[]): Engine {
  return current ?? enabled[0] ?? DEFAULT_ENGINE
}

export interface EnvConfig {
  id: string
  scope: EnvScope
  scope_id: string | null
  key: string
  masked: string
  created_at: number
  updated_at: number
}

export interface AppConfig {
  language: string
  enabled_engines: Engine[]
}
