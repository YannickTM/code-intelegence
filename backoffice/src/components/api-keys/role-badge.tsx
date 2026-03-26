import { Badge } from "~/components/ui/badge";

export function RoleBadge({ role }: { role: "read" | "write" }) {
  if (role === "write") {
    return (
      <Badge
        variant="outline"
        className="border-amber-500/40 text-amber-600 dark:text-amber-400"
      >
        write
      </Badge>
    );
  }

  return <Badge variant="outline">read</Badge>;
}
