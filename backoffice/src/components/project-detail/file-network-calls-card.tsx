"use client";

import { useState } from "react";
import { AlertCircle, Globe, RefreshCw } from "lucide-react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import { Skeleton } from "~/components/ui/skeleton";
import { Alert, AlertDescription } from "~/components/ui/alert";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "~/components/ui/tooltip";
import { api } from "~/trpc/react";
import type { NetworkCall } from "~/server/api/routers/project-files";

const DEFAULT_VISIBLE = 10;

const METHOD_STYLES: Record<string, string> = {
  GET: "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300",
  POST: "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300",
  PUT: "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300",
  DELETE: "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300",
  PATCH: "bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300",
  UNKNOWN: "bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300",
};

function scrollToLine(line: number) {
  document.getElementById(`L${line}`)?.scrollIntoView({ behavior: "smooth", block: "center" });
}

function NetworkCallRow({ call }: { call: NetworkCall }) {
  const methodClass = METHOD_STYLES[call.method] ?? METHOD_STYLES.UNKNOWN!;
  const url = call.url_literal ?? call.url_template ?? "";

  return (
    <div className="flex items-start gap-2">
      <Badge variant="secondary" className={`shrink-0 text-[10px] ${methodClass}`}>
        {call.method}
      </Badge>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <span className="text-muted-foreground text-[10px] lowercase">
            {call.client_kind}
          </span>
          {call.is_relative && (
            <Badge variant="outline" className="text-[9px]">
              relative
            </Badge>
          )}
        </div>
        {url && (
          <Tooltip>
            <TooltipTrigger asChild>
              <code className="text-muted-foreground block truncate font-mono text-xs">
                {url}
              </code>
            </TooltipTrigger>
            <TooltipContent side="left" className="max-w-xs break-all">
              {url}
            </TooltipContent>
          </Tooltip>
        )}
      </div>
      <button
        type="button"
        onClick={() => scrollToLine(call.start_line)}
        className="text-muted-foreground hover:text-foreground shrink-0 text-xs tabular-nums"
        aria-label={`Go to line ${call.start_line}`}
      >
        :{call.start_line}
      </button>
    </div>
  );
}

export function FileNetworkCallsCard({
  projectId,
  filePath,
}: {
  projectId: string;
  filePath: string;
}) {
  const [showAll, setShowAll] = useState(false);

  const query = api.projectFiles.fileNetworkCalls.useQuery(
    { projectId, filePath },
    { retry: false },
  );

  const calls = query.data?.network_calls ?? [];

  // Hide card when loaded and empty
  if (!query.isLoading && !query.isError && calls.length === 0) return null;

  const visible = showAll ? calls : calls.slice(0, DEFAULT_VISIBLE);
  const hasMore = calls.length > DEFAULT_VISIBLE;

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm font-medium">
          <Globe className="size-4" />
          API Calls
          {calls.length > 0 && (
            <Badge variant="secondary" className="text-xs">
              {calls.length}
            </Badge>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {query.isLoading && (
          <div className="flex flex-col gap-3">
            {Array.from({ length: 3 }, (_, i) => (
              <div key={i} className="flex items-center gap-2">
                <Skeleton className="h-4 w-12" />
                <Skeleton className="h-4 w-full" />
              </div>
            ))}
          </div>
        )}

        {query.isError && (
          <div className="flex flex-col gap-3">
            <Alert variant="destructive">
              <AlertCircle className="size-4" />
              <AlertDescription>
                Failed to load network calls.
              </AlertDescription>
            </Alert>
            <Button
              variant="outline"
              size="sm"
              onClick={() => query.refetch()}
            >
              <RefreshCw className="size-4" />
              Retry
            </Button>
          </div>
        )}

        {calls.length > 0 && (
          <div className="flex flex-col gap-2">
            {visible.map((call) => (
              <NetworkCallRow key={call.id} call={call} />
            ))}
            {hasMore && (
              <Button
                variant="ghost"
                size="sm"
                className="h-auto py-1 text-xs"
                onClick={() => setShowAll((v) => !v)}
              >
                {showAll ? "Show less" : `Show all ${calls.length} calls`}
              </Button>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
