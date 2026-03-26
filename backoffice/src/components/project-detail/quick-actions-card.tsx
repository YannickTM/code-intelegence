"use client";

import { RefreshCw } from "lucide-react";
import { Button } from "~/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card";
import {
  getProjectHealthStatus,
} from "~/lib/health-utils";
import { useProjectDetailMutations } from "~/hooks/use-project-detail-mutations";
import type { UserProject } from "~/server/api/routers/users";

export function QuickActionsCard({ project }: { project: UserProject }) {
  const { triggerIndex } = useProjectDetailMutations(project.id);
  const isIndexing = getProjectHealthStatus(project) === "indexing";
  const isDisabled = isIndexing || triggerIndex.isPending;

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-medium">Quick Actions</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        <Button
          variant="outline"
          size="sm"
          className="w-full justify-start"
          onClick={() => triggerIndex.mutate({ projectId: project.id, job_type: "incremental" })}
          disabled={isDisabled}
        >
          <RefreshCw className="size-4" />
          Trigger Incremental Index
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="w-full justify-start"
          onClick={() => triggerIndex.mutate({ projectId: project.id, job_type: "full" })}
          disabled={isDisabled}
        >
          <RefreshCw className="size-4" />
          Trigger Full Index
        </Button>
      </CardContent>
    </Card>
  );
}
