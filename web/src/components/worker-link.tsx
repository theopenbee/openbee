import { Link } from "react-router-dom"
import { cn } from "@/lib/utils"

interface WorkerLinkProps {
  id: string
  name?: string
  className?: string
}

export function WorkerLink({ id, name, className }: WorkerLinkProps) {
  return (
    <Link to={`/workers/${id}`} className={cn("transition-colors hover:text-foreground", className)}>
      {name || id.slice(0, 8)}
    </Link>
  )
}
