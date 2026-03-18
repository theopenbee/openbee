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
  session_id: string
  trigger_input: string
  status: ExecutionStatus
  result: string
  logs: string | null
  ai_process_pid: number
  started_at: number | null
  completed_at: number | null
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
