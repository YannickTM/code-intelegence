"use client";

import { useEffect } from "react";
import { AlertCircle, RefreshCw } from "lucide-react";
import { Alert, AlertDescription } from "~/components/ui/alert";
import { Button } from "~/components/ui/button";
import { api } from "~/trpc/react";
import { type FileNode } from "./file-tree-types";
import { FileTreeSkeleton } from "./file-tree-skeleton";
import { FileTreeEmpty } from "./file-tree-empty";
import { FileTreeView } from "./file-tree-view";

function treeContainsPath(nodes: FileNode[], path: string): boolean {
  for (const node of nodes) {
    if (node.path === path) return true;
    if (node.children && treeContainsPath(node.children, path)) return true;
  }
  return false;
}

export function FileBrowser({
  projectId,
  selectedFilePath,
  onFileSelect,
}: {
  projectId: string;
  selectedFilePath: string | null;
  onFileSelect: (path: string | null) => void;
}) {
  const structureQuery = api.projects.structure.useQuery(
    { id: projectId },
    { retry: false },
  );

  // Clear selection when the selected path no longer exists in the tree
  // (covers: file removed, structure empty, NOT_FOUND error)
  const children = structureQuery.data?.root.children;
  const isSettled = !structureQuery.isLoading;
  useEffect(() => {
    if (!selectedFilePath || !isSettled) return;
    if (
      !children ||
      !treeContainsPath(children, selectedFilePath)
    ) {
      onFileSelect(null);
    }
  }, [children, isSettled, selectedFilePath, onFileSelect]);

  // Loading
  if (structureQuery.isLoading) {
    return (
      <div className="rounded-xl border">
        <FileTreeSkeleton />
      </div>
    );
  }

  // NOT_FOUND → treat as empty (project has no snapshot yet)
  if (
    structureQuery.isError &&
    structureQuery.error?.data?.code === "NOT_FOUND"
  ) {
    return (
      <div className="rounded-xl border">
        <FileTreeEmpty projectId={projectId} />
      </div>
    );
  }

  // Other errors
  if (structureQuery.isError) {
    return (
      <div className="flex min-h-[400px] flex-col gap-4 rounded-xl border p-4">
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertDescription>Failed to load file structure.</AlertDescription>
        </Alert>
        <div className="flex justify-center">
          <Button
            variant="outline"
            size="sm"
            onClick={() => structureQuery.refetch()}
          >
            <RefreshCw className="size-4" />
            Retry
          </Button>
        </div>
      </div>
    );
  }

  const data = structureQuery.data;

  // Empty (no files or no children)
  if (!data || data.file_count === 0 || !data.root.children?.length) {
    return (
      <div className="rounded-xl border">
        <FileTreeEmpty projectId={projectId} />
      </div>
    );
  }

  // Tree
  return (
    <div className="rounded-xl border">
      <div className="p-2">
        <FileTreeView
          nodes={data.root.children}
          selectedFilePath={selectedFilePath}
          onFileSelect={onFileSelect}
        />
      </div>
    </div>
  );
}
