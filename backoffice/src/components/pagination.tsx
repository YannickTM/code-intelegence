"use client";

import { ChevronsLeft, ChevronsRight } from "lucide-react";
import { Button } from "~/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "~/components/ui/tooltip";

// ── Visible page numbers with ellipsis ───────────────────────────────────────

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

// ── Pagination ───────────────────────────────────────────────────────────────

const STEP = 5;

export function Pagination({
  page,
  pageSize,
  total,
  noun,
  onPageChange,
}: {
  page: number;
  pageSize: number;
  total: number;
  noun: string;
  onPageChange: (page: number) => void;
}) {
  const totalPages = Math.ceil(total / pageSize);
  if (totalPages <= 1) return null;

  const rangeStart = page * pageSize + 1;
  const rangeEnd = Math.min((page + 1) * pageSize, total);
  const showStep = totalPages > STEP;

  return (
    <TooltipProvider>
      <div className="flex items-center justify-between">
        <p className="text-muted-foreground text-sm">
          Showing {rangeStart}–{rangeEnd} of {total.toLocaleString()} {noun}
        </p>
        <div className="flex items-center gap-1">
          {showStep && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page < STEP}
                  onClick={() => onPageChange(Math.max(0, page - STEP))}
                >
                  <ChevronsLeft className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Back {STEP} pages</TooltipContent>
            </Tooltip>
          )}
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
                onClick={() => onPageChange(entry)}
              >
                {entry + 1}
              </Button>
            ),
          )}
          {showStep && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page + STEP >= totalPages}
                  onClick={() =>
                    onPageChange(Math.min(totalPages - 1, page + STEP))
                  }
                >
                  <ChevronsRight className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Forward {STEP} pages</TooltipContent>
            </Tooltip>
          )}
        </div>
      </div>
    </TooltipProvider>
  );
}
