# Sidebar Layout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the top horizontal navigation bar with a collapsible left sidebar layout featuring breadcrumbs, a user menu, and theme toggle.

**Architecture:** Install shadcn Sidebar primitives only (not sidebar-08 demo). Hand-write `AppSidebar` and update `Layout`. Auth layer gains username persistence for the user menu. Breadcrumbs are derived from `useLocation` via a static route map.

**Tech Stack:** React, React Router DOM, shadcn/ui (Sidebar, Breadcrumb, DropdownMenu), Lucide icons, TypeScript

**Spec:** `docs/superpowers/specs/2026-04-02-sidebar-layout-design.md`

---

## File Map

| File | Action |
|---|---|
| `web/src/lib/auth.ts` | Modify — add `saveUsername`, `getUsername`, update `clearTokens` |
| `web/src/pages/login.tsx` | Modify — call `saveUsername` on successful login |
| `web/src/components/ui/sidebar.tsx` | Create via `npx shadcn add sidebar` |
| `web/src/components/ui/breadcrumb.tsx` | Create via `npx shadcn add breadcrumb` |
| `web/src/components/ui/dropdown-menu.tsx` | Create via `npx shadcn add dropdown-menu` |
| `web/src/components/app-sidebar.tsx` | Create — main sidebar component |
| `web/src/components/layout.tsx` | Modify — SidebarProvider + SidebarInset + breadcrumbs |
| `web/src/components/nav.tsx` | Delete — replaced by AppSidebar |
| `web/src/components/theme-switcher.tsx` | Keep as-is (reused inside AppSidebar) |

---

## Task 1: Add username persistence to auth layer

**Files:**
- Modify: `web/src/lib/auth.ts`
- Modify: `web/src/pages/login.tsx`

- [ ] **Step 1: Add `saveUsername` and `getUsername` to `auth.ts`**

Open `web/src/lib/auth.ts`. Add a `USERNAME_KEY` constant and two functions after the existing token constants. Also update `clearTokens` to remove the username:

```typescript
// After the existing token key constants, add:
const USERNAME_KEY = "openbee_username"

export function saveUsername(username: string): void {
  localStorage.setItem(USERNAME_KEY, username)
}

export function getUsername(): string {
  return localStorage.getItem(USERNAME_KEY) ?? "Admin"
}
```

And update `clearTokens`:

```typescript
export function clearTokens(): void {
  localStorage.removeItem(ACCESS_TOKEN_KEY)
  localStorage.removeItem(REFRESH_TOKEN_KEY)
  localStorage.removeItem(USERNAME_KEY)
}
```

- [ ] **Step 2: Call `saveUsername` on successful login in `login.tsx`**

Open `web/src/pages/login.tsx`. Add `saveUsername` to the import:

```typescript
import { login, saveUsername } from "@/lib/auth"
```

In `handleSubmit`, call `saveUsername(username)` before `navigate`:

```typescript
if (result.success) {
  saveUsername(username)
  navigate("/", { replace: true })
}
```

- [ ] **Step 3: Verify manually**

Start the dev server: `cd web && pnpm dev`

Log in. Open DevTools → Application → Local Storage. Confirm `openbee_username` key is set to the username you entered.

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/auth.ts web/src/pages/login.tsx
git commit -m "feat(web): persist username to localStorage on login"
```

---

## Task 2: Install shadcn UI components

**Files:**
- Create: `web/src/components/ui/sidebar.tsx`
- Create: `web/src/components/ui/breadcrumb.tsx`
- Create: `web/src/components/ui/dropdown-menu.tsx`
- Create: `web/src/components/ui/separator.tsx` (installed as sidebar dep)
- Create: `web/src/components/ui/tooltip.tsx` (installed as sidebar dep)
- Create: `web/src/components/ui/sheet.tsx` (installed as sidebar dep)
- Create: `web/src/components/ui/skeleton.tsx` (installed as sidebar dep)

- [ ] **Step 1: Install Sidebar primitives**

```bash
cd web && npx shadcn@latest add sidebar
```

Accept all prompts. This installs `sidebar.tsx` and its dependencies (separator, tooltip, sheet, skeleton).

- [ ] **Step 2: Install Breadcrumb component**

```bash
cd web && npx shadcn@latest add breadcrumb
```

- [ ] **Step 3: Install DropdownMenu component**

```bash
cd web && npx shadcn@latest add dropdown-menu
```

- [ ] **Step 4: Verify installation**

```bash
ls web/src/components/ui/
```

Expected to see at minimum: `sidebar.tsx`, `breadcrumb.tsx`, `dropdown-menu.tsx`, `separator.tsx`, `tooltip.tsx`

- [ ] **Step 5: Confirm the app still compiles**

```bash
cd web && pnpm build 2>&1 | tail -5
```

Expected: build succeeds (exit 0).

- [ ] **Step 6: Commit**

```bash
git add web/src/components/ui/
git commit -m "feat(web): install shadcn sidebar, breadcrumb, dropdown-menu primitives"
```

---

## Task 3: Create AppSidebar component

**Files:**
- Create: `web/src/components/app-sidebar.tsx`

- [ ] **Step 1: Create `app-sidebar.tsx`**

Create `web/src/components/app-sidebar.tsx` with the following content:

```tsx
import { Link, useLocation, useNavigate } from "react-router-dom"
import { useState } from "react"
import {
  LayoutDashboard, Bot, Activity, Clock, MessageCircle,
  LogOut, Sun, Moon,
} from "lucide-react"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { clearTokens, getUsername } from "@/lib/auth"
import { cn } from "@/lib/utils"

