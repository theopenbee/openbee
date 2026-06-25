import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"

// Build/runtime metadata is static for a deployment, so it never needs refetching.
export function useVersion() {
  return useQuery({
    queryKey: ["version"],
    queryFn: () => api.version.get(),
    staleTime: Infinity,
  })
}
