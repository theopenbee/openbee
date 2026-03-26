import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api"

export function useSkills() {
  return useQuery({
    queryKey: ["skills"],
    queryFn: api.skills.list,
  })
}

export function useSkill(name: string) {
  return useQuery({
    queryKey: ["skills", name],
    queryFn: () => api.skills.get(name),
    enabled: !!name,
  })
}

export function useSkillVersionContent(name: string, version: string) {
  return useQuery({
    queryKey: ["skills", name, "versions", version],
    queryFn: () => api.skills.getVersionContent(name, version),
    enabled: !!name && !!version,
  })
}

export function useWorkerSkills(workerId: string) {
  return useQuery({
    queryKey: ["workers", workerId, "skills"],
    queryFn: () => api.workerSkills.list(workerId),
    enabled: !!workerId,
  })
}

export function useCreateSkill() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: { name: string; description: string; content: string }) =>
      api.skills.create(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["skills"] })
    },
  })
}

export function useDeleteSkill() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => api.skills.delete(name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["skills"] })
    },
  })
}

export function useCreateSkillVersion(name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (content: string) => api.skills.createVersion(name, content),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["skills", name] })
    },
  })
}

export function useSetGlobalVersion(name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (version: string) => api.skills.setGlobalVersion(name, version),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["skills", name] })
    },
  })
}

export function useAdoptSkill() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => api.skills.adopt(name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["skills"] })
    },
  })
}

export function useSetWorkerSkillVersion(workerId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ name, version }: { name: string; version: string }) =>
      api.workerSkills.setVersion(workerId, name, version),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["workers", workerId, "skills"] })
    },
  })
}

export function useRemoveWorkerOverride(workerId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => api.workerSkills.removeOverride(workerId, name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["workers", workerId, "skills"] })
    },
  })
}
