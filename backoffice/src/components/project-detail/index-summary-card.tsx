"use client";

import { cn } from "~/lib/utils";
import { Badge } from "~/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card";
import type { ProjectWithHealth } from "~/lib/dashboard-types";
import {
  getProjectHealthStatus,
  getStatusDotColor,
  getHealthLabel,
  formatRelativeTime,
} from "~/lib/health-utils";

export function IndexSummaryCard({ project }: { project: ProjectWithHealth }) {
  const healthStatus = getProjectHealthStatus(project);
  const dotColor = getStatusDotColor(healthStatus);
  const healthLabel = getHealthLabel(healthStatus);

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-medium">Index Summary</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <div className="flex justify-between">
          <span className="text-muted-foreground">Branch</span>
          <Badge variant="outline" className="font-normal">
            {project.index_branch ?? project.default_branch}
          </Badge>
        </div>
        <div className="flex justify-between">
          <span className="text-muted-foreground">Commit</span>
          <code className="font-mono text-xs">
            {project.index_git_commit
              ? project.index_git_commit.slice(0, 7)
              : "—"}
          </code>
        </div>
        <div className="flex justify-between">
          <span className="text-muted-foreground">Indexed</span>
          <span>
            {project.index_activated_at
              ? formatRelativeTime(project.index_activated_at)
              : "Never"}
          </span>
        </div>
        <div className="flex justify-between">
          <span className="text-muted-foreground">Health</span>
          <span className="flex items-center gap-1.5">
            <span className={cn("size-2 rounded-full", dotColor)} />
            {healthLabel}
          </span>
        </div>
      </CardContent>
    </Card>
  );
}
