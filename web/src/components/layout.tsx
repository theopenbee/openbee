import { Outlet } from "react-router-dom"
import { AppSidebar } from "@/components/app-sidebar"
import { AppBreadcrumb } from "@/components/app-breadcrumb"
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar"
export function Layout() {
  return (
    <SidebarProvider defaultOpen={true}>
      <AppSidebar />
      <SidebarInset>
        <header className="flex h-12 shrink-0 items-center gap-2 border-b px-4">
          <SidebarTrigger className="-ml-1" />
          <div className="h-4 w-px shrink-0 bg-border" />
          <AppBreadcrumb />
        </header>
        <main className="min-w-0 flex-1 overflow-auto p-6">
          <Outlet />
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}
