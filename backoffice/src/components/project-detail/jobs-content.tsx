"use client";

import { useState, useCallback, useMemo } from "react";
import { AlertCircle, ChevronRight, Database } from "lucide-react";
import { api } from "~/trpc/react";
import { Alert, AlertDescription } from "~/components/ui/alert";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import { Skeleton } from "~/components/ui/skeleton";
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
import { formatRelativeTime, formatDuration } from "~/lib/format";
import type { IndexJob } from "~/server/api/routers/project-indexing";

const PAGE_SIZE = 20;
const MAX_LOAD_MORE_PAGES = 6; // 120 items

// ── Status badge ────────────────────────────────────────────────────────────

function StatusBadge({ status }: { status: IndexJob["status"] }) {
  switch (status) {
    case "queued":
      return <Badge variant="outline">Queued</Badge>;
    case "running":
      return (
        <Badge className="animate-pulse bg-info/10 text-info">
          Running
        </Badge>
      );
    case "completed":
      return (
        <Badge className="bg-success/10 text-success">
          Completed
        </Badge>
      );
    case "failed":
      return <Badge variant="destructive">Failed</Badge>;
    default: {
      const _exhaustive: never = status;
      return <Badge variant="outline">{String(_exhaustive)}</Badge>;
    }
  }
}

// ── Duration cell ───────────────────────────────────────────────────────────

function DurationCell({ job }: { job: IndexJob }) {
  if (!job.started_at) return <span className="text-muted-foreground">—</span>;
  if (!job.finished_at)
    return <span className="text-muted-foreground italic">Running…</span>;
  return <span>{formatDuration(job.started_at, job.finished_at)}</span>;
}

// ── Relative time with tooltip ──────────────────────────────────────────────

function RelativeTime({ date }: { date: string | null }) {
  if (!date) return <span className="text-muted-foreground">—</span>;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="text-muted-foreground text-sm">
          {formatRelativeTime(date)}
        </span>
      </TooltipTrigger>
      <TooltipContent>{new Date(date).toLocaleString()}</TooltipContent>
    </Tooltip>
  );
}

// ── Column definitions ───────────────────────────────────────────────────────

const COLUMNS: { label: string; className?: string }[] = [
  { label: "", className: "w-8" },
  { label: "Type" },
  { label: "Status" },
  { label: "Files", className: "text-right" },
  { label: "Chunks", className: "text-right" },
  { label: "Deleted", className: "text-right" },
  { label: "Started" },
  { label: "Duration" },
  { label: "Created" },
];

function JobsTableHeader() {
  return (
    <TableHeader>
      <TableRow>
        {COLUMNS.map((col, i) => (
          <TableHead key={i} className={col.className}>
            {col.label}
          </TableHead>
        ))}
      </TableRow>
    </TableHeader>
  );
}

// ── Skeleton rows ───────────────────────────────────────────────────────────

function SkeletonRow() {
  return (
    <TableRow>
      <TableCell><Skeleton className="size-4" /></TableCell>
      <TableCell><Skeleton className="h-5 w-20" /></TableCell>
      <TableCell><Skeleton className="h-5 w-16" /></TableCell>
      <TableCell><Skeleton className="h-4 w-10" /></TableCell>
      <TableCell><Skeleton className="h-4 w-10" /></TableCell>
      <TableCell><Skeleton className="h-4 w-10" /></TableCell>
      <TableCell><Skeleton className="h-4 w-16" /></TableCell>
      <TableCell><Skeleton className="h-4 w-14" /></TableCell>
      <TableCell><Skeleton className="h-4 w-16" /></TableCell>
    </TableRow>
  );
}

// ── Helpers ─────────────────────────────────────────────────────────────────

function jobTypeLabel(type: IndexJob["job_type"]): string {
  switch (type) {
    case "full":
      return "Full";
    case "incremental":
      return "Incremental";
    default: {
      const _exhaustive: never = type;
      return String(_exhaustive);
    }
  }
}

function hasActiveJob(items: IndexJob[]) {
  return items.some((j) => j.status === "queued" || j.status === "running");
}

