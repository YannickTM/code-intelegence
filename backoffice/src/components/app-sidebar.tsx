"use client";

import { useEffect, useRef } from "react";
import { usePathname } from "next/navigation";
import Link from "next/link";
import {
  LayoutDashboard,
  MessageSquare,
  FolderGit2,
  PanelLeft,
  SquarePen,
  Search,
} from "lucide-react";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  SidebarSeparator,
  useSidebar,
} from "~/components/ui/sidebar";
import { Logo } from "~/components/logo";
import { NavUser } from "~/components/nav-user";
import { RecentProjectsList } from "~/components/recent-projects-list";
import { RecentChatsList } from "~/components/recent-chats-list";
import { SearchDialog, useSearchDialog } from "~/components/search-dialog";

type User = {
  id: string;
  username: string;
  display_name?: string;
  avatar_url?: string;
  platform_roles?: string[];
};

const navItems = [
  { label: "Dashboard", href: "/dashboard", icon: LayoutDashboard },
] as const;

const navItemsBottom = [
  { label: "Chats", href: "/chats", icon: MessageSquare },
  { label: "Projects", href: "/project", icon: FolderGit2 },
] as const;

function isActive(href: string, pathname: string): boolean {
  if (href === "/dashboard") return pathname === "/dashboard";
  return pathname.startsWith(href);
}

function SidebarAutoCollapse() {
  const pathname = usePathname();
  const { setOpen } = useSidebar();
  const prevPathRef = useRef(pathname);

  useEffect(() => {
    const isProjectDetail =
      /^\/project\/[^/]+/.test(pathname) && pathname !== "/project";
    const wasProjectDetail = /^\/project\/[^/]+/.test(prevPathRef.current);

    if (isProjectDetail && !wasProjectDetail) {
      setOpen(false);
    }

    prevPathRef.current = pathname;
  }, [pathname, setOpen]);

  return null;
}

export function AppSidebar({ user }: { user: User | null }) {
  const pathname = usePathname();
  const { state, toggleSidebar, isMobile } = useSidebar();
  const isCollapsed = state === "collapsed";
  const showExpandedContent = isMobile || !isCollapsed;
  const { open: searchOpen, setOpen: setSearchOpen, openSearch } = useSearchDialog();

  const showRecentProjects = pathname.startsWith("/project");

  return (
    <Sidebar collapsible="icon" side="left">
      <SidebarAutoCollapse />

      <SidebarHeader>
        {isCollapsed && !isMobile ? (
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                onClick={toggleSidebar}
                tooltip="Expand Sidebar"
                aria-label="Expand sidebar"
              >
                <PanelLeft className="size-4" />
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        ) : (
          <div className="flex items-center justify-between px-2">
            <Link href="/dashboard" className="flex items-center gap-2">
              <Logo />
            </Link>
            <SidebarMenuButton
              onClick={toggleSidebar}
              tooltip="Collapse Sidebar"
              aria-label="Collapse sidebar"
              className="size-8 w-auto"
            >
              <PanelLeft className="size-4" />
            </SidebarMenuButton>
          </div>
        )}
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              {navItems.map((item) => (
                <SidebarMenuItem key={item.href}>
                  <SidebarMenuButton
                    asChild
                    isActive={isActive(item.href, pathname)}
                    tooltip={item.label}
                  >
                    <Link href={item.href}>
                      <item.icon className="size-4" />
                      <span>{item.label}</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}

              <SidebarMenuItem>
                <SidebarMenuButton
                  asChild
                  tooltip="New Chat"
                >
                  <Link href="/chats/new">
                    <SquarePen className="size-4" />
                    <span>New Chat</span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>

              <SidebarMenuItem>
                <SidebarMenuButton
                  onClick={openSearch}
                  tooltip="Search"
                >
                  <Search className="size-4" />
                  <span>Search</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarSeparator />

        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              {navItemsBottom.map((item) => (
                <SidebarMenuItem key={item.href}>
                  <SidebarMenuButton
                    asChild
                    isActive={isActive(item.href, pathname)}
                    tooltip={item.label}
                  >
                    <Link href={item.href}>
                      <item.icon className="size-4" />
                      <span>{item.label}</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        {showExpandedContent && (
          <>
            <SidebarSeparator />
            <SidebarGroup>
              <SidebarGroupLabel>Recent Chats</SidebarGroupLabel>
              <SidebarGroupContent>
                <RecentChatsList />
              </SidebarGroupContent>
            </SidebarGroup>
          </>
        )}

        {showRecentProjects && showExpandedContent && (
          <>
            <SidebarSeparator />
            <SidebarGroup>
              <SidebarGroupLabel>Recent Projects</SidebarGroupLabel>
              <SidebarGroupContent>
                <RecentProjectsList />
              </SidebarGroupContent>
            </SidebarGroup>
          </>
        )}
      </SidebarContent>

      <SidebarFooter>
        <NavUser user={user} />
      </SidebarFooter>

      <SidebarRail />

      <SearchDialog open={searchOpen} onOpenChange={setSearchOpen} />
    </Sidebar>
  );
}
