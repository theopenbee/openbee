import { useCallback } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api"
import type { OnboardingStatus } from "@/lib/types"

const ONBOARDING_KEY = ["onboarding"]

export function useOnboarding() {
  return useQuery({
    queryKey: ONBOARDING_KEY,
    queryFn: api.onboarding.get,
    staleTime: Infinity,
    retry: 1,
  })
}

export function useFinishOnboarding() {
  const queryClient = useQueryClient()
  return useCallback(
    () => queryClient.setQueryData<OnboardingStatus>(ONBOARDING_KEY, { pending: false }),
    [queryClient],
  )
}

export function useSkipOnboarding() {
  const finish = useFinishOnboarding()
  return useMutation({
    mutationFn: api.onboarding.skip,
    onSuccess: finish,
  })
}
