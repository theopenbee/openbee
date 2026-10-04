import { NAV, isNavGroup } from "@/lib/nav"

export type CrumbDef = { labelKey: string; to?: string }

// Crumbs mirror the sidebar tree: a grouped page leads with its group label
// (e.g. Digital Employees / Workers), the way the Cloudflare dashboard shows
// where a page sits before naming it. The trail is read from NAV, so moving or
// renaming a nav entry updates its breadcrumbs too.
function navTrail(url: string, linkPage: boolean): CrumbDef[] {
  for (const entry of NAV) {
    if (isNavGroup(entry)) {
      const sub = entry.items.find((s) => s.url === url)
      if (sub) return [{ labelKey: entry.titleKey }, { labelKey: sub.titleKey, to: linkPage ? sub.url : undefined }]
    } else if (entry.url === url) {
      return [{ labelKey: entry.titleKey, to: linkPage ? entry.url : undefined }]
    }
  }
  return []
}

// Sub-pages that carry their own name; any other path below a nav page is a
// detail view. The create route doubles as the copy flow (?copy=<id>), so it is
// named the way its page header is.
const SUBPAGE_LABEL: Record<string, (params: URLSearchParams) => string> = {
  "/workers/create": (params) => (params.get("copy") ? "workers.copyWorker" : "workers.createWorker"),
}

export function resolveCrumbs(pathname: string, search = ""): CrumbDef[] {
  const page = navTrail(pathname, false)
  if (page.length > 0) return page

  // Below a nav page (/workers/:id, /sessions/detail, ...): link the parent
  // page's trail and name the leaf.
  const parent = navTrail(`/${pathname.split("/")[1]}`, true)
  if (parent.length === 0) return []
  const leaf = SUBPAGE_LABEL[pathname]?.(new URLSearchParams(search)) ?? "breadcrumb.detail"
  return [...parent, { labelKey: leaf }]
}
