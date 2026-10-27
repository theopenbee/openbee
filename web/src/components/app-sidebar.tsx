import { Link } from "react-router-dom"
import { LogoFull } from "@/components/brand/logo"
import { NavMain } from "@/components/nav-main"
import { NavUser } from "@/components/nav-user"
import { SettingsButton } from "@/components/settings/settings-button"
import { cn } from "@/lib/utils"

export function AppSidebar({ className }: { className?: string }) {
  return (
    <aside className={cn("flex h-full w-60 shrink-0 flex-col border-r border-border bg-background", className)}>
      <div className="flex h-16 shrink-0 items-center px-5">
        <Link to="/" aria-label="OpenBee" className="flex items-center rounded-control">
          <LogoFull className="h-7 w-auto" />
        </Link>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-3 py-2">
        <NavMain />
      </div>
      <div className="flex shrink-0 items-center justify-between gap-2 border-t border-border-subtle px-3 py-3">
        <NavUser showName align="start" />
        <SettingsButton />
      </div>
    </aside>
  )
}
