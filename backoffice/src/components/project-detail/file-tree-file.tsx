"use client";

import { cn } from "~/lib/utils";
import { getFileIcon } from "./file-icon";
import { type FileNode } from "./file-tree-types";

export function FileTreeFile({
  node,
  isSelected,
  onSelect,
  level,
}: {
  node: FileNode;
  isSelected: boolean;
  onSelect: (path: string) => void;
  level: number;
}) {
  return (
    <button
      type="button"
      className={cn(
        "hover:bg-accent flex w-full items-center gap-2 rounded-md px-2 py-1 text-left text-sm",
        isSelected && "bg-accent",
      )}
      style={{ paddingLeft: level * 20 + 8 }}
      onClick={() => onSelect(node.path)}
    >
      <span className="text-muted-foreground">{getFileIcon(node.name)}</span>
      <span className="truncate">{node.name}</span>
    </button>
  );
}
