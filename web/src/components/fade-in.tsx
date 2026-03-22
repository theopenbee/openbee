import type { ReactNode } from "react"

export function FadeIn({ children }: { children: ReactNode }) {
  return <div className="animate-fade-in">{children}</div>
}
