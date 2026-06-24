import { Link } from "react-router-dom"
import { LogoFull } from "@/components/brand/logo"
import { NavUser } from "@/components/nav-user"
import { SidebarTrigger } from "@/components/ui/sidebar"
import { getStoredUsername } from "@/lib/auth"

/**
 * Full-width top bar that spans above the sidebar and the main area.
 *
 * Holds the brand lockup on the left and the logged-in user's action menu on
 * the right (theme + logout). It intentionally carries no horizontal tabs or
 * global search — navigation lives entirely in the left sidebar. On mobile,
 * where the sidebar collapses off-canvas, a trigger is exposed here so the
 * navigation stays reachable.
 */
export function AppTopbar() {
  const username = getStoredUsername() ?? "User"

  return (
    <header className="flex h-12 w-full shrink-0 items-center gap-3 border-b bg-background px-4">
      <SidebarTrigger className="md:hidden" />
      <Link to="/" className="flex items-center" aria-label="OpenBee">
        <LogoFull className="!h-7 !w-auto" />
      </Link>
      <div className="ml-auto flex items-center gap-1">
        <NavUser username={username} variant="bar" />
      </div>
    </header>
  )
}
