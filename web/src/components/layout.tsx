import { Fragment } from "react"
import { Outlet, Link, useLocation } from "react-router-dom"
import { useTranslation } from "react-i18next"
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
  labelKey: string
  href?: string
}

function getBreadcrumbs(pathname: string): BreadcrumbSegment[] {
  if (pathname === "/") return [{ labelKey: "nav.dashboard" }]
  if (pathname === "/workers") return [{ labelKey: "nav.workers" }]
  if (/^\/workers\/[^/]+/.test(pathname))
    return [{ labelKey: "nav.workers", href: "/workers" }, { labelKey: "nav.workerDetail" }]
  if (pathname === "/executions") return [{ labelKey: "nav.executions" }]
  if (/^\/executions\/[^/]+/.test(pathname))
    return [{ labelKey: "nav.executions", href: "/executions" }, { labelKey: "nav.sessionDetail" }]
  if (/^\/sessions\/[^/]+/.test(pathname))
    return [{ labelKey: "nav.executions", href: "/executions" }, { labelKey: "nav.sessionDetail" }]
  if (pathname === "/tasks") return [{ labelKey: "nav.tasks" }]
  if (pathname === "/local-chat") return [{ labelKey: "nav.localChat" }]
  if (/^\/local-chat\/[^/]+/.test(pathname))
    return [{ labelKey: "nav.localChat", href: "/local-chat" }, { labelKey: "nav.chatDetail" }]
  return [{ labelKey: "nav.dashboard" }]
}

export function Layout() {
  const { pathname } = useLocation()
  const { t } = useTranslation()
  const crumbs = getBreadcrumbs(pathname)

  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <header className="flex h-14 shrink-0 items-center gap-2 border-b px-4">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-2 h-4 self-center" />
          <Breadcrumb>
            <BreadcrumbList>
              {crumbs.map((crumb, i) => (
                <Fragment key={i}>
                  {i > 0 && <BreadcrumbSeparator />}
                  <BreadcrumbItem>
                    {crumb.href ? (
                      <BreadcrumbLink render={<Link to={crumb.href} />}>
                        {t(crumb.labelKey)}
                      </BreadcrumbLink>
                    ) : (
                      <BreadcrumbPage>{t(crumb.labelKey)}</BreadcrumbPage>
                    )}
                  </BreadcrumbItem>
                </Fragment>
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
