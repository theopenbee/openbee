export type WorkerStatus = "idle" | "working" | "error"
export type ExecutionStatus = "pending" | "running" | "completed" | "failed"

export interface Worker {
  id: string
  name: string
  description: string
  memory: string
  work_dir: string
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

export interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  page_size: number
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

export interface ExecStats {
  total: number
  success: number
  failed: number
}

export interface StatsOverview {
  departments: number
  workers: number
  active_workers_today: number
  active_workers_yesterday: number
  active_workers_change: number | null
  messages_received_today: number
  messages_sent_today: number
  messages_total_today: number
  messages_total_global: number
  executions_today: ExecStats
  exec_duration_today_ms: number
  exec_duration_yesterday_ms: number
  exec_duration_total_ms: number
  scheduled_tasks: number
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
