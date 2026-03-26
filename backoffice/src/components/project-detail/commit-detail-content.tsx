"use client";

import { useState, useCallback } from "react";
import Link from "next/link";
import { AlertCircle, ArrowLeft, Check, ChevronRight, Copy } from "lucide-react";
import { toast } from "sonner";
import { api } from "~/trpc/react";
import { Alert, AlertDescription } from "~/components/ui/alert";
import { Skeleton } from "~/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "~/components/ui/tooltip";
import { formatRelativeTime } from "~/lib/format";
import { cn } from "~/lib/utils";
import type { FileDiff } from "~/server/api/routers/project-commits";
import { ChangeTypeBadge } from "./change-type-badge";

// ── Copy hash button ────────────────────────────────────────────────────────

function CopyHashButton({ hash }: { hash: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(hash).then(
      () => {
        toast.success("Commit hash copied");
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      },
      (err: unknown) => {
        toast.error(err instanceof Error ? err.message : "Failed to copy");
      },
    );
  }, [hash]);

  return (
    <button
      type="button"
      onClick={handleCopy}
      className="text-muted-foreground hover:text-foreground rounded p-1 transition-colors"
      aria-label="Copy commit hash"
    >
      {copied ? (
        <Check className="size-3.5" />
      ) : (
        <Copy className="size-3.5" />
      )}
    </button>
  );
}

// ── File path display ───────────────────────────────────────────────────────

function getDisplayPath(diff: FileDiff): string {
  switch (diff.change_type) {
    case "added":
      return diff.new_file_path ?? "(unknown)";
    case "deleted":
      return diff.old_file_path ?? "(unknown)";
    case "renamed":
      return `${diff.old_file_path ?? "?"} \u2192 ${diff.new_file_path ?? "?"}`;
    default:
      return diff.new_file_path ?? diff.old_file_path ?? "(unknown)";
  }
}

// ── Diff line rendering ─────────────────────────────────────────────────────

function DiffLine({ line }: { line: string }) {
  let className = "";
  if (line.startsWith("@@")) {
    className =
      "bg-blue-50 text-blue-600 dark:bg-blue-950/30 dark:text-blue-400";
  } else if (line.startsWith("+")) {
    className =
      "bg-green-50 text-green-800 dark:bg-green-950/30 dark:text-green-300";
  } else if (line.startsWith("-")) {
    className =
      "bg-red-50 text-red-800 dark:bg-red-950/30 dark:text-red-300";
  }

  return (
    <div className={cn("min-h-[1.25rem] px-2", className)}>
      {line || "\u00A0"}
    </div>
  );
}

// ── Diff patch view ─────────────────────────────────────────────────────────

