"use client";

import { useRouter } from "next/navigation";
import { TableCell, TableRow } from "~/components/ui/table";
import type { ProjectWithHealth } from "~/lib/dashboard-types";
import {
  getProjectHealthStatus,
  getStatusDotColor,
  getStatusTextColor,
  getHealthLabel,
  formatRelativeTime,
} from "~/lib/health-utils";

export function ProjectHealthRow({ project }: { project: ProjectWithHealth }) {
  const router = useRouter();
  const status = getProjectHealthStatus(project);

  const navigate = () => router.push(`/project/${project.id}`);

  return (
    <TableRow
      className="cursor-pointer"
      tabIndex={0}
      role="link"
      aria-label={`View project ${project.name}`}
      onClick={navigate}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          navigate();
        }
      }}
    >
      <TableCell className="w-8">
        <span
          className={`inline-block size-2.5 rounded-full ${getStatusDotColor(status)}`}
        />
      </TableCell>
      <TableCell className="font-medium">{project.name}</TableCell>
      <TableCell className="text-muted-foreground">
        {project.default_branch}
      </TableCell>
      <TableCell className="text-muted-foreground font-mono text-xs">
        {project.index_git_commit
          ? project.index_git_commit.slice(0, 7)
          : "\u2014"}
      </TableCell>
      <TableCell className="text-muted-foreground">
        {project.index_activated_at
          ? formatRelativeTime(project.index_activated_at)
          : "Never"}
      </TableCell>
      <TableCell>
        <span className={`text-sm ${getStatusTextColor(status)}`}>
          {getHealthLabel(status)}
        </span>
      </TableCell>
    </TableRow>
  );
}
