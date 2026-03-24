import { Badge } from "@/components/ui/badge"

const statusStyles: Record<string, string> = {
  idle: "bg-green-500/15 text-green-600 dark:text-green-400 border-green-500/20",
  completed: "bg-green-500/15 text-green-600 dark:text-green-400 border-green-500/20",
  working: "bg-blue-500/15 text-blue-600 dark:text-blue-400 border-blue-500/20",
  running: "bg-blue-500/15 text-blue-600 dark:text-blue-400 border-blue-500/20",
  error: "bg-red-500/15 text-red-600 dark:text-red-400 border-red-500/20",
  failed: "bg-red-500/15 text-red-600 dark:text-red-400 border-red-500/20",
  pending: "bg-muted text-muted-foreground border-border",
}

export function StatusBadge({ status }: { status: string }) {
  return (
    <Badge variant="outline" className={statusStyles[status] || "bg-muted text-muted-foreground"}>
      {status}
    </Badge>
  )
}
