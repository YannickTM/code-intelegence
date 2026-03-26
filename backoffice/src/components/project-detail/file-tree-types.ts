export type FileNode = {
  path: string;
  name: string;
  node_type: "file" | "directory";
  children?: FileNode[];
  language?: string;
  size_bytes?: number;
};
