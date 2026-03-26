"use client";

import Link from "next/link";
import {
  ExternalLink,
  ChevronDown,
  Copy,
  Pause,
  Play,
  Settings,
  MoreHorizontal,
} from "lucide-react";
import { toast } from "sonner";
import { cn } from "~/lib/utils";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import type { ProjectWithHealth } from "~/lib/dashboard-types";
import {
  getProjectHealthStatus,
  getStatusDotColor,
  getHealthLabel,
} from "~/lib/health-utils";
import { useProjectDetailMutations } from "~/hooks/use-project-detail-mutations";

export function ProjectDetailHeader({
  project,
  projectId,
}: {
  project: ProjectWithHealth;
  projectId: string;
}) {
  const { triggerIndex, updateProject } = useProjectDetailMutations(projectId);
  const isAdminOrOwner = project.role === "admin" || project.role === "owner";
  const healthStatus = getProjectHealthStatus(project);
  const dotColor = getStatusDotColor(healthStatus);
  const healthLabel = getHealthLabel(healthStatus);
  const isIndexing = healthStatus === "indexing";

  const statusLabel = project.status === "active" ? "Active" : "Paused";
  const statusClass =
    project.status === "active"
      ? "bg-success/10 text-success"
      : "bg-warning/10 text-warning";

  return (
    <div className="flex flex-col gap-3 pb-4">
      <div className="flex items-start justify-between gap-4">
        {/* Left: name, repo, metadata */}
        <div className="flex min-w-0 flex-col gap-1">
          <h1 className="truncate text-2xl font-bold tracking-tight">
            {project.name}
          </h1>
          <div className="text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
            <a
              href={project.repo_url}
              target="_blank"
              rel="noopener noreferrer"
              className="hover:text-foreground inline-flex items-center gap-1"
            >
              {project.repo_url}
              <ExternalLink className="size-3 shrink-0" />
            </a>
            <span>&middot;</span>
            <Badge variant="outline" className="font-normal">
              {project.index_branch ?? project.default_branch}
            </Badge>
            {project.index_git_commit && (
              <>
                <span>&middot;</span>
                <code className="font-mono text-xs">
                  {project.index_git_commit.slice(0, 7)}
                </code>
              </>
            )}
          </div>
          <div className="mt-1 flex items-center gap-2">
            <Badge variant="secondary" className={statusClass}>
              {statusLabel}
            </Badge>
            <span className="flex items-center gap-1.5 text-sm">
              <span className={cn("size-2 rounded-full", dotColor)} />
              {healthLabel}
            </span>
          </div>
        </div>

        {/* Right: trigger index + overflow */}
        <div className="flex shrink-0 items-center gap-2">
          {/* Split button: Trigger Index */}
          <div className="inline-flex items-center rounded-md">
            <Button
              size="sm"
              className="rounded-r-none"
              onClick={() =>
                triggerIndex.mutate({ projectId, job_type: "incremental" })
              }
              disabled={triggerIndex.isPending || isIndexing}
            >
              Trigger Index
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  size="sm"
                  className="rounded-l-none border-l px-2"
                  disabled={triggerIndex.isPending || isIndexing}
                >
                  <ChevronDown className="size-3" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem
                  disabled={isIndexing || triggerIndex.isPending}
                  onClick={() =>
                    triggerIndex.mutate({ projectId, job_type: "incremental" })
                  }
                >
                  Incremental Index
                </DropdownMenuItem>
                <DropdownMenuItem
                  disabled={isIndexing || triggerIndex.isPending}
                  onClick={() =>
                    triggerIndex.mutate({ projectId, job_type: "full" })
                  }
                >
                  Full Index
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>

          {/* Overflow menu */}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="icon" className="size-8">
                <MoreHorizontal className="size-4" />
                <span className="sr-only">More actions</span>
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {isAdminOrOwner && (
                <DropdownMenuItem
                  disabled={updateProject.isPending}
                  onClick={() => {
                    const newStatus =
                      project.status === "active" ? "paused" : "active";
                    updateProject.mutate({ id: projectId, status: newStatus });
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
              )}
              <DropdownMenuItem
                onClick={() => {
                  void navigator.clipboard.writeText(project.repo_url);
                  toast.success("Repo URL copied");
                }}
              >
                <Copy className="size-4" />
                Copy Repo URL
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem asChild>
                <Link href={`/project/${projectId}/settings`}>
                  <Settings className="size-4" />
                  Settings
                </Link>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
    </div>
  );
}