type Theme = "dark" | "light"

function getStoredTheme(): Theme {
  const stored = localStorage.getItem("theme")
  return stored === "light" ? "light" : "dark"
}

const NAV_ITEMS = [
  { href: "/", label: "Dashboard", icon: LayoutDashboard, exact: true },
  { href: "/workers", label: "Workers", icon: Bot, exact: false },
  { href: "/executions", label: "Sessions", icon: Activity, exact: false },
  { href: "/tasks", label: "Scheduled Tasks", icon: Clock, exact: false },
  { href: "/local-chat", label: "Local Chat", icon: MessageCircle, exact: false },
]

export function AppSidebar() {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const username = getUsername()
  const [theme, setTheme] = useState<Theme>(getStoredTheme)

  function isActive(href: string, exact: boolean) {
    if (exact) return pathname === href
    return pathname === href || pathname.startsWith(href + "/")
  }

  function toggleTheme() {
    const next: Theme = theme === "dark" ? "light" : "dark"
    document.documentElement.classList.remove("dark", "light")
    document.documentElement.classList.add(next)
    localStorage.setItem("theme", next)
    setTheme(next)
  }

  function handleLogout() {
    clearTokens()
    navigate("/login", { replace: true })
  }

  const initial = username[0]?.toUpperCase() ?? "A"

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild>
              <Link to="/">
                <svg
                  width="24"
                  height="24"
                  viewBox="0 0 24 24"
                  fill="none"
                  className="shrink-0 text-primary"
                >
                  <path
                    d="M12 2L20 7V17L12 22L4 17V7L12 2Z"
                    stroke="currentColor"
                    strokeWidth="1.5"
                    fill="currentColor"
                    fillOpacity="0.15"
                  />
                </svg>
                <span className="text-base font-bold tracking-tight">OpenBee</span>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              {NAV_ITEMS.map(({ href, label, icon: Icon, exact }) => (
                <SidebarMenuItem key={href}>
                  <SidebarMenuButton
                    asChild
                    isActive={isActive(href, exact)}
                    tooltip={label}
                  >
                    <Link to={href}>
                      <Icon />
                      <span>{label}</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              onClick={toggleTheme}
              tooltip={theme === "dark" ? "Light mode" : "Dark mode"}
            >
              {theme === "dark" ? <Sun /> : <Moon />}
              <span>{theme === "dark" ? "Light mode" : "Dark mode"}</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
          <SidebarMenuItem>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <SidebarMenuButton tooltip={username}>
                  <div
                    className={cn(
                      "flex h-6 w-6 shrink-0 items-center justify-center",
                      "rounded-full bg-primary/20 text-xs font-semibold text-primary"
                    )}
                  >
                    {initial}
                  </div>
                  <span className="truncate font-medium">{username}</span>
                </SidebarMenuButton>
              </DropdownMenuTrigger>
              <DropdownMenuContent side="top" align="start" className="w-48">
                <DropdownMenuItem onClick={handleLogout}>
                  <LogOut className="mr-2 h-4 w-4" />
                  Logout
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  )
}
```

- [ ] **Step 2: Confirm TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors (or only pre-existing errors unrelated to app-sidebar.tsx).

- [ ] **Step 3: Commit**

```bash
git add web/src/components/app-sidebar.tsx
git commit -m "feat(web): add AppSidebar component with nav, theme toggle, and user menu"
```

---

## Task 4: Update Layout and remove Nav

**Files:**
- Modify: `web/src/components/layout.tsx`
- Delete: `web/src/components/nav.tsx`

- [ ] **Step 1: Replace `layout.tsx`**

Replace the entire content of `web/src/components/layout.tsx` with:

```tsx
import { Outlet, Link, useLocation } from "react-router-dom"
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
  label: string
  href?: string
}

function getBreadcrumbs(pathname: string): BreadcrumbSegment[] {
  if (pathname === "/") return [{ label: "Dashboard" }]
  if (pathname === "/workers") return [{ label: "Workers" }]
  if (/^\/workers\/[^/]+/.test(pathname))
    return [{ label: "Workers", href: "/workers" }, { label: "Worker Detail" }]
  if (pathname === "/executions") return [{ label: "Sessions" }]
  if (/^\/executions\/[^/]+/.test(pathname))
    return [{ label: "Sessions", href: "/executions" }, { label: "Session Detail" }]
  if (/^\/sessions\/[^/]+/.test(pathname))
    return [{ label: "Sessions", href: "/executions" }, { label: "Session Detail" }]
  if (pathname === "/tasks") return [{ label: "Scheduled Tasks" }]
  if (pathname === "/local-chat") return [{ label: "Local Chat" }]
  if (/^\/local-chat\/[^/]+/.test(pathname))
    return [{ label: "Local Chat", href: "/local-chat" }, { label: "Chat Detail" }]
  return [{ label: "Dashboard" }]
}

export function Layout() {
  const { pathname } = useLocation()
  const crumbs = getBreadcrumbs(pathname)

  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <header className="flex h-14 shrink-0 items-center gap-2 border-b px-4">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-2 h-4" />
          <Breadcrumb>
            <BreadcrumbList>
              {crumbs.map((crumb, i) => (
                <span key={i} className="flex items-center gap-1.5">
                  {i > 0 && <BreadcrumbSeparator />}
                  <BreadcrumbItem>
                    {crumb.href ? (
                      <BreadcrumbLink asChild>
                        <Link to={crumb.href}>{crumb.label}</Link>
                      </BreadcrumbLink>
                    ) : (
                      <BreadcrumbPage>{crumb.label}</BreadcrumbPage>
                    )}
                  </BreadcrumbItem>
                </span>
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
```

- [ ] **Step 2: Delete `nav.tsx`**

```bash
rm web/src/components/nav.tsx
```

- [ ] **Step 3: Confirm TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors related to the new files. If `ThemeSwitcher` is now unused elsewhere, remove its import if it causes an error (it was only used in nav.tsx).

- [ ] **Step 4: Start dev server and verify visually**

```bash
cd web && pnpm dev
```

Open the app in a browser. Verify:
- Left sidebar appears with OpenBee logo, 5 nav links, theme toggle, and user menu at the bottom
- Active nav link is highlighted as you navigate between pages
- Breadcrumb in the header updates correctly for each route
- Sidebar collapses to icon-only mode when the SidebarTrigger button is clicked
- Theme toggle works (switches dark/light)
- User menu dropdown shows "Logout" option; clicking it redirects to login page
- Username in the user menu matches what was entered at login

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout.tsx
git rm web/src/components/nav.tsx
git commit -m "feat(web): replace top nav with sidebar layout (sidebar-08 inspired)"
```

---

## Self-Review Against Spec

| Spec requirement | Task |
|---|---|
| Left collapsible sidebar replaces top nav | Task 4 (Layout), Task 3 (AppSidebar) |
| Sidebar collapses to icon-only (`collapsible="icon"`) | Task 3, AppSidebar prop |
| Breadcrumb navigation in content header | Task 4, Layout breadcrumb |
| Bottom user menu with username + logout | Task 3, AppSidebar footer |
| Theme switcher preserved | Task 3, AppSidebar footer |
| Username from localStorage (fallback "Admin") | Task 1, `getUsername()` |
| `saveUsername` on login | Task 1, login.tsx |
| `clearTokens` also clears username | Task 1, auth.ts |
| Nav.tsx deleted | Task 4 |
| No sidebar-08 demo files in project | ✅ (only `npx shadcn add sidebar` used) |

All requirements covered.
