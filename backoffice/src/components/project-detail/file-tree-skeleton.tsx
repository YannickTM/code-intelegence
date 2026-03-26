import { Skeleton } from "~/components/ui/skeleton";

const ROWS = [
  { indent: 0, width: "w-28" },
  { indent: 1, width: "w-24" },
  { indent: 1, width: "w-32" },
  { indent: 2, width: "w-20" },
  { indent: 2, width: "w-36" },
  { indent: 0, width: "w-24" },
  { indent: 0, width: "w-20" },
  { indent: 1, width: "w-28" },
  { indent: 1, width: "w-32" },
  { indent: 0, width: "w-16" },
];

export function FileTreeSkeleton() {
  return (
    <div className="flex flex-col gap-1 p-3">
      {ROWS.map((row, i) => (
        <div
          key={i}
          className="flex items-center gap-2 py-1"
          style={{ paddingLeft: row.indent * 20 }}
        >
          <Skeleton className="size-4 rounded" />
          <Skeleton className={`h-4 ${row.width} rounded`} />
        </div>
      ))}
    </div>
  );
}
