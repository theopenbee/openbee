import type { Worker, WorkerExecution, PaginatedResponse, ChatMessage, LocalMessagesResponse, Task, Department, DepartmentTree, StatsOverview, StatsTrend, EnvConfig, ExecDurationTrend, TokenTrend, AppConfig, Engine, SessionDetail } from "./types"
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
  config: {
    get: () => fetchAPI<AppConfig>("/config"),
  },
  workers: {
    list: async (departmentId?: string) => {
      const qs = departmentId ? `?department_id=${departmentId}` : ""
      const workers = await fetchAPI<Worker[] | null>(`/workers${qs}`)
      return Array.isArray(workers) ? workers : []
    },
    get: (id: string) => fetchAPI<Worker>(`/workers/${id}`),
    create: (data: {
      name: string
      engine: Engine
      description: string
      constraints?: string
      work_dir?: string
      permission_scopes?: string
      engine_args?: Record<string, string>
    }) => fetchAPI<Worker>("/workers", { method: "POST", body: JSON.stringify(data) }),
    update: (id: string, data: Partial<Worker>) =>
      fetchAPI<Worker>(`/workers/${id}`, { method: "PUT", body: JSON.stringify(data) }),
    delete: (id: string, deleteWorkDir = false) =>
      fetchAPI(`/workers/${id}${deleteWorkDir ? "?delete_work_dir=true" : ""}`, { method: "DELETE" }),
    getDepartments: (id: string) => fetchAPI<Department[]>(`/workers/${id}/departments`),
    setDepartments: (id: string, departmentIds: string[]) =>
      fetchAPI(`/workers/${id}/departments`, {
        method: "PUT",
        body: JSON.stringify({ department_ids: departmentIds }),
      }),
    randomName: () => fetchAPI<{ name?: string; exhausted?: boolean }>("/workers/random-name"),
  },
  executions: {
    logs: async (id: string, since: number = 0): Promise<{ content: string; size: number; truncated: boolean }> => {
      const qs = since > 0 ? `?since=${since}` : ""
      const res = await fetchWithAuth(`${API_BASE}/sessions/${id}/logs${qs}`, {
        headers: { "Accept-Language": i18n.language || "en" },
      })
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: res.statusText }))
        throw new Error(err.error || res.statusText)
      }
      return res.json()
    },
  },
  sessions: {
    list: async (page: number = 1, pageSize: number = 20, workerID?: string) => {
      const qs = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
      if (workerID) qs.set("worker_id", workerID)
      return fetchAPI<PaginatedResponse<WorkerExecution>>(`/sessions?${qs}`)
    },
    get: async (sessionId: string) => {
      const detail = await fetchAPI<SessionDetail>(`/sessions/${encodeURIComponent(sessionId)}`)
      return {
        executions: Array.isArray(detail?.executions) ? detail.executions : [],
        token_stats: detail?.token_stats ?? null,
      }
    },
  },
  localChat: {
    sendMessage: (content: string, mediaPaths?: string[]) =>
      fetchAPI("/local/messages", {
        method: "POST",
        body: JSON.stringify({ content, media_paths: mediaPaths }),
      }),
    getMessages: async (before?: number, limit = 50): Promise<LocalMessagesResponse> => {
      const qs = new URLSearchParams({ limit: String(limit) })
      if (before) qs.set("before", String(before))
      const res = await fetchAPI<LocalMessagesResponse>(`/local/messages?${qs}`)
      return { messages: Array.isArray(res?.messages) ? res.messages : [], has_more: res?.has_more ?? false }
    },
    uploadMedia: async (file: File) => {
      const form = new FormData()
      form.append("file", file)
      const res = await fetchWithAuth(`${API_BASE}/local/media`, {
        method: "POST",
        body: form,
      })
      if (!res.ok) throw new Error(await res.text())
      return res.json() as Promise<{ path: string }>
    },
  },
  departments: {
    list: async () => {
      const tree = await fetchAPI<DepartmentTree[] | null>("/departments")
      return Array.isArray(tree) ? tree : []
    },
    get: (id: string) => fetchAPI<Department>(`/departments/${id}`),
    create: (data: { name: string; parent_id?: string | null; sort_order?: number }) =>
      fetchAPI<Department>("/departments", { method: "POST", body: JSON.stringify(data) }),
    update: (id: string, data: { name?: string; parent_id?: string | null; sort_order?: number }) =>
      fetchAPI<Department>(`/departments/${id}`, { method: "PUT", body: JSON.stringify(data) }),
    delete: (id: string) =>
      fetchAPI(`/departments/${id}`, { method: "DELETE" }),
    workers: (id: string) => fetchAPI<Worker[]>(`/departments/${id}/workers`),
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
  stats: {
    overview: () => fetchAPI<StatsOverview>("/stats/overview"),
    trend: (days: 7 | 15 | 30) => fetchAPI<StatsTrend>(`/stats/trend?days=${days}`),
    executionDurationTrend: (days: 7 | 15 | 30) =>
      fetchAPI<ExecDurationTrend>(`/stats/execution-duration-trend?days=${days}`),
    tokenTrend: (days: 7 | 15 | 30) =>
      fetchAPI<TokenTrend>(`/stats/token-trend?days=${days}`),
  },
  envs: {
    list: (scope: string, scopeId?: string) => {
      const qs = new URLSearchParams({ scope })
      if (scopeId) qs.set("scope_id", scopeId)
      return fetchAPI<EnvConfig[]>(`/envs?${qs}`)
    },
    create: (data: { scope: string; scope_id?: string; key: string; value: string }) =>
      fetchAPI<EnvConfig>("/envs", { method: "POST", body: JSON.stringify(data) }),
    update: (id: string, value: string) =>
      fetchAPI<{ ok: boolean }>(`/envs/${id}`, {
        method: "PUT",
        body: JSON.stringify({ value }),
      }),
    delete: (id: string) => fetchAPI(`/envs/${id}`, { method: "DELETE" }),
  },
  systemConfigs: {
    get: () => fetchAPI<Record<string, string>>("/system-configs"),
    set: (key: string, value: string) =>
      fetchAPI<{ ok: boolean }>(`/system-configs/${key}`, {
        method: "PUT",
        body: JSON.stringify({ value }),
      }),
  },
}
