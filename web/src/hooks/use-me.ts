import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import type { CurrentUser } from "@/lib/types"

// useMe returns the current authenticated user, including their resolved
// permission set. Cached under a stable ["me"] key so callers that need
// permissions share a single request.
export function useMe() {
  return useQuery<CurrentUser>({
    queryKey: ["me"],
    queryFn: () => api.me.get(),
    staleTime: 5 * 60 * 1000,
  })
}
