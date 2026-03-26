"use client";

import { useState, useCallback } from "react";
import Link from "next/link";
import {
  AlertCircle,
  ArrowLeft,
  Check,
  Copy,
  FileX,
  Loader2,
  RefreshCw,
  Sparkles,
} from "lucide-react";
import { toast } from "sonner";
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
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "~/components/ui/tooltip";
import { api } from "~/trpc/react";
import { formatBytes, formatRelativeTime } from "~/lib/format";
import { CodeViewerWithPreview } from "./code-viewer-with-preview";
import { FileHistoryCard } from "./file-history-card";
import { FileDependenciesCard } from "./file-dependencies-card";
import { FileFactsCard } from "./file-facts-card";
import { FileExportsCard } from "./file-exports-card";
import { FileDiagnosticsCard } from "./file-diagnostics-card";
import { FileReferencesCard } from "./file-references-card";
import { FileJsxUsagesCard } from "./file-jsx-usages-card";
import { FileNetworkCallsCard } from "./file-network-calls-card";

// ── Copy button ─────────────────────────────────────────────────────────────

function CopyButton({ text, label }: { text: string; label: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(text).then(
      () => {
        toast.success(`${label} copied`);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      },
      (err: unknown) => {
        toast.error(err instanceof Error ? err.message : "Failed to copy");
      },
    );
  }, [text, label]);

  return (
    <button
      type="button"
      onClick={handleCopy}
      className="text-muted-foreground hover:text-foreground rounded p-0.5 transition-colors"
      aria-label={`Copy ${label}`}
    >
      {copied ? (
        <Check className="size-3.5" />
      ) : (
        <Copy className="size-3.5" />
      )}
    </button>
  );
}

// ── File path breadcrumb ────────────────────────────────────────────────────

function FilePathBreadcrumb({
  filePath,
  language,
}: {
  filePath: string;
  language: string;
}) {
  const segments = filePath.split("/");
  return (
    <div className="flex flex-wrap items-center gap-1">
      <code className="font-mono text-sm">
        {segments.map((segment, i) => (
          <span key={i}>
            {i > 0 && (
              <span className="text-muted-foreground mx-1">/</span>
            )}
            <span className={i === segments.length - 1 ? "font-semibold" : ""}>
              {segment}
            </span>
          </span>
        ))}
      </code>
      <Badge variant="outline" className="ml-2 text-xs">
        {language || "unknown"}
      </Badge>
    </div>
  );
}

// ── AI Description card (mocked) ────────────────────────────────────────────

