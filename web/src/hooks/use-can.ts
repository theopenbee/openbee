import { useMe } from "@/hooks/use-me"
import { hasPermission } from "@/lib/permissions"

// useCan reports whether the current user holds `perm`. An undefined perm is
// treated as ungated (always allowed). While the permission set is still
// loading it returns false so gated UI stays hidden rather than flashing in
// then out — the same no-flash behaviour the sidebar and Guard rely on.
export function useCan(perm?: string): boolean {
  const { data: me, isLoading } = useMe()
  if (perm === undefined) return true
  if (isLoading) return false
  return hasPermission(me?.permissions, perm)
}
