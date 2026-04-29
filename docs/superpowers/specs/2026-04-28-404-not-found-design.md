# 404 Not Found Page — Design Spec

**Date:** 2026-04-28
**Status:** Approved

## Problem

When a user navigates to a path that does not exist in the app (e.g., `#/nonexistent`), the page renders completely blank. This is because `web/src/app.tsx` has no catch-all route (`<Route path="*" />`), and the `Suspense` fallback is `null`.

## Goal

Display a user-friendly 404 page for any unmatched route, with a context-aware CTA:
- Authenticated users → "Back to Home" (navigates to `/`)
- Unauthenticated users → "Go to Login" (navigates to `/login`)

## Approach

Add a `NotFound` page component and register it as the catch-all route outside of `AuthGuard`, so it is reachable regardless of authentication state.

## Architecture

### Files Changed

| File | Change |
|------|--------|
| `web/src/pages/not-found.tsx` | New — 404 page component |
| `web/src/app.tsx` | Modified — add catch-all route + lazy import |
| `web/src/locales/zh.json` | Modified — add `notFound` i18n keys |
| `web/src/locales/en.json` | Modified — add `notFound` i18n keys |

### Route Structure

```tsx
<Routes>
  <Route path="/login" element={<Login />} />
  <Route element={<AuthGuard><Layout /></AuthGuard>}>
    {/* all existing protected routes unchanged */}
  </Route>
  {/* NEW: catch-all, outside AuthGuard, last in list */}
  <Route path="*" element={<NotFound />} />
</Routes>
```

The catch-all route must be the last `<Route>` inside `<Routes>` so it only matches when no other route does.

## Component: `NotFound`

**Location:** `web/src/pages/not-found.tsx`

**Auth detection:** Calls `getAccessToken()` from `@/lib/auth` — the same function used by `AuthGuard`. No new hooks or context required.

**Rendering:**
- Full-screen centered layout (no sidebar, no top bar — does not use `Layout`)
- Shows the 404 code, a title, and a short description
- A single `Button` (shadcn/ui) whose label and destination depend on auth state:
  - Token present → label: `notFound.backToHome`, destination: `/`
  - No token → label: `notFound.goToLogin`, destination: `/login`

**Styling:** Tailwind semantic colors (`text-muted-foreground`, etc.) for light/dark theme compatibility. No animations, no illustrations.

**Lazy loading:** Loaded via `React.lazy()` consistent with all other page components in `app.tsx`.

## i18n

### `zh.json`

```json
"notFound": {
  "code": "404",
  "title": "页面不存在",
  "description": "你访问的路径不存在或已被移除",
  "backToHome": "返回首页",
  "goToLogin": "去登录"
}
```

### `en.json`

```json
"notFound": {
  "code": "404",
  "title": "Page Not Found",
  "description": "The page you visited doesn't exist or has been removed.",
  "backToHome": "Back to Home",
  "goToLogin": "Go to Login"
}
```

## Error Handling

No additional error handling is needed. The component has no async operations or external dependencies. `getAccessToken()` is a synchronous local read (same guarantee `AuthGuard` relies on).

## Testing

Manual verification steps:
1. While logged in, navigate to `#/does-not-exist` → should see 404 page with "Back to Home" button
2. While logged out, navigate to `#/does-not-exist` → should see 404 page with "Go to Login" button
3. Click the CTA button → should navigate to the correct destination
4. Verify all existing routes still work correctly (no regression)

No unit tests required — component has no logic beyond a ternary on `getAccessToken()`.

## Out of Scope

- Runtime error boundary (React ErrorBoundary for render crashes) — separate concern
- Custom 404 illustrations or animations
- Server-side 404 handling (app uses HashRouter, server always serves `index.html`)
