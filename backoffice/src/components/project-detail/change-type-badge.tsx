import { Badge } from "~/components/ui/badge";

export function ChangeTypeBadge({ type }: { type: string }) {
  switch (type) {
    case "added":
      return (
        <Badge className="bg-success/10 text-success">
          Added
        </Badge>
      );
    case "deleted":
      return <Badge variant="destructive">Deleted</Badge>;
    case "modified":
      return <Badge variant="outline">Modified</Badge>;
    case "renamed":
      return (
        <Badge className="bg-warning/10 text-warning">
          Renamed
        </Badge>
      );
    case "copied":
      return (
        <Badge variant="secondary">Copied</Badge>
      );
    default:
      return <Badge variant="outline">{type}</Badge>;
  }
}
