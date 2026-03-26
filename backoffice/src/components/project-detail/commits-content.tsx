"use client";

import { useState, useCallback, useEffect, useMemo } from "react";
import Link from "next/link";
import {
  AlertCircle,
  Calendar,
  ChevronDown,
  ChevronRight,
  GitCommitHorizontal,
  Search,
} from "lucide-react";
import { keepPreviousData } from "@tanstack/react-query";
import { api } from "~/trpc/react";
import { Alert, AlertDescription } from "~/components/ui/alert";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "~/components/ui/collapsible";
import { Input } from "~/components/ui/input";
import { useDebounce } from "~/hooks/use-debounce";
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
import { Pagination } from "~/components/pagination";
import { formatRelativeTime } from "~/lib/format";
import type { CommitSummary } from "~/server/api/routers/project-commits";

const PAGE_SIZE = 20;
const MAX_LOAD_MORE_PAGES = 6; // 120 items

// ── Column definitions ───────────────────────────────────────────────────────

const COLUMNS: { label: string; className?: string }[] = [
  { label: "Hash", className: "w-[90px]" },
  { label: "Message" },
  { label: "Author", className: "w-[160px]" },
  { label: "Date", className: "w-[100px]" },
];

function CommitsTableHeader() {
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
      <TableCell>
        <Skeleton className="h-4 w-16" />
      </TableCell>
      <TableCell>
        <Skeleton className="h-4 w-64" />
      </TableCell>
      <TableCell>
        <Skeleton className="h-4 w-24" />
      </TableCell>
      <TableCell>
        <Skeleton className="h-4 w-16" />
      </TableCell>
    </TableRow>
  );
}

// ── Relative time with tooltip ──────────────────────────────────────────────

