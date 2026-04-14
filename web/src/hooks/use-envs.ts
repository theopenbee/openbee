import { useQuery, useQueries, useMutation, useQueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api"

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
    onSuccess: (_, { scope, scope_id }) => {
      queryClient.invalidateQueries({ queryKey: ["envs", scope, scope_id ?? null] })
    },
  })
}

export function useUpdateEnv(scope: string, scopeId?: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, value }: { id: string; value: string }) =>
      api.envs.update(id, value),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["envs", scope, scopeId ?? null] })
    },
  })
}

export function useDeleteEnv(scope: string, scopeId?: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.envs.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["envs", scope, scopeId ?? null] })
    },
  })
}

export function useDepartmentEnvs(departmentIds: string[]) {
  return useQueries({
    queries: departmentIds.map((id) => ({
      queryKey: ["envs", "department", id],
      queryFn: () => api.envs.list("department", id),
      select: (data: Awaited<ReturnType<typeof api.envs.list>>) => data ?? [],
    })),
  })
}
