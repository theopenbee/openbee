import * as React from "react"
import { Link, useLocation } from "react-router-dom"
import { ChevronDownIcon } from "lucide-react"

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from "@/components/ui/sidebar"

// A leaf is a direct link (icon + label). A group is a collapsible header
// (icon + label + chevron) whose children are plain, indented text links.
// An optional `perm` marks the entry as permission-gated; gating itself is
// applied by the caller (see app-sidebar) before items reach this component.
export type NavSubItem = { title: string; url: string; perm?: string; section?: string }

export type NavLeaf = {
  title: string
  url: string
  icon: React.ReactNode
  perm?: string
}

export type NavGroupItem = {
  title: string
  icon: React.ReactNode
  perm?: string
  items: NavSubItem[]
}

export type NavEntry = NavLeaf | NavGroupItem

function isGroup(item: NavEntry): item is NavGroupItem {
  return "items" in item
}

export function NavMain({
  items,
  label,
}: {
  label?: string
  items: NavEntry[]
}) {
  const { pathname } = useLocation()

  const isActive = (url: string) =>
    url === "/" ? pathname === "/" : pathname.startsWith(url)

  return (
    <SidebarGroup>
      {label && <SidebarGroupLabel>{label}</SidebarGroupLabel>}
      <SidebarMenu className="gap-1">
        {items.map((item) =>
          isGroup(item) ? (
            <Collapsible
              key={item.title}
              // Multiple groups can stay open independently; the group holding
              // the current route opens by default.
              defaultOpen={item.items.some((sub) => isActive(sub.url))}
              render={<SidebarMenuItem />}
            >
              <CollapsibleTrigger
                render={
                  <SidebarMenuButton
                    className="group/collapsible h-9"
                    tooltip={item.title}
                  >
                    {item.icon}
                    <span>{item.title}</span>
                    <ChevronDownIcon className="ml-auto text-sidebar-foreground/60 transition-transform duration-200 group-data-[panel-open]/collapsible:rotate-180" />
                  </SidebarMenuButton>
                }
              />
              <CollapsibleContent>
                <SidebarMenuSub className="mx-0 gap-1 border-l-0 px-0 pl-[2.125rem]">
                  {item.items.map((sub, i) => {
                    // Render a small section header whenever this sub-item's
                    // section differs from the previous (already permission-
                    // filtered) one. Sub-items without a section render plain.
                    const showSection =
                      sub.section && sub.section !== item.items[i - 1]?.section
                    return (
                      <React.Fragment key={sub.title}>
                        {showSection && (
                          <SidebarGroupLabel className="h-7 px-0 text-sidebar-foreground/60">
                            {sub.section}
                          </SidebarGroupLabel>
                        )}
                        <SidebarMenuSubItem>
                          <SidebarMenuSubButton
                            isActive={isActive(sub.url)}
                            className="h-8 data-active:bg-transparent data-active:font-medium data-active:text-sidebar-primary"
                            render={<Link to={sub.url} />}
                          >
                            <span>{sub.title}</span>
                          </SidebarMenuSubButton>
                        </SidebarMenuSubItem>
                      </React.Fragment>
                    )
                  })}
                </SidebarMenuSub>
              </CollapsibleContent>
            </Collapsible>
          ) : (
            <SidebarMenuItem key={item.title}>
              <SidebarMenuButton
                className="h-9 data-active:bg-transparent data-active:text-sidebar-primary [&_svg]:data-active:text-sidebar-primary"
                tooltip={item.title}
                isActive={isActive(item.url)}
                render={<Link to={item.url} />}
              >
                {item.icon}
                <span>{item.title}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          )
        )}
      </SidebarMenu>
    </SidebarGroup>
  )
}
