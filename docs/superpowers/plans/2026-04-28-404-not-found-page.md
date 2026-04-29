# 404 Not Found Page — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a user-friendly 404 page that displays when a user visits an unmatched route, with an auth-aware CTA.

**Architecture:** Add a `NotFound` page component that calls `getAccessToken()` to determine auth state, then register it as a catch-all `<Route path="*">` outside `AuthGuard` in `app.tsx`. No new hooks, no new context — one new file and three small edits.

**Tech Stack:** React 19, React Router DOM v7, react-i18next, shadcn/ui Button, Tailwind CSS 4, TypeScript

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `web/src/pages/not-found.tsx` | Create | 404 page UI + auth-aware CTA |
| `web/src/app.tsx` | Modify | Lazy import + catch-all route |
| `web/src/locales/zh.json` | Modify | Chinese i18n keys for 404 page |
| `web/src/locales/en.json` | Modify | English i18n keys for 404 page |

---

## Task 1: Add i18n keys

**Files:**
- Modify: `web/src/locales/zh.json`
- Modify: `web/src/locales/en.json`

> No unit tests — these are static JSON files verified by the component in Task 2.

- [ ] **Step 1: Add Chinese keys**

Open `web/src/locales/zh.json`. Add the following block before the final closing `}` (after the `"systemSettings"` block):

```json
  "notFound": {
    "code": "404",
    "title": "页面不存在",
    "description": "你访问的路径不存在或已被移除",
    "backToHome": "返回首页",
    "goToLogin": "去登录"
  }
```

- [ ] **Step 2: Add English keys**

Open `web/src/locales/en.json`. Add the following block before the final closing `}` (after the `"systemSettings"` block):

```json
  "notFound": {
    "code": "404",
    "title": "Page Not Found",
    "description": "The page you visited doesn't exist or has been removed.",
    "backToHome": "Back to Home",
    "goToLogin": "Go to Login"
  }
```

- [ ] **Step 3: Commit**

```bash
git add web/src/locales/zh.json web/src/locales/en.json
git commit -m "feat: add i18n keys for 404 not-found page"
```

---

## Task 2: Create NotFound page component

**Files:**
- Create: `web/src/pages/not-found.tsx`

> The component has no async logic — auth detection is a single synchronous ternary on `getAccessToken()`. Unit tests would only test React rendering plumbing, not real logic. Manual verification in Task 3 covers correctness.

- [ ] **Step 1: Create the component**

Create `web/src/pages/not-found.tsx` with the following content:

```tsx
import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { Button } from "@/components/ui/button"
import { getAccessToken } from "@/lib/auth"

export function NotFound() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isAuthenticated = Boolean(getAccessToken())

  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-4 text-center">
      <p className="text-8xl font-bold text-muted-foreground">{t("notFound.code")}</p>
      <h1 className="text-2xl font-semibold">{t("notFound.title")}</h1>
      <p className="text-muted-foreground">{t("notFound.description")}</p>
      <Button onClick={() => navigate(isAuthenticated ? "/" : "/login")}>
        {isAuthenticated ? t("notFound.backToHome") : t("notFound.goToLogin")}
      </Button>
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/pages/not-found.tsx
git commit -m "feat: add NotFound page component"
```

---

## Task 3: Wire catch-all route in app.tsx

**Files:**
- Modify: `web/src/app.tsx`

- [ ] **Step 1: Add lazy import**

In `web/src/app.tsx`, after the last existing `lazy(...)` import line (currently `SystemSettings`), add:

```tsx
const NotFound = lazy(() => import("@/pages/not-found").then(m => ({ default: m.NotFound })))
```

- [ ] **Step 2: Add catch-all route**

In `web/src/app.tsx`, inside `<Routes>`, add the catch-all as the **last** route — after the closing `</Route>` tag of the `AuthGuard`-wrapped group and before `</Routes>`:

```tsx
<Route path="*" element={<NotFound />} />
```

The full `<Routes>` block should now look like:

```tsx
<Routes>
  <Route path="/login" element={<Login />} />
  <Route element={<AuthGuard><Layout /></AuthGuard>}>
    <Route path="/" element={<Dashboard />} />
    <Route path="/workers" element={<Workers />} />
    <Route path="/workers/:id" element={<WorkerDetail />} />
    <Route path="/departments" element={<Departments />} />
    <Route path="/sessions" element={<Sessions />} />
    <Route path="/sessions/detail" element={<SessionDetail />} />
    <Route path="/tasks" element={<Tasks />} />
    <Route path="/chat" element={<LocalChat />} />
    <Route path="/env" element={<Env />} />
    <Route path="/settings" element={<SystemSettings />} />
  </Route>
  <Route path="*" element={<NotFound />} />
</Routes>
```

- [ ] **Step 3: Commit**

```bash
git add web/src/app.tsx
git commit -m "feat: add catch-all 404 route"
```

---

## Task 4: Manual verification

**Run the dev server:**

```bash
cd web && npm run dev
```

Expected output: Vite dev server starts on `http://localhost:5173` (or similar port).

- [ ] **Step 1: Test — logged-in user visits unknown route**

1. Log in via the UI
2. Navigate to `http://localhost:5173/#/does-not-exist`
3. Expected: 404 page shows "页面不存在" / "Page Not Found" with a **返回首页 / Back to Home** button
4. Click the button → should navigate to `/#/` (Dashboard)

- [ ] **Step 2: Test — logged-out user visits unknown route**

1. Log out (or clear `openbee_access_token` from localStorage via DevTools)
2. Navigate to `http://localhost:5173/#/does-not-exist`
3. Expected: 404 page shows with a **去登录 / Go to Login** button
4. Click the button → should navigate to `/#/login`

- [ ] **Step 3: Regression — existing routes still work**

Navigate to each of these and verify they render normally:
- `/#/` → Dashboard
- `/#/workers` → Workers list
- `/#/login` → Login page (when logged out)

- [ ] **Step 4: TypeScript check**

```bash
cd web && npm run build
```

Expected: Build completes with no TypeScript errors.
