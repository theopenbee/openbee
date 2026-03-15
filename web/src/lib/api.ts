import type { Worker, WorkerExecution } from "./types"
import i18n from "i18next"

const API_BASE = import.meta.env.VITE_API_URL || "http://localhost:8080/api"

async function fetchAPI<T>(path: string, options?: RequestInit): Promise<T> {
  const { headers: extraHeaders, ...restOptions } = options ?? {}
  const res = await fetch(`${API_BASE}${path}`, {
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": i18n.language || "en",
      ...extraHeaders,
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
  },
  sessions: {
    executions: async (sessionId: string) => {
      const execs = await fetchAPI<WorkerExecution[] | null>(`/sessions/${sessionId}/executions`)
      return Array.isArray(execs) ? execs : []
    },
  },
}