function RelativeTime({ date }: { date: string }) {
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

// ── Commit row ──────────────────────────────────────────────────────────────

function CommitRow({
  commit,
  projectId,
}: {
  commit: CommitSummary;
  projectId: string;
}) {
  const href = `/project/${projectId}/commits/${commit.commit_hash}`;
  return (
    <TableRow className="group">
      <TableCell>
        <Link href={href} className="block">
          <code className="text-muted-foreground text-xs font-mono">
            {commit.short_hash}
          </code>
        </Link>
      </TableCell>
      <TableCell>
        <Link
          href={href}
          className="block max-w-[500px] truncate font-medium"
        >
          {commit.message_subject}
        </Link>
      </TableCell>
      <TableCell>
        <Link
          href={href}
          className="text-muted-foreground block text-sm"
        >
          {commit.author_name}
        </Link>
      </TableCell>
      <TableCell>
        <Link href={href} className="block">
          <RelativeTime date={commit.committer_date} />
        </Link>
      </TableCell>
    </TableRow>
  );
}

// ── Main component ──────────────────────────────────────────────────────────

export function CommitsContent({ projectId }: { projectId: string }) {
  const [page, setPage] = useState(0);
  const [loadedPages, setLoadedPages] = useState<
    Record<number, CommitSummary[]>
  >({});

  // Filter state
  const [searchInput, setSearchInput] = useState("");
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");
  const [filtersOpen, setFiltersOpen] = useState(false);
  const debouncedSearch = useDebounce(searchInput, 300);
  const hasDateFilters = fromDate !== "" || toDate !== "";
  const hasFilters = debouncedSearch !== "" || hasDateFilters;

  // Reset pagination when filters change
  useEffect(() => {
    setPage(0);
    setLoadedPages({});
  }, [debouncedSearch, fromDate, toDate]);

  const { data, isLoading, isError, error } =
    api.projectCommits.listCommits.useQuery(
      {
        projectId,
        search: debouncedSearch || undefined,
        fromDate: fromDate
          ? new Date(fromDate + "T00:00:00").toISOString()
          : undefined,
        toDate: toDate
          ? new Date(toDate + "T23:59:59").toISOString()
          : undefined,
        limit: PAGE_SIZE,
        offset: page * PAGE_SIZE,
      },
      { placeholderData: keepPreviousData },
    );

  const total = data?.total ?? 0;
  const usePagination = total > PAGE_SIZE * MAX_LOAD_MORE_PAGES;

  const displayItems = useMemo(() => {
    if (!data) return [];
    if (usePagination) return data.items;

    const pages: CommitSummary[] = [];
    const sortedKeys = Object.keys(loadedPages)
      .map(Number)
      .sort((a, b) => a - b);
    for (const key of sortedKeys) {
      pages.push(...(loadedPages[key] ?? []));
    }
    if (!(page in loadedPages)) {
      pages.push(...data.items);
    }
    return pages;
  }, [data, loadedPages, page, usePagination]);

  const hasMoreToLoad =
    !usePagination && total > PAGE_SIZE && displayItems.length < total;

  const handleLoadMore = useCallback(() => {
    if (!data) return;
    setLoadedPages((prev) => ({ ...prev, [page]: data.items }));
    setPage((p) => p + 1);
  }, [data, page]);

  const handlePageChange = useCallback((newPage: number) => {
    setLoadedPages({});
    setPage(newPage);
  }, []);

  // ── Filter bar ──────────────────────────────────────────────────────────

  const filterBar = (
    <div className="flex flex-col gap-2">
      {/* Row 1: Search input + total */}
      <div className="flex items-center gap-3">
        <div className="relative flex-1">
          <Search className="text-muted-foreground pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2" />
          <Input
            placeholder="Search commits..."
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-9"
          />
        </div>
        {isLoading && !data ? (
          <Skeleton className="h-5 w-24 shrink-0 rounded-full" />
        ) : total > 0 ? (
          <Badge variant="secondary" className="shrink-0">
            {total.toLocaleString()} commits
          </Badge>
        ) : null}
      </div>

      {/* Row 2: Collapsible date range filters */}
      <Collapsible open={filtersOpen} onOpenChange={setFiltersOpen}>
        <CollapsibleTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="text-muted-foreground hover:text-foreground -ml-2 gap-1.5"
          >
            {filtersOpen ? (
              <ChevronDown className="size-4" />
            ) : (
              <ChevronRight className="size-4" />
            )}
            Filters
            {hasDateFilters && (
              <span className="text-primary ml-0.5 text-xs">●</span>
            )}
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent className="flex flex-col gap-2 pt-1">
          <div className="flex items-center gap-2">
            <label className="text-muted-foreground w-10 shrink-0 text-sm">
              From
            </label>
            <div className="relative flex-1">
              <Calendar className="text-muted-foreground pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2" />
              <input
                type="date"
                value={fromDate}
                onChange={(e) => setFromDate(e.target.value)}
                className="border-input bg-background ring-offset-background placeholder:text-muted-foreground focus-visible:ring-ring h-9 w-full rounded-md border pl-9 pr-3 py-1 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2"
                aria-label="From date"
              />
            </div>
          </div>
          <div className="flex items-center gap-2">
            <label className="text-muted-foreground w-10 shrink-0 text-sm">
              To
            </label>
            <div className="relative flex-1">
              <Calendar className="text-muted-foreground pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2" />
              <input
                type="date"
                value={toDate}
                onChange={(e) => setToDate(e.target.value)}
                className="border-input bg-background ring-offset-background placeholder:text-muted-foreground focus-visible:ring-ring h-9 w-full rounded-md border pl-9 pr-3 py-1 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2"
                aria-label="To date"
              />
            </div>
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  );

  // ── Loading ─────────────────────────────────────────────────────────────

  if (isLoading && !data) {
    return (
      <div className="flex flex-col gap-4">
        {filterBar}
        <div className="rounded-md border">
          <Table>
            <CommitsTableHeader />
            <TableBody>
              {Array.from({ length: 6 }, (_, i) => (
                <SkeletonRow key={i} />
              ))}
            </TableBody>
          </Table>
        </div>
      </div>
    );
  }

  // ── Error ───────────────────────────────────────────────────────────────

  if (isError) {
    return (
      <Alert variant="destructive">
        <AlertCircle className="h-4 w-4" />
        <AlertDescription>
          {error?.message ?? "Failed to load commit history."}
        </AlertDescription>
      </Alert>
    );
  }

  // ── Empty (no commits at all) ─────────────────────────────────────────

  if (total === 0 && !hasFilters) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center rounded-xl border border-dashed p-12">
        <GitCommitHorizontal className="text-muted-foreground mb-4 h-10 w-10" />
        <h3 className="text-lg font-semibold">No commits indexed yet</h3>
        <p className="text-muted-foreground text-sm">
          Commit history will appear after the next index run.
        </p>
      </div>
    );
  }

  // ── Table ───────────────────────────────────────────────────────────────

  return (
    <TooltipProvider>
      <div className="flex flex-col gap-4">
        {filterBar}

        {total === 0 && hasFilters ? (
          <div className="flex flex-col items-center justify-center rounded-xl border border-dashed p-12">
            <Search className="text-muted-foreground mb-4 h-10 w-10" />
            <h3 className="text-lg font-semibold">
              No commits match your filters
            </h3>
            <p className="text-muted-foreground text-sm">
              Try adjusting your search or date range.
            </p>
          </div>
        ) : (
          <>
            <div className="rounded-md border">
              <Table>
                <CommitsTableHeader />
                <TableBody>
                  {displayItems.map((commit) => (
                    <CommitRow
                      key={commit.id}
                      commit={commit}
                      projectId={projectId}
                    />
                  ))}
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
            {usePagination && (
              <Pagination
                page={page}
                pageSize={PAGE_SIZE}
                total={total}
                noun="commits"
                onPageChange={handlePageChange}
              />
            )}
          </>
        )}
      </div>
    </TooltipProvider>
  );
}
