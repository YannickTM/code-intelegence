"use client";

import { useState } from "react";
import { ChevronDown, ChevronRight, Folder, FolderOpen } from "lucide-react";
import { FileTreeView } from "./file-tree-view";
import { type FileNode } from "./file-tree-types";

export function FileTreeDirectory({
  node,
  selectedFilePath,
  onFileSelect,
  level,
}: {
  node: FileNode;
  selectedFilePath: string | null;
  onFileSelect: (path: string) => void;
  level: number;
}) {
  const [isOpen, setIsOpen] = useState(level === 0);

  return (
    <div>
      <button
        type="button"
        className="hover:bg-accent flex w-full items-center gap-1 rounded-md px-2 py-1 text-left text-sm font-medium"
        style={{ paddingLeft: level * 20 + 8 }}
        onClick={() => setIsOpen((v) => !v)}
      >
        {isOpen ? (
          <ChevronDown className="text-muted-foreground size-4 shrink-0" />
        ) : (
          <ChevronRight className="text-muted-foreground size-4 shrink-0" />
        )}
        <span className="text-muted-foreground">
          {isOpen ? (
            <FolderOpen className="size-4 shrink-0" />
          ) : (
            <Folder className="size-4 shrink-0" />
          )}
        </span>
        <span className="truncate">{node.name}</span>
      </button>
      {isOpen && node.children && (
        <FileTreeView
          nodes={node.children}
          selectedFilePath={selectedFilePath}
          onFileSelect={onFileSelect}
          level={level + 1}
        />
      )}
    </div>
  );
}
