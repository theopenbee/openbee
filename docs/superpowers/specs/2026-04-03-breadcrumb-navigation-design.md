# Breadcrumb Navigation Design

**Date:** 2026-04-03  
**Status:** Approved

## Summary

Add breadcrumb navigation to all pages in the openbee2 web app. Breadcrumbs are displayed in the header alongside the sidebar toggle, use fixed i18n text, and parent items link back to their list pages.

## Requirements

| Dimension | Decision |
|-----------|----------|
| Scope | All pages (list pages: single crumb; detail pages: two crumbs) |
| Position | Header — `[≡ toggle] | [separator] | [breadcrumb]` |
| Text | Fixed labels via i18n keys (no dynamic data fetch) |
| Interactivity | Parent crumbs are clickable links; current page crumb is not |

## Breadcrumb Structure Per Route

| Route | Breadcrumb |
|-------|-----------|
| `/` | Dashboard |
| `/workers` | Workers |
| `/workers/:id` | Workers (link) › Detail |
| `/executions` | Sessions |
| `/executions/:id` | Sessions (link) › Detail |
| `/sessions/:sessionId` | Sessions (link) › Detail |
| `/tasks` | Scheduled Tasks |
| `/local-chat` | Local Chat |
| `/local-chat/:id` | Local Chat (link) › Detail |

## Architecture

### Files

| File | Operation |
|------|-----------|
| `web/src/lib/breadcrumb-config.ts` | New — route pattern → crumb definitions map |
| `web/src/components/app-breadcrumb.tsx` | New — breadcrumb UI component |
| `web/src/components/layout.tsx` | Modified — insert `<AppBreadcrumb />` into header |
| `web/src/locales/en.json` | Modified — add `breadcrumb.detail` key |
| `web/src/locales/zh.json` | Modified — add `breadcrumb.detail` key |

### breadcrumb-config.ts

Exports a `resolveCrumbs(pathname)` function. Each entry has a `test` regex and a `crumbs` array. A crumb with `to` is rendered as a clickable link; without `to` it is the current page (non-clickable).

```ts
type CrumbDef = { labelKey: string; to?: string }
```

### app-breadcrumb.tsx

Uses `useLocation` from react-router-dom and `useTranslation` from react-i18next. Calls `resolveCrumbs(pathname)` and renders using the existing `ui/breadcrumb` components. `BreadcrumbLink` uses the `render` prop (Base UI pattern) to wrap React Router `<Link>`.

### layout.tsx change

Minimal — add `<AppBreadcrumb />` after the existing `<Separator>` in the header.

## i18n Keys Added

```json
// en.json
"breadcrumb": { "detail": "Detail" }

// zh.json
"breadcrumb": { "detail": "详情" }
```

## Constraints

- No router migration needed (keeps `<HashRouter>` + `<Routes>`)
- `BreadcrumbLink` uses Base UI `render` prop (not `asChild`) to render as React Router `<Link>`
- `localChat` has no `nav.*` key — uses `localChat.title` instead
