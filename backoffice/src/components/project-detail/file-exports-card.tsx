"use client";

import { useState } from "react";
import { AlertCircle, RefreshCw, Upload } from "lucide-react";
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
import { api } from "~/trpc/react";
import type { FileExport } from "~/server/api/routers/project-files";

const DEFAULT_VISIBLE = 10;

const KIND_STYLES: Record<string, string> = {
  NAMED: "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300",
  DEFAULT: "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300",
  REEXPORT: "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300",
  EXPORT_ALL: "bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300",
  TYPE_ONLY: "bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300",
};

function scrollToLine(line: number) {
  document.getElementById(`L${line}`)?.scrollIntoView({ behavior: "smooth", block: "center" });
}

function ExportRow({ item }: { item: FileExport }) {
  const kindClass = KIND_STYLES[item.export_kind] ?? KIND_STYLES.NAMED!;

  return (
    <div className="flex items-start gap-2">
      <Badge variant="secondary" className={`shrink-0 text-[10px] ${kindClass}`}>
        {item.export_kind}
      </Badge>
      <div className="min-w-0 flex-1">
        <code className="text-sm font-mono break-all">{item.exported_name}</code>
        {item.source_module && (
          <span className="text-muted-foreground ml-1 text-xs">
            from &quot;{item.source_module}&quot;
          </span>
        )}
      </div>
      <button
        type="button"
        onClick={() => scrollToLine(item.line)}
        className="text-muted-foreground hover:text-foreground shrink-0 text-xs tabular-nums"
        aria-label={`Go to line ${item.line}`}
      >
        :{item.line}
      </button>
    </div>
  );
}

export function FileExportsCard({
  projectId,
  filePath,
}: {
  projectId: string;
  filePath: string;
}) {
  const [showAll, setShowAll] = useState(false);

  const query = api.projectFiles.fileExports.useQuery(
    { projectId, filePath },
    { retry: false },
  );

  const exports = query.data?.exports ?? [];

  // Hide card when loaded and empty
  if (!query.isLoading && !query.isError && exports.length === 0) return null;

  const visible = showAll ? exports : exports.slice(0, DEFAULT_VISIBLE);
  const hasMore = exports.length > DEFAULT_VISIBLE;

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm font-medium">
          <Upload className="size-4" />
          Exports
          {exports.length > 0 && (
            <Badge variant="secondary" className="text-xs">
              {exports.length}
            </Badge>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {query.isLoading && (
          <div className="flex flex-col gap-3">
            {Array.from({ length: 3 }, (_, i) => (
              <div key={i} className="flex items-center gap-2">
                <Skeleton className="h-4 w-16" />
                <Skeleton className="h-4 w-full" />
              </div>
            ))}
          </div>
        )}

        {query.isError && (
          <div className="flex flex-col gap-3">
            <Alert variant="destructive">
              <AlertCircle className="size-4" />
              <AlertDescription>Failed to load exports.</AlertDescription>
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

        {exports.length > 0 && (
          <div className="flex flex-col gap-2">
            {visible.map((item) => (
              <ExportRow key={item.id} item={item} />
            ))}
            {hasMore && (
              <Button
                variant="ghost"
                size="sm"
                className="h-auto py-1 text-xs"
                onClick={() => setShowAll((v) => !v)}
              >
                {showAll ? "Show less" : `Show all ${exports.length} exports`}
              </Button>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
