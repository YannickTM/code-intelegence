"use client";

import { useState } from "react";
import { AlertTriangle, Info, XCircle } from "lucide-react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import type { FileIssue } from "~/server/api/routers/project-files";

const DEFAULT_VISIBLE = 10;

const SEVERITY_ORDER: Record<string, number> = { error: 0, warning: 1, info: 2 };

function SeverityIcon({ severity }: { severity: string }) {
  switch (severity) {
    case "error":
      return <XCircle className="size-4 shrink-0 text-red-500" />;
    case "warning":
      return <AlertTriangle className="size-4 shrink-0 text-amber-500" />;
    default:
      return <Info className="size-4 shrink-0 text-blue-500" />;
  }
}

function scrollToLine(line: number) {
  document.getElementById(`L${line}`)?.scrollIntoView({ behavior: "smooth", block: "center" });
}

export function FileDiagnosticsCard({
  issues,
}: {
  issues: FileIssue[] | null | undefined;
}) {
  const [showAll, setShowAll] = useState(false);

  if (!issues || issues.length === 0) return null;

  const sorted = [...issues].sort(
    (a, b) => (SEVERITY_ORDER[a.severity] ?? 2) - (SEVERITY_ORDER[b.severity] ?? 2),
  );

  const hasErrors = sorted.some((i) => i.severity === "error");
  const hasWarnings = sorted.some((i) => i.severity === "warning");

  const badgeClass = hasErrors
    ? "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300"
    : hasWarnings
      ? "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300"
      : "";

  const visible = showAll ? sorted : sorted.slice(0, DEFAULT_VISIBLE);
  const hasMore = sorted.length > DEFAULT_VISIBLE;

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm font-medium">
          <AlertTriangle className="size-4" />
          Diagnostics
          <Badge
            variant="secondary"
            className={`text-xs ${badgeClass}`}
          >
            {issues.length}
          </Badge>
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex flex-col gap-2">
          {visible.map((issue, idx) => (
            <div key={idx} className="flex items-start gap-2">
              <SeverityIcon severity={issue.severity} />
              <div className="min-w-0 flex-1">
                <code className="text-xs font-mono font-bold">
                  {issue.code}
                </code>
                <p className="text-muted-foreground text-xs">
                  {issue.message}
                </p>
              </div>
              <button
                type="button"
                onClick={() => scrollToLine(issue.line)}
                className="text-muted-foreground hover:text-foreground shrink-0 text-xs tabular-nums"
                aria-label={`Go to line ${issue.line}`}
              >
                :{issue.line}
              </button>
            </div>
          ))}
          {hasMore && (
            <Button
              variant="ghost"
              size="sm"
              className="h-auto py-1 text-xs"
              onClick={() => setShowAll((v) => !v)}
            >
              {showAll ? "Show less" : `Show all ${sorted.length} issues`}
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
