import type { Worker, ChatMessage, LocalMessagesResponse, Department, DepartmentTree, EnvConfig, AppConfig, Engine, SetupStatus, TokenResponse, PluginStatus, PluginDetail, PluginCatalog, PluginOperation, PluginSettingsUpdate } from "./types"
import i18n from "i18next"
import { config } from "./config"
import { getAccessToken, getRefreshToken, refreshAccessToken, clearTokens } from "./auth"

const API_BASE = config.apiUrl

// ApiError carries the HTTP status alongside the message so callers can tell a
// specific failure (e.g. a 404 or 409) apart from others. Without it the status
// is lost and every failure reads as a generic error.
//
// code (when present) is a stable, machine-readable identifier for the backend
// error that getErrorMessage maps to a localized string via the `errors` i18n
// namespace; params carries interpolation values for that string. message stays
// as the untranslated fallback.
export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public code?: string,
    public params?: Record<string, unknown>,
  ) {
    super(message)
    this.name = "ApiError"
  }
}

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

  if (res.status === 401) {
    const newToken = getRefreshToken() ? await refreshAccessToken() : null
    if (newToken) {
      headers.set("Authorization", `Bearer ${newToken}`)
      res = await fetch(url, { ...init, headers })
    }
    if (res.status === 401) {
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
    throw new ApiError(res.status, err.error || res.statusText, err.code, err.params)
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
    setDepartments: (id: string, departmentIds: string[]) =>
      fetchAPI(`/workers/${id}/departments`, {
        method: "PUT",
        body: JSON.stringify({ department_ids: departmentIds }),
      }),
    randomName: () => fetchAPI<{ name?: string; exhausted?: boolean }>("/workers/random-name"),
  },
  localChat: {
    sendMessage: (id: string, content: string, mediaPaths?: string[]) =>
      fetchAPI<{ id: string; ts: number }>("/local/messages", {
        method: "POST",
        body: JSON.stringify({ id, content, media_paths: mediaPaths }),
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
    create: (data: { name: string; parent_id?: string | null; sort_order?: number }) =>
      fetchAPI<Department>("/departments", { method: "POST", body: JSON.stringify(data) }),
    update: (id: string, data: { name?: string; parent_id?: string | null; sort_order?: number }) =>
      fetchAPI<Department>(`/departments/${id}`, { method: "PUT", body: JSON.stringify(data) }),
    delete: (id: string) =>
      fetchAPI(`/departments/${id}`, { method: "DELETE" }),
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
  setup: {
    // Public endpoints — no token required (fetchAPI tolerates a missing token).
    status: () => fetchAPI<SetupStatus>("/setup/status"),
    create: (data: { username: string; password: string; display_name?: string }) =>
      fetchAPI<TokenResponse>("/setup", { method: "POST", body: JSON.stringify(data) }),
  },
  me: {
    changePassword: (oldPassword: string, newPassword: string) =>
      fetchAPI("/me/password", {
        method: "POST",
        body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
      }),
  },
  plugins: {
    list: async () => {
      const res = await fetchAPI<{ plugins: PluginStatus[] | null }>("/plugins")
      return Array.isArray(res?.plugins) ? res.plugins : []
    },
    get: (id: string) => fetchAPI<PluginDetail>(`/plugins/${encodeURIComponent(id)}`),
    catalog: (refresh = false) => fetchAPI<PluginCatalog>(`/plugins/catalog${refresh ? "?refresh=1" : ""}`),
    install: (spec: string) =>
      fetchAPI<PluginOperation>("/plugins/install", { method: "POST", body: JSON.stringify({ spec }) }),
    operations: async () => {
      const res = await fetchAPI<{ operations: PluginOperation[] | null }>("/plugins/operations")
      return Array.isArray(res?.operations) ? res.operations : []
    },
    updateSettings: (id: string, data: PluginSettingsUpdate) =>
      fetchAPI(`/plugins/${encodeURIComponent(id)}/settings`, { method: "PUT", body: JSON.stringify(data) }),
    enable: (id: string) => fetchAPI(`/plugins/${encodeURIComponent(id)}/enable`, { method: "POST" }),
    disable: (id: string) => fetchAPI(`/plugins/${encodeURIComponent(id)}/disable`, { method: "POST" }),
    restart: (id: string) => fetchAPI(`/plugins/${encodeURIComponent(id)}/restart`, { method: "POST" }),
    uninstall: (id: string) => fetchAPI<PluginOperation>(`/plugins/${encodeURIComponent(id)}`, { method: "DELETE" }),
    startSetup: (id: string) =>
      fetchAPI<{ id: string }>(`/plugins/${encodeURIComponent(id)}/setup`, { method: "POST" }),
    setupInput: (sid: string, text: string) =>
      fetchAPI(`/plugins/setup/${encodeURIComponent(sid)}/input`, { method: "POST", body: JSON.stringify({ text }) }),
    cancelSetup: (sid: string) => fetchAPI(`/plugins/setup/${encodeURIComponent(sid)}`, { method: "DELETE" }),
    setupEventsUrl: (sid: string) => `${API_BASE}/plugins/setup/${encodeURIComponent(sid)}/events`,
  },
}
