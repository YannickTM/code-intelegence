import { FileTreeDirectory } from "./file-tree-directory";
import { FileTreeFile } from "./file-tree-file";
import { type FileNode } from "./file-tree-types";

function sortNodes(nodes: FileNode[]): FileNode[] {
  return [...nodes].sort((a, b) => {
    if (a.node_type !== b.node_type) {
      return a.node_type === "directory" ? -1 : 1;
    }
    return a.name.localeCompare(b.name);
  });
}

export function FileTreeView({
  nodes,
  selectedFilePath,
  onFileSelect,
  level = 0,
}: {
  nodes: FileNode[];
  selectedFilePath: string | null;
  onFileSelect: (path: string) => void;
  level?: number;
}) {
  const sorted = sortNodes(nodes);

  return (
    <div className="flex flex-col">
      {sorted.map((node) =>
        node.node_type === "directory" ? (
          <FileTreeDirectory
            key={node.path}
            node={node}
            selectedFilePath={selectedFilePath}
            onFileSelect={onFileSelect}
            level={level}
          />
        ) : (
          <FileTreeFile
            key={node.path}
            node={node}
            isSelected={selectedFilePath === node.path}
            onSelect={onFileSelect}
            level={level}
          />
        ),
      )}
    </div>
  );
}
