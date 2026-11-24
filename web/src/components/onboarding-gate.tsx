import { type ReactNode } from "react"
import { Navigate } from "react-router-dom"
import { useOnboarding } from "@/hooks/use-onboarding"

export function OnboardingGate({ children }: { children: ReactNode }) {
  const { data, isLoading } = useOnboarding()
  if (isLoading) return null
  if (data?.pending) return <Navigate to="/welcome" replace />
  return <>{children}</>
}
