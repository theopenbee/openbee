import type { Worker, WorkerExecution, LocalChatSession, ChatMessage } from "./types"
import i18n from "i18next"
import { config } from "./config"
import { getAccessToken, getRefreshToken, refreshAccessToken, clearTokens } from "./auth"

const API_BASE = config.apiUrl

function authHeaders(): Record<string, string> {
  const token = getAccessToken()
  return token ? { Authorization: `Bearer ${token}` } : {}
}

function redirectToLogin() {
  clearTokens()
  window.location.hash = "#/login"
}

async function fetchWithAuth(url: string, init?: RequestInit): Promise<Response> {
  let res = await fetch(url, init)

  if (res.status === 401 && getRefreshToken()) {
    const newToken = await refreshAccessToken()
    if (newToken) {
      const headers = new Headers(init?.headers)
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
      ...authHeaders(),
      ...(extraHeaders as Record<string, string>),
    },
    ...restOptions,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
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
    executions: async (id: string) => {
      const execs = await fetchAPI<WorkerExecution[] | null>(`/workers/${id}/executions`)
      return Array.isArray(execs) ? execs : []
    },
  },
  executions: {
    list: async () => {
      const executions = await fetchAPI<WorkerExecution[] | null>("/executions")
      return Array.isArray(executions) ? executions : []
    },
    get: (id: string) => fetchAPI<WorkerExecution>(`/executions/${id}`),
    logs: async (id: string): Promise<string> => {
      const res = await fetchWithAuth(`${API_BASE}/executions/${id}/logs`, {
        headers: {
          "Accept-Language": i18n.language || "en",
          ...authHeaders(),
        },
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
    sendMessage: (sessionId: string, content: string, mediaPath?: string) =>
      fetchAPI(`/local/sessions/${sessionId}/messages`, {
        method: "POST",
        body: JSON.stringify({ content, media_path: mediaPath }),
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
        headers: authHeaders(),
        body: form,
      })
      if (!res.ok) throw new Error(await res.text())
      return res.json() as Promise<{ path: string }>
    },
  },
}
