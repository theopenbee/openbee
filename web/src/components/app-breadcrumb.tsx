import { Fragment } from "react"
import { Link, useLocation } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb"
import { resolveCrumbs } from "@/lib/breadcrumb-config"

export function AppBreadcrumb() {
  const { pathname } = useLocation()
  const { t } = useTranslation()
  const crumbs = resolveCrumbs(pathname)

  return (
    <Breadcrumb>
      <BreadcrumbList>
        {crumbs.map((crumb, i) => (
          <Fragment key={i}>
            {i > 0 && <BreadcrumbSeparator />}
            <BreadcrumbItem>
              {crumb.to ? (
                <BreadcrumbLink render={<Link to={crumb.to} />}>
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
  )
}
