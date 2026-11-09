import { useCallback } from "react"
import { useLocation, useNavigate } from "react-router-dom"
import { settingsPath, settingsReturnPath, type SettingsState, type SettingsTarget } from "@/lib/routes"

function useReturnPath() {
  const { pathname, search, state } = useLocation()
  return settingsReturnPath(pathname, search, state)
}

export function useOpenSettings() {
  const navigate = useNavigate()
  const { pathname, search } = useLocation()
  const returnTo = useReturnPath()
  const current = pathname + search
  return useCallback(
    (target: SettingsTarget, options?: { replace?: boolean }) => {
      const path = settingsPath(target)
      const state: SettingsState = { returnTo }
      navigate(path, { state, replace: options?.replace || path === current })
    },
    [navigate, returnTo, current],
  )
}

export function useLeaveSettings() {
  const navigate = useNavigate()
  const returnTo = useReturnPath()
  return useCallback(() => navigate(returnTo), [navigate, returnTo])
}
