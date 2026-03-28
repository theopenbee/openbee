import { useMutation, useQuery, useQueryClient, keepPreviousData } from "@tanstack/react-query"
import { api } from "@/lib/api"

export function useTasks(params: { workerID?: string; page?: number; pageSize?: number } = {}) {
  return useQuery({
    queryKey: ["tasks", params],
    queryFn: () => api.tasks.list(params),
    placeholderData: keepPreviousData,
    refetchInterval: 30_000,
  })
}

export function useCancelTask() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.tasks.cancel(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["tasks"] }),
  })
}

export function useCancelWorkerTasks() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (workerID: string) => api.tasks.cancelAll(workerID),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["tasks"] }),
  })
}
