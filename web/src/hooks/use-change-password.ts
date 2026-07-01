import { useMutation } from "@tanstack/react-query"
import { api } from "@/lib/api"

export function useChangePassword() {
  return useMutation({
    mutationFn: ({
      oldPassword,
      newPassword,
    }: {
      oldPassword: string
      newPassword: string
    }) => api.me.changePassword(oldPassword, newPassword),
  })
}