/** Returns page indexes and "ellipsis" tokens for truncated pagination. */
function getVisiblePages(
  current: number,
  total: number,
): (number | "ellipsis")[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i);
  const pages: (number | "ellipsis")[] = [0];
  const start = Math.max(1, current - 1);
  const end = Math.min(total - 2, current + 1);
  if (start > 1) pages.push("ellipsis");
  for (let i = start; i <= end; i++) pages.push(i);
  if (end < total - 2) pages.push("ellipsis");
  pages.push(total - 1);
  return pages;
}

// ── Main component ──────────────────────────────────────────────────────────

export function JobsContent({ projectId }: { projectId: string }) {
  const [expandedJobId, setExpandedJobId] = useState<string | null>(null);
  const [page, setPage] = useState(0);
  // Store loaded pages keyed by page index for stable load-more
  const [loadedPages, setLoadedPages] = useState<Record<number, IndexJob[]>>(
    {},
  );

  const { data, isLoading, isError, error } =
    api.projectIndexing.listJobs.useQuery(
      { projectId, limit: PAGE_SIZE, offset: page * PAGE_SIZE },
      {
        refetchInterval: (query) => {
          // Check current query data AND all previously loaded pages
          const currentHasActive = query.state.data?.items
            ? hasActiveJob(query.state.data.items)
            : false;
          const loadedHasActive = Object.values(loadedPages).some(hasActiveJob);
          return currentHasActive || loadedHasActive ? 3000 : false;
        },
      },
    );

  const total = data?.total ?? 0;
  const totalPages = Math.ceil(total / PAGE_SIZE);
  const usePagination = total > PAGE_SIZE * MAX_LOAD_MORE_PAGES;

  // In "load more" mode, flatten loaded pages in order + current page
  // In pagination mode, show only current page items
  const displayItems = useMemo(() => {
    if (!data) return [];
    if (usePagination) return data.items;

    const pages: IndexJob[] = [];
    // Add previously loaded pages in order
    const sortedKeys = Object.keys(loadedPages)
      .map(Number)
      .sort((a, b) => a - b);
    for (const key of sortedKeys) {
      pages.push(...(loadedPages[key] ?? []));
    }
    // Add current page if not already in loadedPages
    if (!(page in loadedPages)) {
      pages.push(...data.items);
    }
    return pages;
  }, [data, loadedPages, page, usePagination]);

  const hasMoreToLoad =
    !usePagination && total > PAGE_SIZE && displayItems.length < total;

  const handleLoadMore = useCallback(() => {
    if (!data) return;
    // Store current page before advancing
    setLoadedPages((prev) => ({ ...prev, [page]: data.items }));
    setPage((p) => p + 1);
  }, [data, page]);

  const handlePageChange = useCallback((newPage: number) => {
    setLoadedPages({});
    setPage(newPage);
  }, []);

  const toggleExpanded = useCallback((jobId: string) => {
    setExpandedJobId((prev) => (prev === jobId ? null : jobId));
  }, []);

  // ── Loading ─────────────────────────────────────────────────────────────

  if (isLoading) {
    return (
      <div className="rounded-md border">
        <Table>
          <JobsTableHeader />
          <TableBody>
            {Array.from({ length: 5 }, (_, i) => (
              <SkeletonRow key={i} />
            ))}
          </TableBody>
        </Table>
      </div>
    );
  }

  // ── Error ───────────────────────────────────────────────────────────────

  if (isError) {
    return (
      <Alert variant="destructive">
        <AlertCircle className="h-4 w-4" />
        <AlertDescription>
          {error?.message ?? "Failed to load indexing jobs."}
        </AlertDescription>
      </Alert>
    );
  }

  // ── Empty ───────────────────────────────────────────────────────────────

  if (total === 0) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center rounded-xl border border-dashed p-12">
        <Database className="text-muted-foreground mb-4 h-10 w-10" />
        <h3 className="text-lg font-semibold">No indexing jobs yet</h3>
        <p className="text-muted-foreground text-sm">
          Trigger your first index from the button in the header.
        </p>
      </div>
    );
  }

  // ── Table ───────────────────────────────────────────────────────────────

  return (
    <TooltipProvider>
      <div className="flex flex-col gap-4">
        <div className="rounded-md border">
          <Table>
            <JobsTableHeader />
            <TableBody>
              {displayItems.map((job) => {
                const isExpandable =
                  job.status === "failed" && job.error_details.length > 0;
                const isExpanded = expandedJobId === job.id;

                return (
                  <JobRow
                    key={job.id}
                    job={job}
                    isExpandable={isExpandable}
                    isExpanded={isExpanded}
                    onToggle={toggleExpanded}
                  />
                );
              })}
            </TableBody>
          </Table>
        </div>

        {/* Load more */}
        {hasMoreToLoad && (
          <div className="flex justify-center">
            <Button variant="outline" onClick={handleLoadMore}>
              Load more
            </Button>
          </div>
        )}

        {/* Page navigation */}
        {usePagination && totalPages > 1 && (
          <div className="flex items-center justify-center gap-1">
            <Button
              variant="outline"
              size="sm"
              disabled={page === 0}
              onClick={() => handlePageChange(page - 1)}
            >
              Previous
            </Button>
            {getVisiblePages(page, totalPages).map((entry, idx) =>
              entry === "ellipsis" ? (
                <span
                  key={`ellipsis-${idx}`}
                  className="text-muted-foreground px-1 text-sm"
                >
                  …
                </span>
              ) : (
                <Button
                  key={entry}
                  variant={entry === page ? "default" : "outline"}
                  size="sm"
                  onClick={() => handlePageChange(entry)}
                >
                  {entry + 1}
                </Button>
              ),
            )}
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages - 1}
              onClick={() => handlePageChange(page + 1)}
            >
              Next
            </Button>
          </div>
        )}
      </div>
    </TooltipProvider>
  );
}

