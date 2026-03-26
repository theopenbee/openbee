export type WorkerStatus = "idle" | "working" | "error"
export type ExecutionStatus = "pending" | "running" | "completed" | "failed"

export interface Worker {
  id: string
  name: string
  description: string
  memory: string
  work_dir: string
  status: WorkerStatus
  created_at: number
  updated_at: number
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

export interface LocalChatSession {
  id: string
  name: string
  created_at: number
  updated_at: number
}

export interface ChatMessage {
  role: "user" | "bee"
  content: string
  ts: number
}

export interface SkillVersion {
  created_at: string
}

export interface SkillEntry {
  description: string
  latest_version: string
  global_version: string
  versions: Record<string, SkillVersion>
}

export interface ScannedSkill {
  name: string
  source: "managed" | "external"
  active_version: string
  is_override: boolean
  scope: string
}
