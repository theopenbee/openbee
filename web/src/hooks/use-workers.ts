import { useQuery, useMutation, useQueryClient, keepPreviousData } from "@tanstack/react-query"
import { api } from "@/lib/api"

export function useWorkers(departmentId?: string) {
  return useQuery({
    queryKey: ["workers", { departmentId }],
    queryFn: () => api.workers.list(departmentId),
  })
}

export function useWorker(id: string) {
  return useQuery({
    queryKey: ["workers", id],
    queryFn: () => api.workers.get(id),
  })
}

export function useCreateWorker() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: {
      name: string
      description: string
      memory?: string
      work_dir?: string
      permission_scopes?: string
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
    mutationFn: ({ id, data }: { id: string; data: { description?: string; memory?: string; permission_scopes?: string } }) =>
      api.workers.update(id, data),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: ["workers", id] })
    },
  })
}

export function useWorkerExecutions(workerId: string, page: number = 1, pageSize: number = 20) {
  return useQuery({
    queryKey: ["workers", workerId, "executions", page, pageSize],
    queryFn: () => api.workers.executions(workerId, page, pageSize),
    placeholderData: keepPreviousData,
  })
}
