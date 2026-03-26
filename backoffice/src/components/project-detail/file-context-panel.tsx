"use client";

import Link from "next/link";
import { AlertCircle, ExternalLink, RefreshCw, X } from "lucide-react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardAction,
} from "~/components/ui/card";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import { Skeleton } from "~/components/ui/skeleton";
import { Alert, AlertDescription } from "~/components/ui/alert";
import { api } from "~/trpc/react";
import { formatBytes, formatRelativeTime } from "~/lib/format";

export function FileContextPanel({
  projectId,
  filePath,
  onClose,
}: {
  projectId: string;
  filePath: string;
  onClose: () => void;
}) {
  const fileQuery = api.projectSearch.fileMetadata.useQuery(
    { projectId, file_path: filePath },
    { retry: false },
  );

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-medium">File Info</CardTitle>
        <CardAction>
          <Button variant="ghost" size="icon-xs" onClick={onClose}>
            <X className="size-4" />
            <span className="sr-only">Close</span>
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        {fileQuery.isLoading && (
          <div className="flex flex-col gap-3">
            <Skeleton className="h-5 w-full" />
            <Skeleton className="h-4 w-24" />
            <Skeleton className="h-4 w-20" />
            <Skeleton className="h-4 w-28" />
          </div>
        )}

        {fileQuery.isError && (
          <div className="flex flex-col gap-3">
            <Alert variant="destructive">
              <AlertCircle className="size-4" />
              <AlertDescription>Failed to load file info.</AlertDescription>
            </Alert>
            <Button
              variant="outline"
              size="sm"
              onClick={() => fileQuery.refetch()}
            >
              <RefreshCw className="size-4" />
              Retry
            </Button>
          </div>
        )}

        {fileQuery.data && (
          <div className="flex flex-col gap-3">
            <code className="bg-muted rounded px-2 py-1 text-xs break-all">
              {fileQuery.data.file_path}
            </code>

            <div className="flex flex-col gap-2 text-sm">
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Language</span>
                <Badge variant="outline" className="text-xs">
                  {fileQuery.data.language || "unknown"}
                </Badge>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Size</span>
                <span>{formatBytes(fileQuery.data.size_bytes)}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Lines</span>
                <span>{fileQuery.data.line_count.toLocaleString()}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Last indexed</span>
                <span title={fileQuery.data.last_indexed_at}>
                  {formatRelativeTime(fileQuery.data.last_indexed_at)}
                </span>
              </div>
            </div>

            <Button variant="outline" size="sm" className="w-full" asChild>
              <Link
                href={`/project/${projectId}/search?file=${encodeURIComponent(filePath)}`}
              >
                <ExternalLink className="size-4" />
                View in search results
              </Link>
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
