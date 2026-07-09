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

// LocalChatWorker is a digital employee a user can open a 1:1 conversation with
// from local chat. Served by GET /local/workers (chat:write scoped).
export interface LocalChatWorker {
  id: string
  name: string
  description: string
  status: string
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
  created_at: number
  updated_at: number
}

export interface StatsOverview {
  departments: number
  workers: number
  scheduled_tasks: number
  tokens_today_total: number
  tokens_yesterday_total: number
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

// Mirrors ai.AllEngines in Go — keep in sync.
export const ENGINES = ["claude", "codex", "pi"] as const
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

export interface AppVersion {
  version: string
  commit: string
  date: string
  go_version: string
  os: string
  arch: string
}

// RBAC --------------------------------------------------------------------

export type UserStatus = "active" | "disabled"

export interface Role {
  id: string
  name: string
  description: string
  is_system: boolean
  permissions?: string[]
}

export interface UserWithRoles {
  id: string
  username: string
  display_name: string
  status: UserStatus
  roles: Role[]
}

export interface CurrentUser extends UserWithRoles {
  permissions: string[]
}

export interface PermissionGroup {
  resource: string
  permissions: string[]
}

export interface SetupStatus {
  initialized: boolean
}

export interface TokenResponse {
  access_token: string
  refresh_token: string
  expires_in: number
}
