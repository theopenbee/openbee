import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api"

export function useDepartments() {
  return useQuery({
    queryKey: ["departments"],
    queryFn: api.departments.list,
  })
}

export function useCreateDepartment() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: { name: string; parent_id?: string | null; sort_order?: number }) =>
      api.departments.create(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["departments"] })
    },
  })
}

export function useUpdateDepartment() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: { name?: string; parent_id?: string | null; sort_order?: number } }) =>
      api.departments.update(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["departments"] })
    },
  })
}

export function useDeleteDepartment() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.departments.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["departments"] })
    },
  })
}

export function useSetWorkerDepartments() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ workerId, departmentIds }: { workerId: string; departmentIds: string[] }) =>
      api.workers.setDepartments(workerId, departmentIds),
    onSuccess: (_, { workerId }) => {
      queryClient.invalidateQueries({ queryKey: ["workers"] })
      queryClient.invalidateQueries({ queryKey: ["workers", workerId] })
      queryClient.invalidateQueries({ queryKey: ["departments"] })
    },
  })
}
