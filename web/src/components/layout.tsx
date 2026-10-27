import { Outlet, useLocation } from "react-router-dom"
import { AppSidebar } from "@/components/app-sidebar"
import { AppTopbar } from "@/components/app-topbar"
import { cn } from "@/lib/utils"

const FULL_BLEED_ROUTES = new Set(["/chat", "/workers"])

const isFullBleedPath = (pathname: string) =>
  FULL_BLEED_ROUTES.has(pathname) ||
  (/^\/workers\/[^/]+$/.test(pathname) && pathname !== "/workers/create")

export function Layout() {
  const { pathname } = useLocation()
  const fullBleed = isFullBleedPath(pathname)

  return (
    <div className="flex h-dvh bg-background">
      <AppSidebar className="hidden md:flex" />
      <div className="flex min-w-0 flex-1 flex-col">
        <AppTopbar className="md:hidden" />
        <main
          className={cn(
            "min-h-0 min-w-0 flex-1",
            fullBleed ? "flex flex-col overflow-hidden" : "overflow-y-auto",
          )}
        >
          {fullBleed ? (
            <Outlet />
          ) : (
            <div className="mx-auto w-full max-w-page px-4 py-6 sm:px-6 md:px-8 md:py-8">
              <Outlet />
            </div>
          )}
        </main>
      </div>
    </div>
  )
}
