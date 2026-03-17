import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api"

export function useWorkers() {
  return useQuery({
    queryKey: ["workers"],
    queryFn: api.workers.list,
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
    mutationFn: ({ id, data }: { id: string; data: { description?: string; memory?: string } }) =>
      api.workers.update(id, data),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: ["workers", id] })
    },
  })
}

export function useWorkerExecutions(workerId: string) {
  return useQuery({
    queryKey: ["workers", workerId, "executions"],
    queryFn: () => api.workers.executions(workerId),
  })
}
