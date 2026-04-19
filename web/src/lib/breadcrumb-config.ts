export type CrumbDef = { labelKey: string; to?: string }

const ROUTES: { test: RegExp; crumbs: CrumbDef[] }[] = [
  {
    test: /^\/$/,
    crumbs: [{ labelKey: "nav.dashboard" }],
  },
  {
    test: /^\/workers$/,
    crumbs: [{ labelKey: "nav.workers" }],
  },
  {
    test: /^\/workers\//,
    crumbs: [
      { labelKey: "nav.workers", to: "/workers" },
      { labelKey: "breadcrumb.detail" },
    ],
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
    test: /^\/settings$/,
    crumbs: [{ labelKey: "nav.systemSettings" }],
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
]

export function resolveCrumbs(pathname: string): CrumbDef[] {
  return ROUTES.find((r) => r.test.test(pathname))?.crumbs ?? []
}
