import { useQuery, useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query"
import { ENGINE_MODELS_QUERY_KEY } from "@/hooks/use-engine-models"
import { api } from "@/lib/api"

export function invalidateEnvs(queryClient: QueryClient, scope: string, scopeId?: string) {
  queryClient.invalidateQueries({ queryKey: ["envs", scope, scopeId ?? null] })
  queryClient.invalidateQueries({ queryKey: [ENGINE_MODELS_QUERY_KEY] })
}

export function useEnvList(scope: string, scopeId?: string) {
  return useQuery({
    queryKey: ["envs", scope, scopeId ?? null],
    queryFn: () => api.envs.list(scope, scopeId),
    select: (data) => data ?? [],
  })
}

export function useCreateEnv() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: { scope: string; scope_id?: string; key: string; value: string }) =>
      api.envs.create(data),
    onSuccess: (_, { scope, scope_id }) => invalidateEnvs(queryClient, scope, scope_id),
  })
}

export function useUpdateEnv(scope: string, scopeId?: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, value }: { id: string; value: string }) =>
      api.envs.update(id, value),
    onSuccess: () => invalidateEnvs(queryClient, scope, scopeId),
  })
}

export function useDeleteEnv(scope: string, scopeId?: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.envs.delete(id),
    onSuccess: () => invalidateEnvs(queryClient, scope, scopeId),
  })
}
