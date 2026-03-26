"use client";

import { useState } from "react";
import {
  AlertCircle,
  ChevronDown,
  ChevronRight,
  Link,
  RefreshCw,
} from "lucide-react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import { Skeleton } from "~/components/ui/skeleton";
import { Alert, AlertDescription } from "~/components/ui/alert";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "~/components/ui/collapsible";
import { api } from "~/trpc/react";
import type { SymbolReference } from "~/server/api/routers/project-files";

const DEFAULT_VISIBLE = 5;

const KIND_GROUPS: Record<string, string> = {
  CALL: "Function Calls",
  TYPE_REF: "Type References",
  JSX_RENDER: "JSX Renders",
  EXTENDS: "Inheritance",
  IMPLEMENTS: "Inheritance",
};

function scrollToLine(line: number) {
  document.getElementById(`L${line}`)?.scrollIntoView({ behavior: "smooth", block: "center" });
}

function groupReferences(refs: SymbolReference[]) {
  const groups = new Map<string, SymbolReference[]>();

  for (const ref of refs) {
    const label = KIND_GROUPS[ref.reference_kind] ?? "Other";
    const existing = groups.get(label);
    if (existing) {
      existing.push(ref);
    } else {
      groups.set(label, [ref]);
    }
  }

  return groups;
}

function ReferenceGroup({
  label,
  refs,
}: {
  label: string;
  refs: SymbolReference[];
}) {
  const [open, setOpen] = useState(true);
  const [showAll, setShowAll] = useState(false);

  const visible = showAll ? refs : refs.slice(0, DEFAULT_VISIBLE);
  const hasMore = refs.length > DEFAULT_VISIBLE;

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="text-muted-foreground hover:text-foreground flex w-full items-center gap-1 text-xs font-medium uppercase tracking-wide"
        >
          {open ? (
            <ChevronDown className="size-3" />
          ) : (
            <ChevronRight className="size-3" />
          )}
          {label} ({refs.length})
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mt-1 flex flex-col gap-1.5 pl-4">
          {visible.map((ref) => (
            <div key={ref.id} className="flex items-center gap-2">
              <code className="min-w-0 flex-1 truncate font-mono text-sm">
                {ref.target_name}
              </code>
              <button
                type="button"
                onClick={() => scrollToLine(ref.start_line)}
                className="text-muted-foreground hover:text-foreground shrink-0 text-xs tabular-nums"
                aria-label={`Go to line ${ref.start_line}`}
              >
                :{ref.start_line}
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
              {showAll ? "Show less" : `Show all (${refs.length})`}
            </Button>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

export function FileReferencesCard({
  projectId,
  filePath,
}: {
  projectId: string;
  filePath: string;
}) {
  const query = api.projectFiles.fileReferences.useQuery(
    { projectId, filePath },
    { retry: false },
  );

  const references = query.data?.references ?? [];

  // Hide card when loaded and empty
  if (!query.isLoading && !query.isError && references.length === 0) return null;

  const groups = groupReferences(references);

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm font-medium">
          <Link className="size-4" />
          References
          {references.length > 0 && (
            <Badge variant="secondary" className="text-xs">
              {references.length}
            </Badge>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {query.isLoading && (
          <div className="flex flex-col gap-3">
            {Array.from({ length: 3 }, (_, i) => (
              <div key={i} className="flex flex-col gap-1">
                <Skeleton className="h-3 w-24" />
                <Skeleton className="h-4 w-full" />
                <Skeleton className="h-4 w-2/3" />
              </div>
            ))}
          </div>
        )}

        {query.isError && (
          <div className="flex flex-col gap-3">
            <Alert variant="destructive">
              <AlertCircle className="size-4" />
              <AlertDescription>Failed to load references.</AlertDescription>
            </Alert>
            <Button
              variant="outline"
              size="sm"
              onClick={() => query.refetch()}
            >
              <RefreshCw className="size-4" />
              Retry
            </Button>
          </div>
        )}

        {references.length > 0 && (
          <div className="flex flex-col gap-3">
            {Array.from(groups.entries()).map(([label, refs]) => (
              <ReferenceGroup key={label} label={label} refs={refs} />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
