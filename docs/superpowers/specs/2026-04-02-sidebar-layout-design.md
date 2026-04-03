# Sidebar Layout Design

**Date:** 2026-04-02  
**Status:** Approved

## Overview

Replace the current top horizontal navigation bar in the `web/` project with a left sidebar layout inspired by shadcn's sidebar-08 block. The sidebar-08 block itself is NOT installed into the project — only the shadcn Sidebar base primitives are installed, and the application-level components are hand-written.

## Requirements

- Left collapsible sidebar replaces the top navigation bar entirely
- Sidebar collapses to icon-only mode (collapsible="icon")
- Breadcrumb navigation in the content area header
- Bottom user menu with username display and logout
- Theme switcher preserved in the sidebar footer

## Architecture

### Component Changes

| File | Action | Notes |
|---|---|---|
| `components/ui/sidebar.tsx` | Create (via shadcn) | `npx shadcn add sidebar` — base primitives only |
| `components/app-sidebar.tsx` | Create | Main sidebar component |
| `components/layout.tsx` | Modify | SidebarProvider + SidebarInset structure |
| `components/nav.tsx` | Delete | Replaced by AppSidebar |
| `lib/auth.ts` | Modify | Add username save/clear to localStorage |
| `pages/login.tsx` | Modify | Save username on successful login |

### Layout Structure

```
SidebarProvider
├── AppSidebar (left, fixed width, collapsible)
│   ├── SidebarHeader: OpenBee logo + name
│   ├── SidebarContent: Main navigation links
│   └── SidebarFooter: Theme switcher + User menu
└── SidebarInset (right, responsive main area)
    ├── Header: SidebarTrigger + Breadcrumb
    └── main: <Outlet /> (page content)
```

## AppSidebar Component

### Navigation Links

| Label | Icon | Route |
|---|---|---|
| Dashboard | LayoutDashboard | `/` |
| Workers | Bot | `/workers` |
| Sessions | Activity | `/executions` |
| Scheduled Tasks | Clock | `/tasks` |
| Local Chat | MessageCircle | `/local-chat` |

Active state determined by `useLocation` — exact match for `/`, prefix match for all others (same logic as the existing Nav).

### Sidebar Footer

**Theme switcher:** Reuse existing `ThemeSwitcher` component logic (Sun/Moon toggle, persists to localStorage).

**User menu (DropdownMenu):**
- Display: avatar placeholder (first letter of username) + username
- Username source: `localStorage.getItem("openbee_username")` — falls back to `"Admin"` if absent
- Dropdown item: "Logout" → `clearTokens()` + `clearUsername()` + redirect to `/login`

## Breadcrumb Navigation

Placed in the `SidebarInset` header, between `SidebarTrigger` and the right edge.

### Route-to-Breadcrumb Mapping

| Path pattern | Breadcrumb |
|---|---|
| `/` | Dashboard |
| `/workers` | Workers |
| `/workers/:id` | Workers › Worker Detail |
| `/executions` | Sessions |
| `/executions/:id` | Sessions › Session Detail |
| `/sessions/:sessionId` | Sessions › Session Detail |
| `/tasks` | Scheduled Tasks |
| `/local-chat` | Local Chat |
| `/local-chat/:id` | Local Chat › Chat Detail |

**Implementation:** A route-to-segments map in `layout.tsx`. `useLocation` reads the current path, matches against the map, and renders shadcn `Breadcrumb` + `BreadcrumbItem` + `BreadcrumbSeparator`. Dynamic segments (`:id`) use fixed display text — no API calls needed.

## Auth Changes

### `lib/auth.ts`

Add two functions:
- `saveUsername(username: string)` — `localStorage.setItem("openbee_username", username)`
- `getUsername(): string` — returns stored username, falls back to `"Admin"`
- Update `clearTokens()` to also remove `openbee_username`

### `pages/login.tsx`

On successful login, call `saveUsername(username)` before navigating to `/`.

## Sidebar Collapse Behavior

Uses shadcn Sidebar's built-in `collapsible="icon"` prop on the `<Sidebar>` component:
- Expanded: icons + labels visible
- Collapsed: icons only, labels hidden
- Toggle: `SidebarTrigger` button in the content header
- State persisted by shadcn's `SidebarProvider` via cookie (`sidebar_state`)

## Out of Scope

- Mobile responsive drawer behavior (shadcn Sidebar handles this automatically)
- Fetching the username from an API endpoint (no `/me` endpoint exists)
- Any changes to page content (only layout shell changes)
