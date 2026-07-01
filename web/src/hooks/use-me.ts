import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import type { CurrentUser } from "@/lib/types"

// useMe returns the current authenticated user, including their resolved
// permission set. Cached under a stable ["me"] key so the several callers that
// need permissions share a single request. staleTime is 0: identity and
// permissions are security-sensitive, so every mount/refocus revalidates in the
// background (still serving the cached value instantly) — a permission change
// reflects on the next navigation instead of lagging behind a long cache window.
export function useMe() {
  return useQuery<CurrentUser>({
    queryKey: ["me"],
    queryFn: () => api.me.get(),
    staleTime: 0,
  })
}
