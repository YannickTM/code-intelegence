import { TreePine } from "lucide-react";

export function Logo() {
  return (
    <div className="flex items-center gap-2">
      <TreePine className="size-5 text-primary" />
      <span className="text-lg font-bold tracking-tight">MYJUNGLE</span>
    </div>
  );
}
