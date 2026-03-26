"use client";

import { useState } from "react";
import Link from "next/link";
import {
  AlertCircle,
  ExternalLink,
  Network,
  Package,
  RefreshCw,
} from "lucide-react";
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
import type { DependencyEdge } from "~/server/api/routers/project-files";
import { DependencyGraphDialog } from "./dependency-graph-dialog";

const DEFAULT_VISIBLE = 5;

export function FileDependenciesCard({
  projectId,
  filePath,
}: {
  projectId: string;
  filePath: string;
}) {
  const [showAllImports, setShowAllImports] = useState(false);
  const [showAllImportedBy, setShowAllImportedBy] = useState(false);
  const [graphOpen, setGraphOpen] = useState(false);

  const depsQuery = api.projectFiles.fileDependencies.useQuery(
    { projectId, filePath },
    { retry: false },
  );

  const totalCount =
    (depsQuery.data?.imports.length ?? 0) +
    (depsQuery.data?.imported_by.length ?? 0);

  return (
    <>
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-sm font-medium">
            <Network className="size-4" />
            Dependencies
            {depsQuery.data && totalCount > 0 && (
              <Badge variant="secondary" className="text-xs">
                {totalCount}
              </Badge>
            )}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {/* Loading */}
          {depsQuery.isLoading && (
            <div className="flex flex-col gap-3">
              {Array.from({ length: 3 }, (_, i) => (
                <div key={i} className="flex flex-col gap-1">
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-3 w-2/3" />
                </div>
              ))}
            </div>
          )}

          {/* Error */}
          {depsQuery.isError && (
            <div className="flex flex-col gap-3">
              <Alert variant="destructive">
                <AlertCircle className="size-4" />
                <AlertDescription>
                  Failed to load dependencies.
                </AlertDescription>
              </Alert>
              <Button
                variant="outline"
                size="sm"
                onClick={() => depsQuery.refetch()}
              >
                <RefreshCw className="size-4" />
                Retry
              </Button>
            </div>
          )}

          {/* Empty */}
          {!depsQuery.isLoading &&
            !depsQuery.isError &&
            depsQuery.data &&
            totalCount === 0 && (
              <p className="text-muted-foreground text-sm italic">
                No dependencies found for this file.
              </p>
            )}

          {/* Data */}
          {depsQuery.data && totalCount > 0 && (
            <div className="flex flex-col gap-4">
              {/* Imports section */}
              {depsQuery.data.imports.length > 0 && (
                <DependencySection
                  label="Imports"
                  edges={depsQuery.data.imports}
                  projectId={projectId}
                  direction="forward"
                  showAll={showAllImports}
                  onToggle={() => setShowAllImports((v) => !v)}
                />
              )}

              {/* Imported By section */}
              {depsQuery.data.imported_by.length > 0 && (
                <DependencySection
                  label="Imported By"
                  edges={depsQuery.data.imported_by}
                  projectId={projectId}
                  direction="reverse"
                  showAll={showAllImportedBy}
                  onToggle={() => setShowAllImportedBy((v) => !v)}
                />
              )}

              {/* Graph button */}
              <Button
                variant="outline"
                size="sm"
                className="w-full"
                onClick={() => setGraphOpen(true)}
              >
                <Network className="size-4" />
                View dependency graph
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      <DependencyGraphDialog
        projectId={projectId}
        filePath={filePath}
        open={graphOpen}
        onOpenChange={setGraphOpen}
      />
    </>
  );
}

function DependencySection({
  label,
  edges,
  projectId,
  direction,
  showAll,
  onToggle,
}: {
  label: string;
  edges: DependencyEdge[];
  projectId: string;
  direction: "forward" | "reverse";
  showAll: boolean;
  onToggle: () => void;
}) {
  const visible = showAll ? edges : edges.slice(0, DEFAULT_VISIBLE);
  const hasMore = edges.length > DEFAULT_VISIBLE;

  return (
    <div className="flex flex-col gap-2">
      <p className="text-muted-foreground text-xs font-medium uppercase tracking-wide">
        {label} ({edges.length})
      </p>
      <div className="flex flex-col gap-2">
        {visible.map((edge) => (
          <DependencyRow
            key={edge.id}
            edge={edge}
            projectId={projectId}
            direction={direction}
          />
        ))}
      </div>
      {hasMore && (
        <Button
          variant="ghost"
          size="sm"
          className="h-auto py-1 text-xs"
          onClick={onToggle}
        >
          {showAll ? "Show less" : `Show all (${edges.length})`}
        </Button>
      )}
    </div>
  );
}

function DependencyRow({
  edge,
  projectId,
  direction,
}: {
  edge: DependencyEdge;
  projectId: string;
  direction: "forward" | "reverse";
}) {
  const isExternal = edge.target_file_path === null;
  const filePath =
    direction === "forward"
      ? edge.target_file_path
      : edge.source_file_path;

  if (isExternal) {
    return (
      <div className="flex flex-col gap-0.5">
        <div className="flex items-center gap-2">
          <Package className="text-muted-foreground size-3.5 shrink-0" />
          <span className="truncate text-sm">
            {edge.package_name ?? edge.import_name}
          </span>
          <Badge variant="outline" className="shrink-0 text-[10px]">
            <ExternalLink className="size-2.5" />
            external
          </Badge>
        </div>
        <code className="text-muted-foreground truncate pl-5.5 font-mono text-xs">
          {edge.import_name}
        </code>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-0.5">
      <Link
        href={`/project/${projectId}/file?path=${encodeURIComponent(filePath!)}`}
        className="text-foreground hover:text-foreground/80 truncate text-sm hover:underline"
        title={filePath!}
      >
        {filePath!.split("/").pop()}
      </Link>
      <code className="text-muted-foreground truncate font-mono text-xs">
        {edge.import_name}
      </code>
    </div>
  );
}
