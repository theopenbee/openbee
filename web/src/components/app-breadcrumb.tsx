import { Fragment, useMemo } from "react"
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
  const { pathname, search } = useLocation()
  const { t } = useTranslation()
  const crumbs = useMemo(() => resolveCrumbs(pathname, search), [pathname, search])

  return (
    <Breadcrumb aria-label={t("breadcrumb.label")}>
      <BreadcrumbList>
        {crumbs.map((crumb, i) => (
          <Fragment key={i}>
            {i > 0 && <BreadcrumbSeparator />}
            <BreadcrumbItem>
              {i === crumbs.length - 1 ? (
                <BreadcrumbPage>{t(crumb.labelKey)}</BreadcrumbPage>
              ) : crumb.to ? (
                <BreadcrumbLink render={<Link to={crumb.to} />}>
                  {t(crumb.labelKey)}
                </BreadcrumbLink>
              ) : (
                // Group labels (e.g. Digital Employees) only locate the page in
                // the sidebar tree; they are neither links nor the current page.
                <span className="truncate">{t(crumb.labelKey)}</span>
              )}
            </BreadcrumbItem>
          </Fragment>
        ))}
      </BreadcrumbList>
    </Breadcrumb>
  )
}
