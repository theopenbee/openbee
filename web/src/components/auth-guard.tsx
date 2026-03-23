import { type ReactNode } from "react"
import { Navigate } from "react-router-dom"
import { getAccessToken } from "@/lib/auth"

export function AuthGuard({ children }: { children: ReactNode }) {
  if (!getAccessToken()) {
    return <Navigate to="/login" replace />
  }
  return <>{children}</>
}
