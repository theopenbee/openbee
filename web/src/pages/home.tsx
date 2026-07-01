import type { ReactNode } from "react"
import { Navigate } from "react-router-dom"
import { useMe } from "@/hooks/use-me"
import { hasPermission, Perm } from "@/lib/permissions"
import { firstAccessiblePath } from "@/lib/nav"
import { NoAccessLanding } from "@/components/no-access-landing"

// Home resolves the "/" landing page against the user's permissions:
//   1. holds dashboard:read   -> render the dashboard (passed as children)
//   2. otherwise              -> redirect to their first accessible page
//   3. nothing is accessible  -> show the no-access landing (no redirect, so an
//                                empty-permission account can never loop)
export function Home({ children }: { children: ReactNode }) {
  const { data: me, isLoading } = useMe()

  if (isLoading) return null

  const perms = me?.permissions
  if (hasPermission(perms, Perm.DashboardRead)) return <>{children}</>

  const target = firstAccessiblePath(perms)
  if (target && target !== "/") return <Navigate to={target} replace />

  return <NoAccessLanding />
}