function AiDescriptionCard() {
  const [state, setState] = useState<"idle" | "loading" | "done">("idle");

  const handleRequest = useCallback(() => {
    setState("loading");
    setTimeout(() => setState("done"), 2000);
  }, []);

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm font-medium">
          <Sparkles className="size-4" />
          AI Description
        </CardTitle>
      </CardHeader>
      <CardContent>
        {state === "idle" && (
          <div className="flex flex-col gap-2">
            <p className="text-muted-foreground text-sm">
              Generate an AI-powered summary of this file.
            </p>
            <Button variant="outline" size="sm" className="w-full" onClick={handleRequest}>
              <Sparkles className="size-4" />
              Request AI Description
            </Button>
          </div>
        )}
        {state === "loading" && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="size-4 animate-spin" />
            Generating description…
          </div>
        )}
        {state === "done" && (
          <div className="flex flex-col gap-2">
            <p className="text-sm">
              This file implements the main file detail view with a two-column
              layout: a syntax-highlighted code viewer on the left and analysis
              cards on the right. It fetches file content via tRPC, handles
              loading and error states, and renders metadata, breadcrumbs, and
              multiple sidebar cards for exports, dependencies, diagnostics, and
              more.
            </p>
            <Button
              variant="ghost"
              size="sm"
              className="h-auto py-1 text-xs"
              onClick={() => setState("idle")}
            >
              Regenerate
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

// ── Main component ──────────────────────────────────────────────────────────

export function FileViewerContent({
  projectId,
  filePath,
}: {
  projectId: string;
  filePath: string;
}) {
  const fileQuery = api.projectFiles.fileContent.useQuery(
    { projectId, filePath },
    { retry: false },
  );

  const backHref = `/project/${projectId}`;

  // ── Loading ─────────────────────────────────────────────────────────────

  if (fileQuery.isLoading) {
    return (
      <div className="flex flex-col gap-6">
        <Skeleton className="h-4 w-32" />
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1fr_340px]">
          <div className="min-w-0 flex flex-col gap-4">
            <Skeleton className="h-5 w-64" />
            <Skeleton className="h-4 w-48" />
            <Skeleton className="h-[400px] w-full rounded-md" />
          </div>
          <div className="flex flex-col gap-4">
            {Array.from({ length: 4 }, (_, i) => (
              <div key={i} className="flex flex-col gap-2 rounded-lg border p-4">
                <Skeleton className="h-4 w-24" />
                <Skeleton className="h-4 w-full" />
                <Skeleton className="h-4 w-2/3" />
              </div>
            ))}
          </div>
        </div>
      </div>
    );
  }

  // ── Error / Not found ─────────────────────────────────────────────────

  if (fileQuery.isError) {
    const isNotFound = fileQuery.error?.data?.code === "NOT_FOUND";

    if (isNotFound) {
      return (
        <div className="flex flex-col items-center justify-center gap-4 py-20">
          <FileX className="text-muted-foreground size-12" />
          <h2 className="text-lg font-semibold">File not found</h2>
          <p className="text-muted-foreground text-sm">
            This file may not be indexed yet, or the path may have changed.
          </p>
          <Button variant="outline" asChild>
            <Link href={backHref}>Back to files</Link>
          </Button>
        </div>
      );
    }

    return (
      <div className="flex flex-col gap-4">
        <Link
          href={backHref}
          className="text-muted-foreground hover:text-foreground flex w-fit items-center gap-1 text-sm"
        >
          <ArrowLeft className="size-4" />
          Back to files
        </Link>
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>
            {fileQuery.error?.message ?? "Failed to load file."}
          </AlertDescription>
        </Alert>
        <Button
          variant="outline"
          size="sm"
          onClick={() => fileQuery.refetch()}
        >
          <RefreshCw className="size-4" />
          Retry
        </Button>
      </div>
    );
  }

  const file = fileQuery.data!;

  // ── Render ──────────────────────────────────────────────────────────────

  return (
    <TooltipProvider>
      <div className="flex flex-1 flex-col gap-6">
        {/* Back link */}
        <Link
          href={backHref}
          className="text-muted-foreground hover:text-foreground flex w-fit items-center gap-1 text-sm"
        >
          <ArrowLeft className="size-4" />
          Back to files
        </Link>

        <div className="grid flex-1 grid-cols-1 gap-6 lg:grid-cols-[1fr_340px]">
          {/* ── Left Column: Code ─────────────────────────────────────── */}
          <div className="min-w-0 flex flex-col gap-4">
            {/* File path breadcrumb */}
            <FilePathBreadcrumb
              filePath={file.file_path}
              language={file.language}
            />

            {/* Metadata bar */}
            <div className="flex items-center gap-4 text-sm text-muted-foreground">
              <span>{formatBytes(file.size_bytes)}</span>
              <span>{file.line_count.toLocaleString()} lines</span>
              <span className="flex items-center gap-1 font-mono text-xs">
                {file.content_hash.slice(0, 12)}
                <CopyButton text={file.content_hash} label="Content hash" />
              </span>
            </div>

            {/* Syntax-highlighted code */}
            <CodeViewerWithPreview code={file.content} language={file.language} filePath={filePath} />
          </div>

          {/* ── Right Column: Sidebar ─────────────────────────────────── */}
          <div className="flex flex-col gap-4">
            {/* File Info Card */}
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm font-medium">
                  File Info
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="flex flex-col gap-3">
                  <code className="bg-muted rounded px-2 py-1 text-xs break-all">
                    {file.file_path}
                  </code>
                  <div className="flex flex-col gap-2 text-sm">
                    <div className="flex items-center justify-between">
                      <span className="text-muted-foreground">Language</span>
                      <Badge variant="outline" className="text-xs">
                        {file.language || "unknown"}
                      </Badge>
                    </div>
                    <div className="flex items-center justify-between">
                      <span className="text-muted-foreground">Size</span>
                      <span>{formatBytes(file.size_bytes)}</span>
                    </div>
                    <div className="flex items-center justify-between">
                      <span className="text-muted-foreground">Lines</span>
                      <span>{file.line_count.toLocaleString()}</span>
                    </div>
                    <div className="flex items-center justify-between">
                      <span className="text-muted-foreground">
                        Last indexed
                      </span>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span>
                            {formatRelativeTime(file.last_indexed_at)}
                          </span>
                        </TooltipTrigger>
                        <TooltipContent>
                          {new Date(file.last_indexed_at).toLocaleString()}
                        </TooltipContent>
                      </Tooltip>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>

            {/* AI Description Card */}
            <AiDescriptionCard />

            {/* File Facts Card */}
            <FileFactsCard fileFacts={file.file_facts} />

            {/* Exports Card */}
            <FileExportsCard projectId={projectId} filePath={filePath} />

            {/* Diagnostics Card */}
            <FileDiagnosticsCard issues={file.issues} />

            {/* Editorial History Card */}
            <FileHistoryCard projectId={projectId} filePath={filePath} />

            {/* Dependencies Card */}
            <FileDependenciesCard projectId={projectId} filePath={filePath} />

            {/* References Card */}
            <FileReferencesCard projectId={projectId} filePath={filePath} />

            {/* JSX Usages Card */}
            <FileJsxUsagesCard projectId={projectId} filePath={filePath} language={file.language} />

            {/* Network Calls Card */}
            <FileNetworkCallsCard projectId={projectId} filePath={filePath} />
          </div>
        </div>
      </div>
    </TooltipProvider>
  );
}
