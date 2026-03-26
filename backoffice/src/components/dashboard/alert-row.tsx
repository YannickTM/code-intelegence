"use client";

import Link from "next/link";
import { X, AlertTriangle, Clock, FolderX } from "lucide-react";
import { Button } from "~/components/ui/button";
import type { ProjectWithHealth, AlertType } from "~/lib/dashboard-types";
import { formatRelativeTime } from "~/lib/health-utils";

export function AlertRow({
  project,
  alertType,
  onDismiss,
  onIndexNow,
}: {
  project: ProjectWithHealth;
  alertType: AlertType;
  onDismiss: () => void;
  onIndexNow: (projectId: string) => void;
}) {
  const icon =
    alertType === "failed" ? (
      <AlertTriangle className="size-4 shrink-0 text-destructive" />
    ) : alertType === "never_indexed" ? (
      <FolderX className="size-4 shrink-0 text-muted-foreground" />
    ) : (
      <Clock className="size-4 shrink-0 text-warning" />
    );

  const message = (() => {
    switch (alertType) {
      case "failed":
        return (
          <>
            Indexing failed for <strong>{project.name}</strong>
            {project.failed_job_type
              ? ` \u00b7 ${project.failed_job_type} index`
              : ""}
            {project.failed_job_finished_at
              ? ` \u00b7 ${formatRelativeTime(project.failed_job_finished_at)}`
              : ""}
          </>
        );
      case "never_indexed":
        return (
          <>
            <strong>{project.name}</strong> has never been indexed
          </>
        );
      case "stale":
        return (
          <>
            <strong>{project.name}</strong> index is{" "}
            {project.index_activated_at
              ? formatRelativeTime(project.index_activated_at).replace(
                  " ago",
                  " old",
                )
              : "stale"}
          </>
        );
    }
  })();

  return (
    <div
      className={`flex items-center gap-3 rounded-lg border px-4 py-2 ${
        alertType === "failed"
          ? "border-destructive/20 bg-destructive/5"
          : alertType === "stale"
            ? "border-warning/20 bg-warning/5"
            : "border-border bg-muted"
      }`}
    >
      {icon}
      <span className="flex-1 text-sm">{message}</span>
      {alertType === "failed" ? (
        <Button variant="outline" size="xs" asChild>
          <Link href={`/project/${project.id}`}>View</Link>
        </Button>
      ) : (
        <Button
          variant="outline"
          size="xs"
          onClick={() => onIndexNow(project.id)}
        >
          Index Now
        </Button>
      )}
      <button
        onClick={onDismiss}
        className="text-muted-foreground hover:text-foreground transition-colors"
        aria-label="Dismiss alert"
      >
        <X className="size-3.5" />
      </button>
    </div>
  );
}
