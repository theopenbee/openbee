import { useQuery, useMutation, useQueryClient, keepPreviousData } from "@tanstack/react-query"
import { api } from "@/lib/api"
import type { Worker, Engine } from "@/lib/types"

export function useWorkers(departmentId?: string) {
  return useQuery({
    queryKey: ["workers", { departmentId }],
    queryFn: () => api.workers.list(departmentId),
  })
}

export function useWorker(id: string, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: ["workers", id],
    queryFn: () => api.workers.get(id),
    enabled: options?.enabled ?? true,
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
    },
  })
}

export function useDeleteWorker() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, deleteWorkDir }: { id: string; deleteWorkDir: boolean }) =>
      api.workers.delete(id, deleteWorkDir),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["workers"] })
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

export function useWorkerExecutions(
  workerId: string,
  page: number = 1,
  pageSize: number = 20,
  options?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: ["workers", workerId, "executions", page, pageSize],
    queryFn: () => api.sessions.list(page, pageSize, workerId),
    placeholderData: keepPreviousData,
    enabled: options?.enabled ?? true,
  })
}

export function useRandomWorkerName() {
  return useMutation({
    mutationFn: () => api.workers.randomName(),
  })
}
