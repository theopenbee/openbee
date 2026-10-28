import { useQuery, useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { CONVERSATIONS_KEY } from "@/lib/chat-events"
import type { Worker, Engine } from "@/lib/types"

function workersQuery(departmentId?: string) {
  return {
    queryKey: ["workers", { departmentId }],
    queryFn: () => api.workers.list(departmentId),
  }
}

export function useWorkers(departmentId?: string) {
  return useQuery(workersQuery(departmentId))
}

export function fetchWorkers(queryClient: QueryClient) {
  return queryClient.fetchQuery(workersQuery())
}

export function useWorker(id: string, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: ["workers", id],
    queryFn: () => api.workers.get(id),
    enabled: options?.enabled,
  })
}

export function useCreateWorker() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: {
      name: string
      engine: Engine
      description: string
      constraints?: string
      work_dir?: string
      permission_scopes?: string
      engine_args?: Record<string, string>
    }) => api.workers.create(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["workers"] })
      queryClient.invalidateQueries({ queryKey: CONVERSATIONS_KEY })
    },
  })
}

export function useDeleteWorker() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, deleteWorkDir }: { id: string; deleteWorkDir: boolean }) =>
      api.workers.delete(id, deleteWorkDir),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: ["workers"], predicate: (query) => query.queryKey[1] !== id })
      queryClient.invalidateQueries({ queryKey: CONVERSATIONS_KEY })
    },
  })
}

export function useUpdateWorker() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<Worker> }) =>
      api.workers.update(id, data),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: ["workers"] })
      queryClient.invalidateQueries({ queryKey: ["workers", id] })
    },
  })
}

export function useRandomWorkerName() {
  return useMutation({
    mutationFn: () => api.workers.randomName(),
  })
}
