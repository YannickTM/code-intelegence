"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { AlertTriangle, Plus } from "lucide-react";
import { Alert, AlertDescription } from "~/components/ui/alert";
import { Button } from "~/components/ui/button";
import { PageHeader } from "~/components/page-header";
import { api } from "~/trpc/react";
import type { UserProject } from "~/server/api/routers/users";
import { deriveProjectStatus } from "~/lib/project-status";
import type { StatusFilter } from "~/lib/project-status";
import { useProjectMutations } from "~/hooks/use-project-mutations";

import { ProjectsToolbar } from "./projects-toolbar";
import { ProjectsTable } from "./projects-table";
import { ProjectsEmptyState } from "./projects-empty-state";
import { DeleteProjectDialog } from "./delete-project-dialog";
import { ProjectsTableSkeleton } from "./projects-table-skeleton";

export function ProjectsContent() {
  // ── State ───────────────────────────────────────────────────────────────
  const [searchQuery, setSearchQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [deleteTarget, setDeleteTarget] = useState<UserProject | null>(null);

  // ── Queries ─────────────────────────────────────────────────────────────
  const projects = api.users.listMyProjects.useQuery(undefined, {
    retry: false,
    refetchInterval: 30_000,
  });

  // ── Mutations ───────────────────────────────────────────────────────────
  const { triggerIndex, toggleStatus } = useProjectMutations(
    projects.data?.items,
  );

  // ── Handlers ────────────────────────────────────────────────────────────
  const handleTriggerIndex = (projectId: string) => {
    triggerIndex.mutate({ projectId });
  };

  const handleToggleStatus = (
    projectId: string,
    newStatus: "active" | "paused",
  ) => {
    toggleStatus.mutate({ id: projectId, status: newStatus });
  };

  const handleDelete = (project: UserProject) => {
    setDeleteTarget(project);
  };

  // ── Derived state ───────────────────────────────────────────────────────
  const isLoading = projects.isLoading;
  const hasData = Boolean(projects.data);
  const isFatalError = !hasData && projects.isError;
  const hasRefetchError = hasData && projects.isError;
  const hasProjects = (projects.data?.items?.length ?? 0) > 0;

  const filteredProjects = useMemo(() => {
    let items = projects.data?.items ?? [];

    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      items = items.filter(
        (p) =>
          p.name.toLowerCase().includes(q) ||
          p.repo_url.toLowerCase().includes(q),
      );
    }

    if (statusFilter !== "all") {
      items = items.filter((p) => deriveProjectStatus(p) === statusFilter);
    }

    return items;
  }, [projects.data?.items, searchQuery, statusFilter]);

  // ── Render ──────────────────────────────────────────────────────────────
  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Projects"
        description="Manage your connected repositories."
      >
        <Button asChild>
          <Link href="/project/create">
            <Plus className="size-4" />
            New Project
          </Link>
        </Button>
      </PageHeader>

      {/* Fatal error */}
      {isFatalError && (
        <Alert variant="destructive">
          <AlertTriangle className="h-4 w-4" />
          <AlertDescription>
            Failed to load projects. Is the backend running?
          </AlertDescription>
        </Alert>
      )}

      {/* Refetch error (show stale data with warning) */}
      {hasRefetchError && (
        <Alert variant="destructive">
          <AlertTriangle className="h-4 w-4" />
          <AlertDescription>
            Failed to refresh projects. Showing cached results.
          </AlertDescription>
        </Alert>
      )}

      {/* Loading */}
      {!isFatalError && isLoading && !hasData && <ProjectsTableSkeleton />}

      {/* Empty state */}
      {!isFatalError && !isLoading && !hasProjects && <ProjectsEmptyState />}

      {/* Main content */}
      {!isFatalError && hasProjects && (
        <>
          <ProjectsToolbar
            searchQuery={searchQuery}
            onSearchChange={setSearchQuery}
            statusFilter={statusFilter}
            onStatusFilterChange={setStatusFilter}
          />
          <ProjectsTable
            projects={filteredProjects}
            onTriggerIndex={handleTriggerIndex}
            onToggleStatus={handleToggleStatus}
            onDelete={handleDelete}
          />
        </>
      )}

      {/* Dialogs */}
      <DeleteProjectDialog
        project={deleteTarget}
        onClose={() => setDeleteTarget(null)}
      />
    </div>
  );
}
