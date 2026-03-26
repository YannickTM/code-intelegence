"use client";

import type { ProjectWithHealth } from "~/lib/dashboard-types";
import { getAlertForProject } from "~/lib/health-utils";
import { useDismissedAlerts } from "~/hooks/use-dismissed-alerts";
import { AlertRow } from "./alert-row";

export function AlertsZone({
  projects,
  onIndexNow,
}: {
  projects: ProjectWithHealth[];
  onIndexNow: (projectId: string) => void;
}) {
  const { isDismissed, dismiss } = useDismissedAlerts();

  const alerts = projects
    .map((project) => {
      const alertType = getAlertForProject(project);
      if (!alertType) return null;
      // Include a version so dismissals don't suppress new/changed alerts
      const alertVersion =
        alertType === "failed"
          ? project.failed_job_id
          : alertType === "stale"
            ? project.index_activated_at
            : "v1"; // never_indexed is stateless
      const alertId = `alert:${project.id}:${alertType}:${alertVersion}`;
      if (isDismissed(alertId)) return null;
      return { project, alertType, alertId };
    })
    .filter(Boolean) as {
    project: ProjectWithHealth;
    alertType: "failed" | "never_indexed" | "stale";
    alertId: string;
  }[];

  if (alerts.length === 0) return null;

  return (
    <div className="flex flex-col gap-2">
      {alerts.map(({ project, alertType, alertId }) => (
        <AlertRow
          key={alertId}
          project={project}
          alertType={alertType}
          onDismiss={() => dismiss(alertId)}
          onIndexNow={onIndexNow}
        />
      ))}
    </div>
  );
}
