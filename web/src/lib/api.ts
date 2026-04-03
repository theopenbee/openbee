import type { Worker, WorkerExecution, PaginatedResponse, LocalChatSession, ChatMessage, Task } from "./types"
import i18n from "i18next"
import { config } from "./config"
import { getAccessToken, getRefreshToken, refreshAccessToken, clearTokens } from "./auth"

const API_BASE = config.apiUrl

function redirectToLogin() {
  clearTokens()
  window.location.hash = "#/login"
}

async function fetchWithAuth(url: string, init?: RequestInit): Promise<Response> {
  const headers = new Headers(init?.headers)
  const token = getAccessToken()
  if (token) {
    headers.set("Authorization", `Bearer ${token}`)
  }

  let res = await fetch(url, { ...init, headers })

  if (res.status === 401 && getRefreshToken()) {
    const newToken = await refreshAccessToken()
    if (newToken) {
      headers.set("Authorization", `Bearer ${newToken}`)
      res = await fetch(url, { ...init, headers })
    } else {
      redirectToLogin()
      throw new Error("unauthorized")
    }
  }

  return res
}

async function fetchAPI<T>(path: string, options?: RequestInit): Promise<T> {
  const { headers: extraHeaders, ...restOptions } = options ?? {}
  const res = await fetchWithAuth(`${API_BASE}${path}`, {
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": i18n.language || "en",
      ...(extraHeaders as Record<string, string>),
    },
    ...restOptions,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

export const api = {
  workers: {
    list: async () => {
      const workers = await fetchAPI<Worker[] | null>("/workers")
      return Array.isArray(workers) ? workers : []
    },
    get: (id: string) => fetchAPI<Worker>(`/workers/${id}`),
    create: (data: {
      name: string
      description: string
      memory?: string
      work_dir?: string
    }) => fetchAPI<Worker>("/workers", { method: "POST", body: JSON.stringify(data) }),
    update: (id: string, data: Partial<Worker>) =>
      fetchAPI<Worker>(`/workers/${id}`, { method: "PUT", body: JSON.stringify(data) }),
    delete: (id: string, deleteWorkDir = false) =>
      fetchAPI(`/workers/${id}${deleteWorkDir ? "?delete_work_dir=true" : ""}`, { method: "DELETE" }),
    executions: async (id: string, page: number = 1, pageSize: number = 20) => {
      return fetchAPI<PaginatedResponse<WorkerExecution>>(
        `/workers/${id}/executions?page=${page}&page_size=${pageSize}`
      )
    },
  },
  executions: {
    list: async (page: number = 1, pageSize: number = 20) => {
      return fetchAPI<PaginatedResponse<WorkerExecution>>(
        `/executions?page=${page}&page_size=${pageSize}`
      )
    },
    get: (id: string) => fetchAPI<WorkerExecution>(`/executions/${id}`),
    logs: async (id: string): Promise<string> => {
      const res = await fetchWithAuth(`${API_BASE}/executions/${id}/logs`, {
        headers: { "Accept-Language": i18n.language || "en" },
      })
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: res.statusText }))
        throw new Error(err.error || res.statusText)
      }
      return res.text()
    },
  },
  sessions: {
    executions: async (sessionId: string) => {
      const execs = await fetchAPI<WorkerExecution[] | null>(`/sessions/${sessionId}/executions`)
      return Array.isArray(execs) ? execs : []
    },
  },
  localChat: {
    listSessions: async () => {
      const sessions = await fetchAPI<LocalChatSession[] | null>("/local/sessions")
      return Array.isArray(sessions) ? sessions : []
    },
    createSession: (name: string) =>
      fetchAPI<LocalChatSession>("/local/sessions", {
        method: "POST",
        body: JSON.stringify({ name }),
      }),
    deleteSession: (id: string) =>
      fetchAPI(`/local/sessions/${id}`, { method: "DELETE" }),
    sendMessage: (sessionId: string, content: string, mediaPaths?: string[]) =>
      fetchAPI(`/local/sessions/${sessionId}/messages`, {
        method: "POST",
        body: JSON.stringify({ content, media_paths: mediaPaths }),
      }),
    getMessages: async (sessionId: string) => {
      const msgs = await fetchAPI<ChatMessage[] | null>(`/local/sessions/${sessionId}/messages`)
      return Array.isArray(msgs) ? msgs : []
    },
    uploadMedia: async (sessionId: string, file: File) => {
      const form = new FormData()
      form.append("file", file)
      const res = await fetchWithAuth(`${API_BASE}/local/sessions/${sessionId}/media`, {
        method: "POST",
        body: form,
      })
      if (!res.ok) throw new Error(await res.text())
      return res.json() as Promise<{ path: string }>
    },
  },
  tasks: {
    list: (params: { workerID?: string; page?: number; pageSize?: number } = {}) => {
      const { workerID, page = 1, pageSize = 20 } = params
      const qs = new URLSearchParams({
        type: "scheduled,countdown",
        status: "pending,running",
        page: String(page),
        page_size: String(pageSize),
      })
      if (workerID) qs.set("worker_id", workerID)
      return fetchAPI<PaginatedResponse<Task>>(`/tasks?${qs}`)
    },
    cancel: (id: string) =>
      fetchAPI(`/tasks/${id}`, { method: "DELETE" }),
    cancelAll: (workerID: string) =>
      fetchAPI(`/workers/${workerID}/tasks/cancel-all`, { method: "POST" }),
  },
}
