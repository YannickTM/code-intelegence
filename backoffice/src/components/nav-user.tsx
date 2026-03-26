"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import {
  ChevronsUpDown,
  KeyRound,
  LogOut,
  Loader2,
  Settings,
  Shield,
  User as UserIcon,
} from "lucide-react";
import { Avatar, AvatarFallback, AvatarImage } from "~/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "~/components/ui/sidebar";
import { ThemeToggle } from "~/components/theme-toggle";
import Link from "next/link";

type User = {
  id: string;
  username: string;
  display_name?: string;
  avatar_url?: string;
  platform_roles?: string[];
};

function getInitials(user: User): string {
  const name = user.display_name ?? user.username;
  return name
    .split(/[\s_-]+/)
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase() ?? "")
    .join("");
}

export function NavUser({ user }: { user: User | null }) {
  const { isMobile } = useSidebar();
  const router = useRouter();
  const [isLoggingOut, setIsLoggingOut] = useState(false);

  async function handleLogout() {
    setIsLoggingOut(true);
    try {
      await fetch("/api/auth/logout", { method: "POST" });
    } catch {
      // continue to redirect even if logout call fails
    }
    router.push("/login");
  }

  if (!user) return null;

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton
              size="lg"
              className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
              tooltip={user.display_name ?? user.username}
            >
              <Avatar className="size-8 rounded-lg">
                <AvatarImage
                  src={user.avatar_url}
                  alt={user.display_name ?? user.username}
                />
                <AvatarFallback className="rounded-lg">
                  {getInitials(user)}
                </AvatarFallback>
              </Avatar>
              <div className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-medium">
                  {user.display_name ?? user.username}
                </span>
                {user.display_name && (
                  <span className="text-muted-foreground truncate text-xs">
                    @{user.username}
                  </span>
                )}
              </div>
              <ChevronsUpDown className="ml-auto size-4" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="w-60 rounded-lg"
            side={isMobile ? "bottom" : "top"}
            align="end"
            sideOffset={4}
          >
            <DropdownMenuLabel className="p-0 font-normal">
              <div className="flex items-center gap-2 px-1 py-1.5 text-left text-sm">
                <Avatar className="size-8 rounded-lg">
                  <AvatarImage
                    src={user.avatar_url}
                    alt={user.display_name ?? user.username}
                  />
                  <AvatarFallback className="rounded-lg">
                    {getInitials(user)}
                  </AvatarFallback>
                </Avatar>
                <div className="grid flex-1 text-left text-sm leading-tight">
                  <span className="truncate font-medium">
                    {user.display_name ?? user.username}
                  </span>
                  <span className="text-muted-foreground truncate text-xs">
                    @{user.username}
                  </span>
                </div>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem asChild>
                <Link href="/settings/profile">
                  <UserIcon className="size-4" />
                  Profile
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Link href="/settings/ssh-keys">
                  <KeyRound className="size-4" />
                  SSH Keys
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Link href="/settings/system">
                  <Settings className="size-4" />
                  System Settings
                </Link>
              </DropdownMenuItem>
              {user.platform_roles?.includes("platform_admin") && (
                <DropdownMenuItem asChild>
                  <Link href="/platform-settings">
                    <Shield className="size-4" />
                    Platform Settings
                  </Link>
                </DropdownMenuItem>
              )}
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <ThemeToggle />
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={handleLogout} disabled={isLoggingOut}>
              {isLoggingOut ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <LogOut className="size-4" />
              )}
              Sign out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
