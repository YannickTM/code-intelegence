"use client";

import { Activity, AlertTriangle, FolderGit2, Zap } from "lucide-react";
import { Card } from "~/components/ui/card";
import { Skeleton } from "~/components/ui/skeleton";
import { cn } from "~/lib/utils";
import type { DashboardSummary } from "~/lib/dashboard-types";

type StatVariant = "default" | "info" | "destructive";

function StatCard({
  title,
  value,
  icon,
  subtitle,
  variant = "default",
  pulse = false,
}: {
  title: string;
  value: number;
  icon: React.ReactNode;
  subtitle?: string;
  variant?: StatVariant;
  pulse?: boolean;
}) {
  return (
    <Card
      className={cn(
        "relative overflow-hidden px-4 py-4",
        variant === "info" && "border-l-2 border-l-info",
        variant === "destructive" && "border-l-2 border-l-destructive",
      )}
    >
      <div className="flex items-start justify-between">
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {title}
          </span>
          <span className="text-3xl font-bold tabular-nums">{value}</span>
          {subtitle && (
            <span className="text-xs text-muted-foreground">{subtitle}</span>
          )}
        </div>
        <div
          className={cn(
            "text-muted-foreground",
            variant === "info" && "text-info",
            variant === "destructive" && "text-destructive",
            pulse && "animate-pulse",
          )}
        >
          {icon}
        </div>
      </div>
    </Card>
  );
}

export function HealthStrip({
  summary,
  isLoading,
}: {
  summary: DashboardSummary | undefined;
  isLoading: boolean;
}) {
  if (isLoading) {
    return (
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-24 rounded-xl" />
        ))}
      </div>
    );
  }

  if (!summary) return null;

  return (
    <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
      <StatCard
        title="Total Projects"
        value={summary.projects_total}
        icon={<FolderGit2 className="size-5" />}
      />
      <StatCard
        title="Active Jobs"
        value={summary.jobs_active}
        icon={<Activity className="size-5" />}
        variant={summary.jobs_active > 0 ? "info" : "default"}
        pulse={summary.jobs_active > 0}
      />
      <StatCard
        title="Failed (24h)"
        value={summary.jobs_failed_24h}
        icon={<AlertTriangle className="size-5" />}
        variant={summary.jobs_failed_24h > 0 ? "destructive" : "default"}
      />
      <StatCard
        title="Queries (24h)"
        value={summary.query_count_24h}
        icon={<Zap className="size-5" />}
        subtitle={`p95: ${summary.p95_latency_ms_24h}ms`}
      />
    </div>
  );
}
