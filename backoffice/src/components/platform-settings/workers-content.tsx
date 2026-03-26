"use client";

import { AlertCircle, RefreshCw, Activity } from "lucide-react";
import { Button } from "~/components/ui/button";
import { Skeleton } from "~/components/ui/skeleton";
import { api } from "~/trpc/react";
import { WorkerTable } from "./worker-table";

export function WorkersContent() {
  const utils = api.useUtils();

  const workersQuery = api.platformWorkers.list.useQuery(undefined, {
    retry: false,
  });

  const handleRefresh = () => {
    void utils.platformWorkers.list.invalidate();
  };

  const isRedisError =
    workersQuery.error?.message?.toLowerCase().includes("redis") ||
    workersQuery.error?.message?.toLowerCase().includes("worker status unavailable");

  return (
    <div className="flex flex-col gap-4">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <h2 className="text-lg font-semibold">Workers</h2>
          <p className="text-muted-foreground text-sm">
            View active worker instances and their current status.
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={handleRefresh}
          disabled={workersQuery.isLoading}
        >
          <RefreshCw
            className={`size-4 ${workersQuery.isFetching ? "animate-spin" : ""}`}
          />
          Refresh
        </Button>
      </div>

      {/* Content */}
      {workersQuery.isLoading ? (
        <div className="rounded-md border">
          <div className="space-y-3 p-4">
            {[1, 2, 3].map((i) => (
              <div key={i} className="flex items-center gap-3">
                <Skeleton className="h-4 w-40" />
                <Skeleton className="h-5 w-16 rounded-full" />
                <Skeleton className="h-4 w-28" />
                <Skeleton className="h-4 w-20" />
                <div className="flex gap-1">
                  <Skeleton className="h-5 w-16 rounded-full" />
                  <Skeleton className="h-5 w-16 rounded-full" />
                </div>
                <Skeleton className="h-4 w-20" />
                <Skeleton className="h-4 w-16" />
                <Skeleton className="h-4 w-14" />
              </div>
            ))}
          </div>
        </div>
      ) : workersQuery.isError ? (
        <div className="text-destructive flex items-center gap-2 text-sm">
          <AlertCircle className="size-4 shrink-0" />
          <span>
            {isRedisError
              ? "Worker status is temporarily unavailable. Redis may not be configured or reachable."
              : "Failed to load worker status."}
          </span>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => void workersQuery.refetch()}
          >
            Retry
          </Button>
        </div>
      ) : workersQuery.data?.items.length === 0 ? (
        <div className="flex flex-1 items-center justify-center rounded-xl border border-dashed p-16">
          <div className="flex flex-col items-center gap-4 text-center">
            <div className="rounded-full bg-primary/5 p-4">
              <Activity className="text-primary/60 size-12" />
            </div>
            <div>
              <h3 className="text-lg font-semibold">No workers online</h3>
              <p className="text-muted-foreground text-sm">
                No worker instances are currently running. Workers will appear
                here when they start and begin sending heartbeats.
              </p>
            </div>
          </div>
        </div>
      ) : (
        <WorkerTable items={workersQuery.data?.items ?? []} />
      )}
    </div>
  );
}
