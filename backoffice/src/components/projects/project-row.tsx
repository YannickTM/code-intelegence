"use client";

import { useRouter } from "next/navigation";
import { TableRow, TableCell } from "~/components/ui/table";
import { Badge } from "~/components/ui/badge";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "~/components/ui/tooltip";
import type { UserProject } from "~/server/api/routers/users";
import {
  deriveProjectStatus,
  getStatusBadgeConfig,
} from "~/lib/project-status";
import { formatRelativeTime } from "~/lib/format";
import { ProjectActions } from "./project-actions";

function stripProtocol(url: string): string {
  return url
    .replace(/^https?:\/\//, "")
    .replace(/^git@([^:]+):/, "$1/")
    .replace(/\.git$/, "");
}

export function ProjectRow({
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
  const status = deriveProjectStatus(project);
  const badgeConfig = getStatusBadgeConfig(status);

  const navigate = () => router.push(`/project/${project.id}`);

  return (
    <TableRow
      className="cursor-pointer"
      onClick={navigate}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          navigate();
        }
      }}
      tabIndex={0}
      role="link"
      aria-label={`View project ${project.name}`}
    >
      <TableCell>
        <div>
          <span className="font-medium">{project.name}</span>
          <p className="text-muted-foreground text-xs">
            {stripProtocol(project.repo_url)}
          </p>
        </div>
      </TableCell>
      <TableCell className="text-muted-foreground">
        {project.default_branch}
      </TableCell>
      <TableCell>
        <Badge variant="outline" className={badgeConfig.className}>
          {badgeConfig.label}
        </Badge>
      </TableCell>
      <TableCell>
        {project.index_activated_at ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="text-muted-foreground text-sm">
                {formatRelativeTime(project.index_activated_at)}
              </span>
            </TooltipTrigger>
            <TooltipContent>
              {project.index_git_commit
                ? project.index_git_commit.slice(0, 7)
                : "No commit hash"}
            </TooltipContent>
          </Tooltip>
        ) : (
          <span className="text-muted-foreground text-sm">Never</span>
        )}
      </TableCell>
      <TableCell>
        <Badge variant="outline" className="capitalize">
          {project.role}
        </Badge>
      </TableCell>
      <TableCell>
        <ProjectActions
          project={project}
          onTriggerIndex={onTriggerIndex}
          onToggleStatus={onToggleStatus}
          onDelete={onDelete}
        />
      </TableCell>
    </TableRow>
  );
}
