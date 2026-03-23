import { config } from "./config"

const API_BASE = config.apiUrl

const ACCESS_TOKEN_KEY = "openbee_access_token"
const REFRESH_TOKEN_KEY = "openbee_refresh_token"

export function saveTokens(accessToken: string, refreshToken: string): void {
  localStorage.setItem(ACCESS_TOKEN_KEY, accessToken)
  localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken)
}

export function getAccessToken(): string | null {
  return localStorage.getItem(ACCESS_TOKEN_KEY)
}

export function getRefreshToken(): string | null {
  return localStorage.getItem(REFRESH_TOKEN_KEY)
}

export function clearTokens(): void {
  localStorage.removeItem(ACCESS_TOKEN_KEY)
  localStorage.removeItem(REFRESH_TOKEN_KEY)
}

// Concurrent refresh dedup — multiple 401s only trigger one refresh call
let refreshPromise: Promise<string | null> | null = null

export async function refreshAccessToken(): Promise<string | null> {
  if (refreshPromise) return refreshPromise
  refreshPromise = doRefresh().finally(() => {
    refreshPromise = null
  })
  return refreshPromise
}

async function doRefresh(): Promise<string | null> {
  const refreshToken = getRefreshToken()
  if (!refreshToken) return null

  try {
    const res = await fetch(`${API_BASE}/auth/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: refreshToken }),
    })
    if (!res.ok) return null

    const data = await res.json()
    const newAccessToken = data.access_token as string
    // Keep existing refresh token, only update access token
    localStorage.setItem(ACCESS_TOKEN_KEY, newAccessToken)
    return newAccessToken
  } catch {
    return null
  }
}

let authRequiredCache: boolean | null = null

export async function checkAuthRequired(): Promise<boolean> {
  if (authRequiredCache !== null) return authRequiredCache
  try {
    const res = await fetch(`${API_BASE}/auth/status`)
    if (!res.ok) return false
    const data = await res.json()
    authRequiredCache = data.auth_required === true
    return authRequiredCache
  } catch {
    return false
  }
}

export interface LoginResult {
  success: boolean
  status?: number
}

export async function login(username: string, password: string): Promise<LoginResult> {
  try {
    const res = await fetch(`${API_BASE}/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
    })

    if (!res.ok) {
      return { success: false, status: res.status }
    }

    const data = await res.json()
    saveTokens(data.access_token, data.refresh_token)
    return { success: true }
  } catch {
    return { success: false }
  }
}
