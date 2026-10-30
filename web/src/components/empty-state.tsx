import type { ReactNode } from "react"
import { EmptyState as ArcEmptyState } from "@/components/arc/empty-state/empty-state"

interface EmptyStateProps {
  title: string
  description?: string
  action?: ReactNode
  icon?: ReactNode
  className?: string
}

export function EmptyState({ title, description, action, icon, className }: EmptyStateProps) {
  return (
    <ArcEmptyState
      title={title}
      description={description ?? ""}
      action={action}
      icon={icon}
      className={className}
    />
  )
}
