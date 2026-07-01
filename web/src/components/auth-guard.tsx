import { type ReactNode } from "react"
import { Navigate } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import { getAccessToken } from "@/lib/auth"
import { api } from "@/lib/api"

export function AuthGuard({ children }: { children: ReactNode }) {
  const { data, isLoading } = useQuery({
    queryKey: ["setup", "status"],
    queryFn: () => api.setup.status(),
    staleTime: Infinity,
    retry: 1,
  })

  // An authenticated user is always allowed through. Checking the token first
  // means a freshly-created admin (who just saved tokens on /setup) is never
  // bounced back by a stale setup probe still reporting initialized: false.
  if (getAccessToken()) {
    return <>{children}</>
  }

  // Wait for the setup probe before deciding where to send an anonymous user.
  if (isLoading) {
    return null
  }

  // First-run: no account exists yet — route to the setup wizard.
  if (data && !data.initialized) {
    return <Navigate to="/setup" replace />
  }

  return <Navigate to="/login" replace />
}
