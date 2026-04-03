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
    test: /^\/executions$/,
    crumbs: [{ labelKey: "nav.executions" }],
  },
  {
    test: /^\/executions\//,
    crumbs: [
      { labelKey: "nav.executions", to: "/executions" },
      { labelKey: "breadcrumb.detail" },
    ],
  },
  {
    test: /^\/sessions\//,
    crumbs: [
      { labelKey: "nav.executions", to: "/executions" },
      { labelKey: "breadcrumb.detail" },
    ],
  },
  {
    test: /^\/tasks$/,
    crumbs: [{ labelKey: "nav.tasks" }],
  },
  {
    test: /^\/local-chat$/,
    crumbs: [{ labelKey: "localChat.title" }],
  },
  {
    test: /^\/local-chat\//,
    crumbs: [
      { labelKey: "localChat.title", to: "/local-chat" },
      { labelKey: "breadcrumb.detail" },
    ],
  },
]

export function resolveCrumbs(pathname: string): CrumbDef[] {
  return ROUTES.find((r) => r.test.test(pathname))?.crumbs ?? []
}
