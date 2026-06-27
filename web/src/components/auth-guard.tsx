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

  // Wait for the setup probe before deciding where to send the user.
  if (isLoading) {
    return null
  }

  // First-run: no account exists yet — route to the setup wizard.
  if (data && !data.initialized) {
    return <Navigate to="/setup" replace />
  }

  if (!getAccessToken()) {
    return <Navigate to="/login" replace />
  }

  return <>{children}</>
}
