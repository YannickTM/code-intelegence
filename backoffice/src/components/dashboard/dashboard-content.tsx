"use client";

import { AlertTriangle } from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription } from "~/components/ui/alert";
import { Skeleton } from "~/components/ui/skeleton";
import { PageHeader } from "~/components/page-header";
import { api } from "~/trpc/react";

import { EmptyState } from "./empty-state";
import { HealthStrip } from "./health-strip";
import { AlertsZone } from "./alerts-zone";
import { ProjectHealthList } from "./project-health-list";

type User = {
  id: string;
  username: string;
  display_name?: string;
  avatar_url?: string;
};

export function DashboardContent({ user }: { user: User | null }) {
  const summary = api.dashboard.summary.useQuery(undefined, {
    retry: false,
  });

  const projects = api.users.listMyProjects.useQuery(undefined, {
    retry: false,
  });

  const utils = api.useUtils();

  const triggerIndex = api.projectIndexing.triggerIndex.useMutation({
    onSuccess: (_data, variables) => {
      const project = projects.data?.items.find(
        (p) => p.id === variables.projectId,
      );
      toast.success(`Indexing started for ${project?.name ?? "project"}`);
      void utils.dashboard.summary.invalidate();
      void utils.users.listMyProjects.invalidate();
    },
    onError: (error) => {
      toast.error(`Failed to start indexing: ${error.message}`);
    },
  });

  const handleIndexNow = (projectId: string) => {
    triggerIndex.mutate({ projectId });
  };

  const isLoading = summary.isLoading || projects.isLoading;
  const hasData = Boolean(summary.data && projects.data);
  const isFatalError = !hasData && (summary.isError || projects.isError);
  const hasRefetchError = hasData && (summary.isError || projects.isError);
  const hasProjects =
    (projects.data?.items?.length ?? 0) > 0 ||
    (summary.data?.projects_total ?? 0) > 0;

  const welcomeTitle = user
    ? `Welcome back, ${user.display_name ?? user.username}`
    : "Dashboard";

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={welcomeTitle}
        description="Here's an overview of your platform."
      />

      {isFatalError && (
        <Alert variant="destructive">
          <AlertTriangle className="h-4 w-4" />
          <AlertDescription>
            Failed to load dashboard data. Is the backend running?
          </AlertDescription>
        </Alert>
      )}

      {hasRefetchError && (
        <Alert variant="destructive">
          <AlertTriangle className="h-4 w-4" />
          <AlertDescription>
            Failed to refresh dashboard data. Showing cached results.
          </AlertDescription>
        </Alert>
      )}

      {!isFatalError && isLoading && !hasData && (
        <div className="flex flex-col gap-4">
          <Skeleton className="h-10 w-full rounded-lg" />
          <Skeleton className="h-64 w-full rounded-lg" />
        </div>
      )}

      {!isFatalError && !isLoading && !hasProjects && <EmptyState />}

      {!isFatalError && hasProjects && (
        <>
          <HealthStrip
            summary={summary.data}
            isLoading={summary.isLoading && !summary.data}
          />

          <AlertsZone
            projects={projects.data?.items ?? []}
            onIndexNow={handleIndexNow}
          />

          <section>
            <h2 className="mb-3 text-sm font-medium uppercase tracking-wide text-muted-foreground">
              Project Health
            </h2>
            <ProjectHealthList
              projects={projects.data?.items ?? []}
              isLoading={projects.isLoading && !projects.data}
            />
          </section>
        </>
      )}
    </div>
  );
}
