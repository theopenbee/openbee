import type { ReactNode } from "react"

function HoneycombSvg() {
  return (
    <svg
      width="120"
      height="100"
      viewBox="0 0 120 100"
      fill="none"
      className="text-primary/20"
    >
      <path
        d="M30 10 L45 2 L60 10 L60 26 L45 34 L30 26Z"
        stroke="currentColor"
        strokeWidth="1.5"
        fill="currentColor"
        fillOpacity="0.05"
      />
      <path
        d="M60 10 L75 2 L90 10 L90 26 L75 34 L60 26Z"
        stroke="currentColor"
        strokeWidth="1.5"
        fill="currentColor"
        fillOpacity="0.05"
      />
      <path
        d="M15 34 L30 26 L45 34 L45 50 L30 58 L15 50Z"
        stroke="currentColor"
        strokeWidth="1.5"
        fill="currentColor"
        fillOpacity="0.05"
      />
      <path
        d="M45 34 L60 26 L75 34 L75 50 L60 58 L45 50Z"
        stroke="currentColor"
        strokeWidth="1.5"
        fill="currentColor"
        fillOpacity="0.08"
      />
      <path
        d="M75 34 L90 26 L105 34 L105 50 L90 58 L75 50Z"
        stroke="currentColor"
        strokeWidth="1.5"
        fill="currentColor"
        fillOpacity="0.05"
      />
      <path
        d="M30 58 L45 50 L60 58 L60 74 L45 82 L30 74Z"
        stroke="currentColor"
        strokeWidth="1.5"
        fill="currentColor"
        fillOpacity="0.05"
      />
      <path
        d="M60 58 L75 50 L90 58 L90 74 L75 82 L60 74Z"
        stroke="currentColor"
        strokeWidth="1.5"
        fill="currentColor"
        fillOpacity="0.05"
      />
    </svg>
  )
}

interface EmptyStateProps {
  title: string
  description?: string
  action?: ReactNode
}

export function EmptyState({ title, description, action }: EmptyStateProps) {
  return (
    <div className="animate-fade-in flex flex-col items-center justify-center py-16 text-center">
      <HoneycombSvg />
      <h2 className="mt-4 text-lg font-medium text-foreground">{title}</h2>
      {description && (
        <p className="mt-1 text-sm text-muted-foreground max-w-sm">{description}</p>
      )}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}
