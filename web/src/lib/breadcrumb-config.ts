export type CrumbDef = { labelKey: string; to?: string }

// Crumbs mirror the sidebar tree: a grouped page leads with its group label
// (e.g. Digital Employees / Workers), the way the Cloudflare dashboard shows
// where a page sits before naming it.
const ROUTES: {
  test: RegExp
  crumbs: CrumbDef[] | ((params: URLSearchParams) => CrumbDef[])
}[] = [
  {
    test: /^\/$/,
    crumbs: [{ labelKey: "nav.dashboard" }],
  },
  {
    test: /^\/chat$/,
    crumbs: [{ labelKey: "localChat.title" }],
  },
  {
    test: /^\/chat\//,
    crumbs: [
      { labelKey: "localChat.title", to: "/chat" },
      { labelKey: "breadcrumb.detail" },
    ],
  },
  {
    test: /^\/departments$/,
    crumbs: [{ labelKey: "nav.digitalEmployees" }, { labelKey: "nav.departments" }],
  },
  {
    test: /^\/workers$/,
    crumbs: [{ labelKey: "nav.digitalEmployees" }, { labelKey: "nav.workers" }],
  },
  {
    test: /^\/workers\/create$/,
    // The create route doubles as the copy flow (?copy=<id>); name it the way
    // the page header does.
    crumbs: (params) => [
      { labelKey: "nav.digitalEmployees" },
      { labelKey: "nav.workers", to: "/workers" },
      { labelKey: params.get("copy") ? "workers.copyWorker" : "workers.createWorker" },
    ],
  },
  {
    test: /^\/workers\//,
    crumbs: [
      { labelKey: "nav.digitalEmployees" },
      { labelKey: "nav.workers", to: "/workers" },
      { labelKey: "breadcrumb.detail" },
    ],
  },
  {
    test: /^\/users$/,
    crumbs: [{ labelKey: "nav.directory" }, { labelKey: "nav.users" }],
  },
  {
    test: /^\/roles$/,
    crumbs: [{ labelKey: "nav.directory" }, { labelKey: "nav.roles" }],
  },
  {
    test: /^\/sessions$/,
    crumbs: [{ labelKey: "nav.sessions" }],
  },
  {
    test: /^\/sessions\//,
    crumbs: [
      { labelKey: "nav.sessions", to: "/sessions" },
      { labelKey: "breadcrumb.detail" },
    ],
  },
  {
    test: /^\/tasks$/,
    crumbs: [{ labelKey: "nav.tasks" }],
  },
  {
    test: /^\/env$/,
    crumbs: [{ labelKey: "nav.systemConfig" }, { labelKey: "nav.settings" }],
  },
  {
    test: /^\/settings$/,
    crumbs: [{ labelKey: "nav.systemConfig" }, { labelKey: "nav.systemSettings" }],
  },
]

export function resolveCrumbs(pathname: string, search = ""): CrumbDef[] {
  const crumbs = ROUTES.find((r) => r.test.test(pathname))?.crumbs ?? []
  return typeof crumbs === "function" ? crumbs(new URLSearchParams(search)) : crumbs
}
