"use client";

import Link from "next/link";
import { FolderGit2 } from "lucide-react";
import { api } from "~/trpc/react";
import {
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
  SidebarMenuSkeleton,
} from "~/components/ui/sidebar";
import { usePathname } from "next/navigation";

type Project = {
  id: string;
  name: string;
  description?: string;
};

type ProjectListResponse = {
  items: Project[];
  total: number;
};

export function RecentProjectsList() {
  const pathname = usePathname();
  const projects = api.users.listMyProjects.useQuery(undefined, {
    retry: false,
  }) as {
    data: ProjectListResponse | undefined;
    isLoading: boolean;
    isError: boolean;
  };

  if (projects.isLoading) {
    return (
      <SidebarMenu>
        {Array.from({ length: 3 }).map((_, i) => (
          <SidebarMenuItem key={i}>
            <SidebarMenuSkeleton showIcon />
          </SidebarMenuItem>
        ))}
      </SidebarMenu>
    );
  }

  if (projects.isError || !projects.data?.items?.length) {
    return (
      <div className="text-muted-foreground px-2 py-1.5 text-xs">
        No projects yet
      </div>
    );
  }

  return (
    <SidebarMenu>
      {projects.data.items.slice(0, 15).map((project) => (
        <SidebarMenuItem key={project.id}>
          <SidebarMenuButton
            asChild
            isActive={
              pathname === `/project/${project.id}` ||
              pathname.startsWith(`/project/${project.id}/`)
            }
            tooltip={project.name}
          >
            <Link href={`/project/${project.id}`}>
              <FolderGit2 className="size-4" />
              <span>{project.name}</span>
            </Link>
          </SidebarMenuButton>
        </SidebarMenuItem>
      ))}
    </SidebarMenu>
  );
}
