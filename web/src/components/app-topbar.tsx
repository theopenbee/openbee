import { Link } from "react-router-dom"
import { AppBreadcrumb } from "@/components/app-breadcrumb"
import { LogoFull } from "@/components/brand/logo"
import { NavUser } from "@/components/nav-user"
import { SidebarTrigger } from "@/components/ui/sidebar"
import { getStoredUsername } from "@/lib/auth"

/**
 * Full-width top bar that spans above the sidebar and the main area.
 *
 * Cloudflare-dashboard shape: the brand mark, a slash, then the breadcrumb of
 * the current location on the left; the logged-in user's action menu (theme +
 * logout) on the right. Navigation itself lives in the left sidebar. On mobile,
 * where the sidebar collapses off-canvas, a trigger is exposed here so the
 * navigation stays reachable and the breadcrumb yields its space.
 */
export function AppTopbar() {
  const username = getStoredUsername() ?? "User"

  return (
    <header className="flex h-12 w-full shrink-0 items-center gap-3 border-b border-border bg-background px-4">
      <SidebarTrigger className="md:hidden" />
      <Link
        to="/"
        className="flex items-center rounded-sm focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        aria-label="OpenBee"
      >
        <LogoFull className="!h-6 !w-auto" />
      </Link>
      <span aria-hidden className="hidden h-5 w-px rotate-[20deg] bg-border md:block" />
      <div className="hidden min-w-0 md:block">
        <AppBreadcrumb />
      </div>
      <div className="ml-auto flex items-center gap-1">
        <NavUser username={username} variant="bar" />
      </div>
    </header>
  )
}
