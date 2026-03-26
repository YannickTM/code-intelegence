"use client";

import {
  Table,
  TableBody,
  TableHead,
  TableHeader,
  TableRow,
} from "~/components/ui/table";
import { Card } from "~/components/ui/card";
import { Skeleton } from "~/components/ui/skeleton";
import type { ProjectWithHealth } from "~/lib/dashboard-types";
import { sortProjectsByHealth } from "~/lib/health-utils";
import { ProjectHealthRow } from "./project-health-row";

export function ProjectHealthList({
  projects,
  isLoading,
}: {
  projects: ProjectWithHealth[];
  isLoading: boolean;
}) {
  if (isLoading) {
    return (
      <div className="space-y-2">
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-12 w-full rounded-lg" />
        ))}
      </div>
    );
  }

  const sorted = sortProjectsByHealth(projects);

  return (
    <Card className="overflow-hidden p-0">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-8" />
            <TableHead>Name</TableHead>
            <TableHead>Branch</TableHead>
            <TableHead>Commit</TableHead>
            <TableHead>Last Indexed</TableHead>
            <TableHead>Health</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {sorted.map((project) => (
            <ProjectHealthRow key={project.id} project={project} />
          ))}
        </TableBody>
      </Table>
    </Card>
  );
}
