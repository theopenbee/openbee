import * as React from "react"
import { useMe } from "@/hooks/use-me"
import { useCan } from "@/hooks/use-can"
import { hasPermission } from "@/lib/permissions"
import { isForbidden } from "@/lib/api"
import { PermissionDenied } from "@/components/permission-denied"

// Guard front-runs a protected route: when the current user lacks `perm` it
// renders PermissionDenied instead of the page, so the gated endpoint is never
// even called. Ungated routes (perm omitted) pass straight through. While the
// permission set is still loading it renders nothing, matching the sidebar's
// no-flash behaviour.
export function Guard({
  perm,
  children,
}: {
  perm?: string
  children: React.ReactNode
}) {
  const { data: me, isLoading } = useMe()

  if (perm === undefined) return <>{children}</>
  if (isLoading) return null
  if (!hasPermission(me?.permissions, perm)) return <PermissionDenied />
  return <>{children}</>
}

// Can is the inline counterpart to Guard: it renders its children only when the
// current user holds `perm`, and nothing otherwise. Use it to hide action
// buttons and menu items the user has no permission to act on (an undefined
// perm renders children unconditionally).
export function Can({
  perm,
  children,
}: {
  perm?: string
  children: React.ReactNode
}) {
  return useCan(perm) ? <>{children}</> : null
}

// ForbiddenBoundary catches a 403 that a query throws after permissions drift
// mid-session (the guard's client-side pre-check covers the common case). The
// query client is configured to throw only on 403, so any other error here is
// re-thrown for an outer boundary rather than masked as "no permission". Reset
// it by keying on the route path so navigating away clears the forbidden state.
export class ForbiddenBoundary extends React.Component<
  { children: React.ReactNode },
  { error: unknown }
> {
  state: { error: unknown } = { error: null }

  static getDerivedStateFromError(error: unknown) {
    return { error }
  }

  render() {
    if (this.state.error) {
      if (isForbidden(this.state.error)) return <PermissionDenied />
      throw this.state.error
    }
    return this.props.children
  }
}
