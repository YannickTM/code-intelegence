"use client";

import { FolderSearch, RefreshCw } from "lucide-react";
import { Button } from "~/components/ui/button";
import { useProjectDetailMutations } from "~/hooks/use-project-detail-mutations";

export function FileTreeEmpty({ projectId }: { projectId: string }) {
  const { triggerIndex } = useProjectDetailMutations(projectId);

  return (
    <div className="flex min-h-[400px] items-center justify-center">
      <div className="text-muted-foreground flex flex-col items-center gap-3 text-center">
        <FolderSearch className="size-10" />
        <div>
          <p className="font-medium">No files indexed yet</p>
          <p className="text-sm">Trigger an index to populate the file tree.</p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() =>
            triggerIndex.mutate({ projectId, job_type: "full" })
          }
          disabled={triggerIndex.isPending}
        >
          <RefreshCw className="size-4" />
          Trigger Index
        </Button>
      </div>
    </div>
  );
}
