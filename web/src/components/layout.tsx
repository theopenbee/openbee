import { Outlet, Link, useLocation } from "react-router-dom"
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar"
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb"
import { Separator } from "@/components/ui/separator"
import { AppSidebar } from "./app-sidebar"

interface BreadcrumbSegment {
  label: string
  href?: string
}

function getBreadcrumbs(pathname: string): BreadcrumbSegment[] {
  if (pathname === "/") return [{ label: "Dashboard" }]
  if (pathname === "/workers") return [{ label: "Workers" }]
  if (/^\/workers\/[^/]+/.test(pathname))
    return [{ label: "Workers", href: "/workers" }, { label: "Worker Detail" }]
  if (pathname === "/executions") return [{ label: "Sessions" }]
  if (/^\/executions\/[^/]+/.test(pathname))
    return [{ label: "Sessions", href: "/executions" }, { label: "Session Detail" }]
  if (/^\/sessions\/[^/]+/.test(pathname))
    return [{ label: "Sessions", href: "/executions" }, { label: "Session Detail" }]
  if (pathname === "/tasks") return [{ label: "Scheduled Tasks" }]
  if (pathname === "/local-chat") return [{ label: "Local Chat" }]
  if (/^\/local-chat\/[^/]+/.test(pathname))
    return [{ label: "Local Chat", href: "/local-chat" }, { label: "Chat Detail" }]
  return [{ label: "Dashboard" }]
}

export function Layout() {
  const { pathname } = useLocation()
  const crumbs = getBreadcrumbs(pathname)

  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <header className="flex h-14 shrink-0 items-center gap-2 border-b px-4">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-2 h-4" />
          <Breadcrumb>
            <BreadcrumbList>
              {crumbs.map((crumb, i) => (
                <span key={i} className="flex items-center gap-1.5">
                  {i > 0 && <BreadcrumbSeparator />}
                  <BreadcrumbItem>
                    {crumb.href ? (
                      <BreadcrumbLink render={<Link to={crumb.href} />}>
                        {crumb.label}
                      </BreadcrumbLink>
                    ) : (
                      <BreadcrumbPage>{crumb.label}</BreadcrumbPage>
                    )}
                  </BreadcrumbItem>
                </span>
              ))}
            </BreadcrumbList>
          </Breadcrumb>
        </header>
        <main className="flex-1 overflow-auto p-6">
          <Outlet />
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}
