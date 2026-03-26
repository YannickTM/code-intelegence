"use client";

import { useState } from "react";
import { AlertCircle, Blocks, RefreshCw } from "lucide-react";
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
import { api } from "~/trpc/react";

const DEFAULT_VISIBLE = 10;

function isJsxFile(language: string, filePath: string): boolean {
  const lang = language.toLowerCase();
  if (["jsx", "tsx", "javascriptreact", "typescriptreact"].includes(lang)) return true;
  return /\.(jsx|tsx)$/.test(filePath);
}

function scrollToLine(line: number) {
  document.getElementById(`L${line}`)?.scrollIntoView({ behavior: "smooth", block: "center" });
}

export function FileJsxUsagesCard({
  projectId,
  filePath,
  language,
}: {
  projectId: string;
  filePath: string;
  language: string;
}) {
  const [showAll, setShowAll] = useState(false);
  const enabled = isJsxFile(language, filePath);

  const query = api.projectFiles.fileJsxUsages.useQuery(
    { projectId, filePath },
    { retry: false, enabled },
  );

  if (!enabled) return null;

  const usages = query.data?.jsx_usages ?? [];

  // Hide card when loaded and empty
  if (!query.isLoading && !query.isError && usages.length === 0) return null;

  const customComponents = usages.filter((u) => !u.is_intrinsic && !u.is_fragment);
  const intrinsics = usages.filter((u) => u.is_intrinsic);
  const fragments = usages.filter((u) => u.is_fragment);

  const visibleCustom = showAll
    ? customComponents
    : customComponents.slice(0, DEFAULT_VISIBLE);
  const hasMore = customComponents.length > DEFAULT_VISIBLE;

  // Summarize intrinsic element names
  const intrinsicNames = [...new Set(intrinsics.map((u) => u.component_name))];
  const intrinsicPreview = intrinsicNames.slice(0, 4).join(", ");
  const intrinsicMore = intrinsicNames.length > 4 ? ", ..." : "";

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm font-medium">
          <Blocks className="size-4" />
          JSX Components
          {usages.length > 0 && (
            <Badge variant="secondary" className="text-xs">
              {usages.length}
            </Badge>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {query.isLoading && (
          <div className="flex flex-col gap-3">
            {Array.from({ length: 3 }, (_, i) => (
              <Skeleton key={i} className="h-4 w-full" />
            ))}
          </div>
        )}

        {query.isError && (
          <div className="flex flex-col gap-3">
            <Alert variant="destructive">
              <AlertCircle className="size-4" />
              <AlertDescription>Failed to load JSX usages.</AlertDescription>
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

        {usages.length > 0 && (
          <div className="flex flex-col gap-3">
            {/* Custom Components */}
            {customComponents.length > 0 && (
              <div className="flex flex-col gap-2">
                <p className="text-muted-foreground text-xs font-medium uppercase tracking-wide">
                  Custom Components ({customComponents.length})
                </p>
                <div className="flex flex-wrap gap-1.5">
                  {visibleCustom.map((u) => (
                    <button
                      key={u.id}
                      type="button"
                      onClick={() => scrollToLine(u.line)}
                      className="hover:opacity-80"
                      aria-label={`Go to ${u.component_name} at line ${u.line}`}
                    >
                      <Badge variant="outline" className="text-xs">
                        {u.component_name}
                        <span className="text-muted-foreground ml-1">
                          :{u.line}
                        </span>
                      </Badge>
                    </button>
                  ))}
                </div>
                {hasMore && (
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-auto py-1 text-xs"
                    onClick={() => setShowAll((v) => !v)}
                  >
                    {showAll
                      ? "Show less"
                      : `Show all ${customComponents.length} components`}
                  </Button>
                )}
              </div>
            )}

            {/* Fragments */}
            {fragments.length > 0 && (
              <p className="text-muted-foreground text-xs">
                {fragments.length} fragment{fragments.length !== 1 ? "s" : ""}
              </p>
            )}

            {/* Intrinsic Elements */}
            {intrinsics.length > 0 && (
              <p className="text-muted-foreground text-xs">
                {intrinsics.length} intrinsic element
                {intrinsics.length !== 1 ? "s" : ""} ({intrinsicPreview}
                {intrinsicMore})
              </p>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
