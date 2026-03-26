"use client";

import {
  Table,
  TableBody,
  TableHead,
  TableHeader,
  TableRow,
  TableCell,
} from "~/components/ui/table";
import { Skeleton } from "~/components/ui/skeleton";

function SkeletonRow() {
  return (
    <TableRow>
      {/* Name + repo subtitle */}
      <TableCell>
        <div className="space-y-1.5">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-3 w-48" />
        </div>
      </TableCell>
      {/* Branch */}
      <TableCell>
        <Skeleton className="h-4 w-16" />
      </TableCell>
      {/* Status badge */}
      <TableCell>
        <Skeleton className="h-5 w-16 rounded-full" />
      </TableCell>
      {/* Last Indexed */}
      <TableCell>
        <Skeleton className="h-4 w-14" />
      </TableCell>
      {/* Role badge */}
      <TableCell>
        <Skeleton className="h-5 w-14 rounded-full" />
      </TableCell>
      {/* Actions icon */}
      <TableCell>
        <Skeleton className="size-8 rounded-md" />
      </TableCell>
    </TableRow>
  );
}

export function ProjectsTableSkeleton() {
  return (
    <div className="rounded-lg border">
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
          {Array.from({ length: 5 }, (_, i) => (
            <SkeletonRow key={i} />
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
