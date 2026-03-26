import { Skeleton } from "~/components/ui/skeleton";

export function ProjectDetailSkeleton() {
  return (
    <div className="flex flex-col gap-0">
      {/* Header skeleton */}
      <div className="flex flex-col gap-3 pb-4">
        <div className="flex items-start justify-between gap-4">
          <div className="flex flex-col gap-2">
            <Skeleton className="h-8 w-48" />
            <Skeleton className="h-4 w-72" />
            <div className="mt-1 flex items-center gap-2">
              <Skeleton className="h-5 w-16 rounded-full" />
              <Skeleton className="h-4 w-20" />
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Skeleton className="h-8 w-28" />
            <Skeleton className="size-8" />
          </div>
        </div>
      </div>

      {/* Tab nav skeleton */}
      <div className="border-b">
        <div className="flex gap-0">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="mx-4 my-2 h-5 w-16" />
          ))}
        </div>
      </div>

      {/* Content skeleton */}
      <div className="pt-6">
        <Skeleton className="h-64 w-full rounded-xl" />
      </div>
    </div>
  );
}
