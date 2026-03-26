import {
  File,
  FileCode,
  FileJson,
  FileText,
  Terminal,
} from "lucide-react";

const ICON_MAP: Record<string, typeof File> = {
  ".ts": FileCode,
  ".tsx": FileCode,
  ".js": FileCode,
  ".jsx": FileCode,
  ".go": FileCode,
  ".py": FileCode,
  ".rs": FileCode,
  ".java": FileCode,
  ".rb": FileCode,
  ".c": FileCode,
  ".cpp": FileCode,
  ".h": FileCode,
  ".yaml": FileCode,
  ".yml": FileCode,
  ".toml": FileCode,
  ".xml": FileCode,
  ".html": FileCode,
  ".css": FileCode,
  ".scss": FileCode,
  ".json": FileJson,
  ".md": FileText,
  ".txt": FileText,
  ".rst": FileText,
  ".sh": Terminal,
  ".bash": Terminal,
  ".zsh": Terminal,
};

export function getFileIcon(fileName: string) {
  const raw = fileName.includes(".") ? `.${fileName.split(".").pop()}` : "";
  const ext = raw.toLowerCase();
  const Icon = ICON_MAP[ext] ?? File;
  return <Icon className="size-4 shrink-0" />;
}