function DiffPatchView({
  patch,
  isLoading,
}: {
  patch: string | null | undefined;
  isLoading: boolean;
}) {
  if (isLoading) {
    return (
      <div className="border-t bg-muted/30 px-4 py-6">
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  if (!patch) {
    return (
      <div className="border-t bg-muted/30 px-4 py-4">
        <p className="text-muted-foreground text-sm italic">
          No diff available &mdash; binary or omitted content.
        </p>
      </div>
    );
  }

  return (
    <div className="border-t bg-muted/30">
      <pre className="max-h-[600px] overflow-auto p-4 text-xs font-mono leading-relaxed">
        {patch.split("\n").map((line, i) => (
          <DiffLine key={i} line={line} />
        ))}
      </pre>
    </div>
  );
}

// ── File diff row ───────────────────────────────────────────────────────────

function FileDiffRow({
  diff,
  isExpanded,
  onToggle,
  projectId,
  commitHash,
}: {
  diff: FileDiff;
  isExpanded: boolean;
  onToggle: (id: string) => void;
  projectId: string;
  commitHash: string;
}) {
  const displayPath = getDisplayPath(diff);

  // Per-file patch fetch — only fires when this row is expanded
  const { data: patchData, isLoading: patchLoading } =
    api.projectCommits.getCommitDiffs.useQuery(
      { projectId, commitHash, diffId: diff.id, includePatch: true },
      { enabled: isExpanded },
    );

  const patch = patchData?.diffs[0]?.patch;

  return (
    <div>
      <button
        type="button"
        onClick={() => onToggle(diff.id)}
        aria-expanded={isExpanded}
        aria-label={`${isExpanded ? "Collapse" : "Expand"} diff for ${displayPath}`}
        className="flex w-full items-center gap-3 px-4 py-3 text-left hover:bg-muted/50 transition-colors"
      >
        <ChevronRight
          className={cn(
            "size-4 shrink-0 text-muted-foreground transition-transform",
            isExpanded && "rotate-90",
          )}
        />
        <span
          className="flex-1 truncate font-mono text-sm"
          title={displayPath}
        >
          {displayPath}
        </span>
        <ChangeTypeBadge type={diff.change_type} />
        <span className="text-green-600 dark:text-green-400 text-xs tabular-nums">
          +{diff.additions}
        </span>
        <span className="text-red-600 dark:text-red-400 text-xs tabular-nums">
          -{diff.deletions}
        </span>
      </button>

      {isExpanded && (
        <DiffPatchView patch={patch} isLoading={patchLoading} />
      )}
    </div>
  );
}

// ── Main component ──────────────────────────────────────────────────────────

export function CommitDetailContent({
  projectId,
  commitHash,
}: {
  projectId: string;
  commitHash: string;
}) {
  const [expandedFileIds, setExpandedFileIds] = useState<Set<string>>(
    new Set(),
  );

  const {
    data: commit,
    isLoading,
    isError,
    error,
  } = api.projectCommits.getCommit.useQuery({ projectId, commitHash });

  // File list (metadata only, no patches)
  const { data: diffsData, isLoading: diffsLoading } =
    api.projectCommits.getCommitDiffs.useQuery(
      { projectId, commitHash, includePatch: false, limit: 200 },
      { enabled: !!commit },
    );

  const toggleFile = useCallback((fileId: string) => {
    setExpandedFileIds((prev) => {
      const next = new Set(prev);
      if (next.has(fileId)) {
        next.delete(fileId);
      } else {
        next.add(fileId);
      }
      return next;
    });
  }, []);

  const backHref = `/project/${projectId}/commits`;

  // ── Loading ─────────────────────────────────────────────────────────────

  if (isLoading) {
    return (
      <div className="flex flex-col gap-6">
        <Skeleton className="h-4 w-32" />
        <Skeleton className="h-6 w-96" />
        <Skeleton className="h-4 w-64" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  // ── Error ───────────────────────────────────────────────────────────────

  if (isError || !commit) {
    return (
      <div className="flex flex-col gap-4">
        <Link
          href={backHref}
          className="text-muted-foreground hover:text-foreground flex w-fit items-center gap-1 text-sm"
        >
          <ArrowLeft className="size-4" />
          Back to commits
        </Link>
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>
            {error?.message ?? "Commit not found."}
          </AlertDescription>
        </Alert>
      </div>
    );
  }

  // ── Commit body (lines after subject) ─────────────────────────────────

  const messageBody = commit.message !== commit.message_subject
    ? commit.message.slice(commit.message_subject.length).trim()
    : null;

  // ── Render ──────────────────────────────────────────────────────────────

  return (
    <TooltipProvider>
      <div className="flex flex-col gap-6">
        {/* Back link */}
        <Link
          href={backHref}
          className="text-muted-foreground hover:text-foreground flex w-fit items-center gap-1 text-sm"
        >
          <ArrowLeft className="size-4" />
          Back to commits
        </Link>

        {/* Subject */}
        <h2 className="text-lg font-bold">{commit.message_subject}</h2>

        {/* Body */}
        {messageBody && (
          <pre className="text-muted-foreground whitespace-pre-wrap text-sm font-sans">
            {messageBody}
          </pre>
        )}

        {/* Metadata */}
        <div className="flex flex-wrap gap-x-8 gap-y-2 text-sm">
          <div>
            <span className="text-muted-foreground">Author: </span>
            <span className="font-medium">
              {commit.author_name}
              {commit.author_email && (
                <span className="text-muted-foreground font-normal">
                  {" "}
                  &lt;{commit.author_email}&gt;
                </span>
              )}
            </span>
          </div>
          <div>
            <span className="text-muted-foreground">Date: </span>
            <Tooltip>
              <TooltipTrigger asChild>
                <span>{formatRelativeTime(commit.committer_date)}</span>
              </TooltipTrigger>
              <TooltipContent>
                {new Date(commit.committer_date).toLocaleString()}
              </TooltipContent>
            </Tooltip>
          </div>
        </div>

        {/* Hash + copy */}
        <div className="flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Commit:</span>
          <code className="font-mono text-xs">{commit.commit_hash}</code>
          <CopyHashButton hash={commit.commit_hash} />
        </div>

        {/* Parents */}
        {commit.parents.length > 0 ? (
          <div className="flex items-center gap-2 text-sm">
            <span className="text-muted-foreground">
              {commit.parents.length === 1 ? "Parent:" : "Parents:"}
            </span>
            {commit.parents.map((p) => (
              <Link
                key={p.parent_commit_id}
                href={`/project/${projectId}/commits/${p.parent_commit_hash}`}
                className="font-mono text-xs text-blue-600 hover:underline dark:text-blue-400"
              >
                {p.parent_short_hash}
              </Link>
            ))}
          </div>
        ) : (
          <p className="text-muted-foreground text-sm italic">
            Initial commit
          </p>
        )}

        {/* Diff stats summary */}
        <div className="flex gap-4 text-sm">
          <span>
            {commit.diff_stats.files_changed} file
            {commit.diff_stats.files_changed !== 1 && "s"} changed
          </span>
          <span className="text-green-600 dark:text-green-400">
            +{commit.diff_stats.total_additions}
          </span>
          <span className="text-red-600 dark:text-red-400">
            -{commit.diff_stats.total_deletions}
          </span>
        </div>

        {/* File diff list */}
        {diffsLoading ? (
          <div className="divide-y rounded-md border">
            {Array.from({ length: 5 }, (_, i) => (
              <div key={i} className="flex items-center gap-3 px-4 py-3">
                <Skeleton className="h-4 w-4" />
                <Skeleton className="h-4 flex-1" />
                <Skeleton className="h-5 w-16" />
              </div>
            ))}
          </div>
        ) : diffsData && diffsData.diffs.length > 0 ? (
          <div className="divide-y rounded-md border">
            {diffsData.diffs.map((diff) => (
              <FileDiffRow
                key={diff.id}
                diff={diff}
                isExpanded={expandedFileIds.has(diff.id)}
                onToggle={toggleFile}
                projectId={projectId}
                commitHash={commitHash}
              />
            ))}
          </div>
        ) : (
          <p className="text-muted-foreground text-sm italic">
            No file changes in this commit.
          </p>
        )}
      </div>
    </TooltipProvider>
  );
}
