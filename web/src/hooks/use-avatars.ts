import { useQuery, type QueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api"

const AVATARS_KEY = ["avatars"]

export function useAvatars() {
  return useQuery({
    queryKey: AVATARS_KEY,
    queryFn: () => api.avatars.list(),
  })
}

export function invalidateAvatars(queryClient: QueryClient) {
  queryClient.invalidateQueries({ queryKey: AVATARS_KEY })
  queryClient.invalidateQueries({ queryKey: ["workers"] })
}
