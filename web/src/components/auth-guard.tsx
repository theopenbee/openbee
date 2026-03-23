import { useEffect, useState, type ReactNode } from "react"
import { Navigate } from "react-router-dom"
import { checkAuthRequired, getAccessToken } from "@/lib/auth"

export function AuthGuard({ children }: { children: ReactNode }) {
  const [state, setState] = useState<"loading" | "authed" | "login">("loading")

  useEffect(() => {
    checkAuthRequired().then((required) => {
      if (!required) {
        setState("authed")
        return
      }
      if (getAccessToken()) {
        setState("authed")
        return
      }
      setState("login")
    })
  }, [])

  if (state === "loading") return null
  if (state === "login") return <Navigate to="/login" replace />
  return <>{children}</>
}
