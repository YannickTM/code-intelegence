"use client";

import { useReducer, useState } from "react";
import Link from "next/link";
import { AlertCircle, History, RefreshCw } from "lucide-react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
import { Button } from "~/components/ui/button";
import { Skeleton } from "~/components/ui/skeleton";
import { Alert, AlertDescription } from "~/components/ui/alert";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "~/components/ui/tooltip";
import { api } from "~/trpc/react";
import { formatRelativeTime } from "~/lib/format";
import { ChangeTypeBadge } from "./change-type-badge";
import type { FileHistoryEntry } from "~/server/api/routers/project-files";

const INITIAL_VISIBLE = 3;
const PAGE_SIZE = 10;
const MAX_VISIBLE = 50;

type State = {
  pages: FileHistoryEntry[][];
  total: number | null;
  currentPage: number;
};

type Action =
  | { type: "LOAD_MORE" }
  | { type: "SET_PAGE"; page: number; items: FileHistoryEntry[]; total: number };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "LOAD_MORE":
      return { ...state, currentPage: state.currentPage + 1 };
    case "SET_PAGE":
      return {
        ...state,
        total: action.total,
        pages: [...state.pages.slice(0, action.page), action.items],
      };
  }
}

export function FileHistoryCard({
  projectId,
  filePath,
}: {
  projectId: string;
  filePath: string;
}) {
  const [state, dispatch] = useReducer(reducer, {
    pages: [],
    total: null,
    currentPage: 0,
  });

  const offset = state.currentPage * PAGE_SIZE;
  // Only fetch if we haven't already loaded this page
  const needsFetch = !state.pages[state.currentPage];

  const historyQuery = api.projectFiles.fileHistory.useQuery(
    { projectId, filePath, limit: PAGE_SIZE, offset },
    {
      retry: false,
      enabled: needsFetch,
    },
  );

  // When data arrives, store it in the reducer
  if (historyQuery.data && needsFetch) {
    dispatch({
      type: "SET_PAGE",
      page: state.currentPage,
      items: historyQuery.data.items,
      total: historyQuery.data.total,
    });
  }

  const allItems = state.pages.flat();
  const [expanded, setExpanded] = useState(false);
  const visibleItems = expanded ? allItems : allItems.slice(0, INITIAL_VISIBLE);
  const hasMore =
    state.total !== null &&
    allItems.length < state.total &&
    allItems.length < MAX_VISIBLE;

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm font-medium">
          <History className="size-4" />
          Editorial History
        </CardTitle>
      </CardHeader>
      <CardContent>
        {/* Loading (initial) */}
        {historyQuery.isLoading && state.currentPage === 0 && (
          <div className="flex flex-col gap-3">
            {Array.from({ length: 4 }, (_, i) => (
              <div key={i} className="flex flex-col gap-1">
                <Skeleton className="h-4 w-full" />
                <Skeleton className="h-3 w-2/3" />
              </div>
            ))}
          </div>
        )}

        {/* Error */}
        {historyQuery.isError && (
          <div className="flex flex-col gap-3">
            <Alert variant="destructive">
              <AlertCircle className="size-4" />
              <AlertDescription>
                Failed to load file history.
              </AlertDescription>
            </Alert>
            <Button
              variant="outline"
              size="sm"
              onClick={() => historyQuery.refetch()}
            >
              <RefreshCw className="size-4" />
              Retry
            </Button>
          </div>
        )}

        {/* Empty */}
        {!historyQuery.isLoading &&
          !historyQuery.isError &&
          allItems.length === 0 && (
            <p className="text-muted-foreground text-sm italic">
              No commit history available for this file.
            </p>
          )}

        {/* Entries */}
        {allItems.length > 0 && (
          <div className="flex flex-col gap-3">
            {visibleItems.map((entry) => (
              <HistoryEntry
                key={entry.diff_id}
                entry={entry}
                projectId={projectId}
              />
            ))}

            {/* Expand collapsed entries */}
            {!expanded && allItems.length > INITIAL_VISIBLE && (
              <Button
                variant="ghost"
                size="sm"
                className="h-auto py-1 text-xs"
                onClick={() => setExpanded(true)}
              >
                Load more
              </Button>
            )}

            {/* Load more pages from server */}
            {expanded && hasMore && (
              <Button
                variant="ghost"
                size="sm"
                className="h-auto py-1 text-xs"
                onClick={() => dispatch({ type: "LOAD_MORE" })}
                disabled={historyQuery.isFetching}
              >
                {historyQuery.isFetching ? "Loading..." : "Load more"}
              </Button>
            )}

            {/* Cap reached */}
            {state.total !== null &&
              state.total > MAX_VISIBLE &&
              allItems.length >= MAX_VISIBLE && (
                <Button variant="link" size="sm" className="w-full" asChild>
                  <Link href={`/project/${projectId}/commits`}>
                    View all in commits
                  </Link>
                </Button>
              )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function HistoryEntry({
  entry,
  projectId,
}: {
  entry: FileHistoryEntry;
  projectId: string;
}) {
  return (
    <div className="flex flex-col gap-1 text-sm">
      <div className="flex items-center justify-between gap-2">
        <Link
          href={`/project/${projectId}/commits/${entry.commit_hash}`}
          className="font-mono text-xs text-muted-foreground hover:text-foreground"
        >
          {entry.short_hash}
        </Link>
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="text-xs text-muted-foreground">
              {formatRelativeTime(entry.committer_date)}
            </span>
          </TooltipTrigger>
          <TooltipContent>
            {new Date(entry.committer_date).toLocaleString()}
          </TooltipContent>
        </Tooltip>
      </div>
      <Link
        href={`/project/${projectId}/commits/${entry.commit_hash}`}
        className="truncate text-sm hover:underline"
        title={entry.message_subject}
      >
        {entry.message_subject}
      </Link>
      <div className="flex items-center gap-2">
        <span className="text-xs text-muted-foreground">
          {entry.author_name}
        </span>
        <ChangeTypeBadge type={entry.change_type} />
        <span className="text-xs tabular-nums text-green-600 dark:text-green-400">
          +{entry.additions}
        </span>
        <span className="text-xs tabular-nums text-red-600 dark:text-red-400">
          -{entry.deletions}
        </span>
      </div>
    </div>
  );
}
