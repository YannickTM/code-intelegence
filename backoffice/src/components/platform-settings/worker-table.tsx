"use client";

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "~/components/ui/table";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "~/components/ui/tooltip";
import { formatUptime, formatHeartbeat } from "~/lib/format";
import type { WorkerStatus } from "~/server/api/routers/platform-workers";
import { WorkerStatusBadge } from "./worker-status-badge";
import { WorkflowBadges } from "./workflow-badges";

interface WorkerTableProps {
  items: WorkerStatus[];
}

export function WorkerTable({ items }: WorkerTableProps) {
  return (
    <TooltipProvider>
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Worker ID</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Hostname</TableHead>
              <TableHead>Version</TableHead>
              <TableHead>Workflows</TableHead>
              <TableHead>Current Job</TableHead>
              <TableHead>Uptime</TableHead>
              <TableHead>Last Heartbeat</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((worker) => (
              <TableRow key={worker.worker_id}>
                {/* Worker ID */}
                <TableCell>
                  <span className="font-mono text-sm">
                    {worker.worker_id}
                  </span>
                </TableCell>

                {/* Status */}
                <TableCell>
                  <WorkerStatusBadge
                    status={worker.status}
                    drainReason={worker.drain_reason}
                  />
                </TableCell>

                {/* Hostname */}
                <TableCell>
                  <span className="text-muted-foreground text-sm">
                    {worker.hostname || "—"}
                  </span>
                </TableCell>

                {/* Version */}
                <TableCell>
                  <span className="text-muted-foreground text-sm">
                    {worker.version || "—"}
                  </span>
                </TableCell>

                {/* Workflows */}
                <TableCell>
                  <WorkflowBadges workflows={worker.supported_workflows} />
                </TableCell>

                {/* Current Job */}
                <TableCell>
                  {worker.current_job_id ? (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span className="font-mono text-sm">
                          {worker.current_job_id.slice(0, 8)}
                        </span>
                      </TooltipTrigger>
                      <TooltipContent>
                        {worker.current_project_id
                          ? `Project: ${worker.current_project_id}`
                          : worker.current_job_id}
                      </TooltipContent>
                    </Tooltip>
                  ) : (
                    <span className="text-muted-foreground text-sm">—</span>
                  )}
                </TableCell>

                {/* Uptime */}
                <TableCell>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className="text-muted-foreground text-sm">
                        {formatUptime(worker.started_at)}
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>
                      {new Date(worker.started_at).toLocaleString()}
                    </TooltipContent>
                  </Tooltip>
                </TableCell>

                {/* Last Heartbeat */}
                <TableCell>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className="text-muted-foreground text-sm">
                        {formatHeartbeat(worker.last_heartbeat_at)}
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>
                      {new Date(worker.last_heartbeat_at).toLocaleString()}
                    </TooltipContent>
                  </Tooltip>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </TooltipProvider>
  );
}
