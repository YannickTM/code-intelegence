"use client";

import {
  Table,
  TableBody,
  TableHead,
  TableHeader,
  TableRow,
  TableCell,
} from "~/components/ui/table";
import { Card } from "~/components/ui/card";
import { TooltipProvider } from "~/components/ui/tooltip";
import type { UserProject } from "~/server/api/routers/users";
import { ProjectRow } from "./project-row";

export function ProjectsTable({
  projects,
  onTriggerIndex,
  onToggleStatus,
  onDelete,
}: {
  projects: UserProject[];
  onTriggerIndex: (projectId: string) => void;
  onToggleStatus: (projectId: string, newStatus: "active" | "paused") => void;
  onDelete: (project: UserProject) => void;
}) {
  return (
    <TooltipProvider>
      <Card className="overflow-hidden p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Branch</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Last Indexed</TableHead>
              <TableHead>Role</TableHead>
              <TableHead className="w-12">
                <span className="sr-only">Actions</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {projects.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={6}
                  className="text-muted-foreground h-24 text-center"
                >
                  No projects match your filters.
                </TableCell>
              </TableRow>
            )}
            {projects.map((project) => (
              <ProjectRow
                key={project.id}
                project={project}
                onTriggerIndex={onTriggerIndex}
                onToggleStatus={onToggleStatus}
                onDelete={onDelete}
              />
            ))}
          </TableBody>
        </Table>
      </Card>
    </TooltipProvider>
  );
}
