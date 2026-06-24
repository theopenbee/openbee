import { Outlet } from "react-router-dom"
import { AppSidebar } from "@/components/app-sidebar"
import { AppTopbar } from "@/components/app-topbar"
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"

export function Layout() {
  return (
    // The shell is a column: a full-width top bar over a row of [sidebar | main].
    // SidebarProvider normally lays its children out as a row, so we flip it to a
    // column and pin it to the viewport height; the body row below owns the
    // remaining space and lets the main pane scroll on its own.
    <SidebarProvider defaultOpen className="h-svh flex-col">
      <AppTopbar />
      <div className="flex min-h-0 w-full flex-1">
        {/* The sidebar container is `position: fixed; inset-y-0` in the primitive,
            so it would slide under the top bar. An inline offset wins over the
            class unconditionally, pinning it to start below the 3rem bar. */}
        <AppSidebar style={{ top: "3rem", height: "calc(100svh - 3rem)" }} />
        <SidebarInset>
          <main className="min-w-0 flex-1 overflow-auto p-6">
            <Outlet />
          </main>
        </SidebarInset>
      </div>
    </SidebarProvider>
  )
}
