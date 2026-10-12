import { useMutation, useQuery, useQueryClient, keepPreviousData } from "@tanstack/react-query"
import { api } from "@/lib/api"
import type { CreateTaskInput, TaskInput } from "@/lib/types"

export function useTasks(params: { workerID?: string; page?: number; pageSize?: number } = {}) {
  return useQuery({
    queryKey: ["tasks", params.workerID ?? null, params.page, params.pageSize],
    queryFn: () => api.tasks.list(params),
    placeholderData: keepPreviousData,
    refetchInterval: 30_000,
  })
}

export function useTaskTargets(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: ["task-targets"],
    queryFn: () => api.tasks.targets(),
    enabled: options?.enabled ?? true,
  })
}

export function useCreateTask() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateTaskInput) => api.tasks.create(data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["tasks"] }),
  })
}

export function useUpdateTask() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: TaskInput }) => api.tasks.update(id, data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["tasks"] }),
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
