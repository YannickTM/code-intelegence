"use client";

import Link from "next/link";
import { CheckCircle2, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "~/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
import { api } from "~/trpc/react";
import type { WizardAction, WizardState } from "~/lib/wizard-state";

export function StepDone({
  state,
  dispatch,
}: {
  state: WizardState;
  dispatch: React.Dispatch<WizardAction>;
}) {
  const triggerIndex = api.projectIndexing.triggerIndex.useMutation({
    onSuccess: () => {
      dispatch({ type: "SET_INDEX_TRIGGERED" });
      toast.success("Indexing started");
    },
    onError: (err) => {
      toast.error(
        `Index could not be started. You can trigger it from the project page. (${err.message})`,
      );
    },
  });

  const projectId = state.createdProject?.id;
  const projectName = state.createdProject?.name ?? state.projectName;

  function handleTriggerIndex() {
    if (!projectId) return;
    triggerIndex.mutate({ projectId, job_type: "full" });
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <CheckCircle2 className="size-6 text-success" />
          <div>
            <CardTitle>{projectName} is ready</CardTitle>
            <CardDescription>
              Your repository is connected. Start your first index to make it
              searchable.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
          <span className="text-muted-foreground">Repository</span>
          <span className="truncate font-mono text-xs">{state.repoUrl}</span>
          <span className="text-muted-foreground">Branch</span>
          <span>{state.defaultBranch}</span>
          <span className="text-muted-foreground">SSH Key</span>
          <span className="truncate font-mono text-xs">
            {state.resolvedKey?.fingerprint ?? "—"}
          </span>
        </div>
      </CardContent>
      <CardFooter className="gap-3">
        <Button
          onClick={handleTriggerIndex}
          disabled={
            !projectId || triggerIndex.isPending || state.indexTriggered
          }
        >
          {triggerIndex.isPending && (
            <Loader2 className="size-4 animate-spin" />
          )}
          {state.indexTriggered ? "Indexing Started" : "Start First Index"}
        </Button>
        {projectId && (
          <Button variant="outline" asChild>
            <Link href={`/project/${projectId}`}>Go to Project</Link>
          </Button>
        )}
      </CardFooter>
    </Card>
  );
}