// ── Job row (with optional expanded error details) ──────────────────────────

function JobRow({
  job,
  isExpandable,
  isExpanded,
  onToggle,
}: {
  job: IndexJob;
  isExpandable: boolean;
  isExpanded: boolean;
  onToggle: (id: string) => void;
}) {
  return (
    <>
      <TableRow>
        <TableCell className="w-8 px-2">
          {isExpandable ? (
            <button
              type="button"
              aria-expanded={isExpanded}
              aria-label={`${isExpanded ? "Collapse" : "Expand"} error details`}
              className="text-muted-foreground hover:text-foreground flex items-center justify-center rounded p-0.5 transition-transform"
              onClick={() => onToggle(job.id)}
            >
              <ChevronRight
                className={`size-4 transition-transform ${isExpanded ? "rotate-90" : ""}`}
              />
            </button>
          ) : null}
        </TableCell>
        <TableCell>
          <Badge variant="outline">{jobTypeLabel(job.job_type)}</Badge>
        </TableCell>
        <TableCell>
          <StatusBadge status={job.status} />
        </TableCell>
        <TableCell className="text-right tabular-nums">
          {job.files_processed}
        </TableCell>
        <TableCell className="text-right tabular-nums">
          {job.chunks_upserted}
        </TableCell>
        <TableCell className="text-right tabular-nums">
          {job.vectors_deleted}
        </TableCell>
        <TableCell>
          <RelativeTime date={job.started_at} />
        </TableCell>
        <TableCell>
          <DurationCell job={job} />
        </TableCell>
        <TableCell>
          <RelativeTime date={job.created_at} />
        </TableCell>
      </TableRow>
      {isExpanded && (
        <TableRow>
          <TableCell colSpan={COLUMNS.length} className="bg-muted/50 p-0">
            <div className="px-4 py-3">
              <p className="text-muted-foreground mb-1 text-xs font-medium uppercase">
                Error Details
              </p>
              <pre className="text-sm whitespace-pre-wrap font-mono">
                {job.error_details
                  .map(
                    (e) =>
                      `[${e.category}] ${e.message} (step: ${e.step})`,
                  )
                  .join("\n")}
              </pre>
            </div>
          </TableCell>
        </TableRow>
      )}
    </>
  );
}
