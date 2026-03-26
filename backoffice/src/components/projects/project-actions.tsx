"use client";

import { useRouter } from "next/navigation";
import {
  ExternalLink,
  Settings,
  RefreshCw,
  Pause,
  Play,
  Trash2,
  MoreHorizontal,
} from "lucide-react";
import { Button } from "~/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import type { UserProject } from "~/server/api/routers/users";

export function ProjectActions({
  project,
  onTriggerIndex,
  onToggleStatus,
  onDelete,
}: {
  project: UserProject;
  onTriggerIndex: (projectId: string) => void;
  onToggleStatus: (projectId: string, newStatus: "active" | "paused") => void;
  onDelete: (project: UserProject) => void;
}) {
  const router = useRouter();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-8 w-8 p-0"
          onClick={(e) => e.stopPropagation()}
        >
          <MoreHorizontal className="size-4" />
          <span className="sr-only">Actions</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem
          onClick={(e) => {
            e.stopPropagation();
            router.push(`/project/${project.id}`);
          }}
        >
          <ExternalLink className="size-4" />
          Open
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={(e) => {
            e.stopPropagation();
            router.push(`/project/${project.id}/settings`);
          }}
        >
          <Settings className="size-4" />
          Settings
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={(e) => {
            e.stopPropagation();
            onTriggerIndex(project.id);
          }}
        >
          <RefreshCw className="size-4" />
          Trigger Index
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={(e) => {
            e.stopPropagation();
            onToggleStatus(
              project.id,
              project.status === "active" ? "paused" : "active",
            );
          }}
        >
          {project.status === "active" ? (
            <>
              <Pause className="size-4" />
              Pause
            </>
          ) : (
            <>
              <Play className="size-4" />
              Resume
            </>
          )}
        </DropdownMenuItem>
        {project.role === "owner" && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              className="text-destructive"
              onClick={(e) => {
                e.stopPropagation();
                onDelete(project);
              }}
            >
              <Trash2 className="size-4" />
              Delete
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
