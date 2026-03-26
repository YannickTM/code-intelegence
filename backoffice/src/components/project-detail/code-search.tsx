"use client";

import { useState, useCallback } from "react";
import { keepPreviousData } from "@tanstack/react-query";
import {
  AlertCircle,
  ChevronDown,
  ChevronRight,
  Code,
  FileCode,
  FolderMinus,
  FolderOpen,
  RefreshCw,
  Search,
} from "lucide-react";
import { api } from "~/trpc/react";
import { useDebounce } from "~/hooks/use-debounce";
import { Alert, AlertDescription } from "~/components/ui/alert";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "~/components/ui/collapsible";
import { Input } from "~/components/ui/input";
import { Skeleton } from "~/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "~/components/ui/tooltip";
import { Pagination } from "~/components/pagination";
import { CodeSearchResult, SEARCH_CODE_STYLES } from "./code-search-result";

const PAGE_SIZE = 20;

// Values must match what the backend stores: the parser sets "typescript" /
// "javascript" explicitly; every other language is the file extension
// (strings.TrimPrefix(filepath.Ext(f), ".")).
const LANGUAGE_OPTIONS = [
  { label: "All Languages", value: "__all__" },
  { label: "Go", value: "go" },
  { label: "TypeScript", value: "typescript" },
  { label: "JavaScript", value: "javascript" },
  { label: "Python", value: "py" },
  { label: "Rust", value: "rs" },
  { label: "Java", value: "java" },
  { label: "C", value: "c" },
  { label: "C++", value: "cpp" },
  { label: "Ruby", value: "rb" },
  { label: "PHP", value: "php" },
  { label: "CSS", value: "css" },
  { label: "HTML", value: "html" },
  { label: "SQL", value: "sql" },
  { label: "Shell", value: "sh" },
  { label: "YAML", value: "yaml" },
  { label: "JSON", value: "json" },
  { label: "Markdown", value: "md" },
] as const;

// ── Skeleton cards ───────────────────────────────────────────────────────────

function SkeletonCard() {
  return (
    <div className="rounded-md border">
      <div className="bg-muted/50 flex items-center gap-2 px-4 py-2">
        <Skeleton className="h-4 w-4 shrink-0" />
        <Skeleton className="h-4 w-4 shrink-0" />
        <Skeleton className="h-4 w-60" />
        <Skeleton className="h-5 w-16 shrink-0 rounded-full" />
        <Skeleton className="h-3 w-20 shrink-0" />
        <Skeleton className="h-5 w-20 shrink-0 rounded-full" />
      </div>
    </div>
  );
}

// ── Main component ───────────────────────────────────────────────────────────

export function CodeSearch({ projectId }: { projectId: string }) {
  const [query, setQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");
  const [caseSensitive, setCaseSensitive] = useState(false);
  const [useRegex, setUseRegex] = useState(false);
  const [language, setLanguage] = useState<string | undefined>();
  const [filePattern, setFilePattern] = useState("");
  const [includeDir, setIncludeDir] = useState("");
  const [excludeDir, setExcludeDir] = useState("");
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [page, setPage] = useState(0);

  const searchMode = useRegex
    ? ("regex" as const)
    : caseSensitive
      ? ("sensitive" as const)
      : undefined;

  const debouncedFilePattern = useDebounce(filePattern, 300);
  const debouncedIncludeDir = useDebounce(includeDir, 300);
  const debouncedExcludeDir = useDebounce(excludeDir, 300);

  const { data, isLoading, isError, error, refetch } =
    api.projectSearch.search.useQuery(
      {
        projectId,
        query: submittedQuery,
        searchMode,
        language,
        filePattern: debouncedFilePattern || undefined,
        includeDir: debouncedIncludeDir || undefined,
        excludeDir: debouncedExcludeDir || undefined,
        limit: PAGE_SIZE,
        offset: page * PAGE_SIZE,
      },
      {
        enabled: !!submittedQuery,
        retry: false,
        placeholderData: keepPreviousData,
      },
    );

  const isRegexError = isError && error?.data?.httpStatus === 422;
  const regexErrorMessage = isRegexError ? error.message : null;

  const total = data?.total ?? 0;

  const resetPagination = useCallback(() => {
    setPage(0);
  }, []);

  const handleSubmit = useCallback(() => {
    if (query.trim()) {
      setSubmittedQuery(query.trim());
      setPage(0);
    }
  }, [query]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter") handleSubmit();
    },
    [handleSubmit],
  );

  const handleToggleCaseSensitive = useCallback(() => {
    setCaseSensitive((prev) => !prev);
    setUseRegex(false);
    resetPagination();
  }, [resetPagination]);

  const handleToggleRegex = useCallback(() => {
    setUseRegex((prev) => !prev);
    setCaseSensitive(false);
    resetPagination();
  }, [resetPagination]);

  const handleLanguageChange = useCallback(
    (value: string) => {
      setLanguage(value === "__all__" ? undefined : value);
      resetPagination();
    },
    [resetPagination],
  );

  const handleFilePatternChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setFilePattern(e.target.value);
      resetPagination();
    },
    [resetPagination],
  );

  const handleIncludeDirChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setIncludeDir(e.target.value);
      resetPagination();
    },
    [resetPagination],
  );

  const handleExcludeDirChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setExcludeDir(e.target.value);
      resetPagination();
    },
    [resetPagination],
  );

  const hasActiveFilters =
    !!debouncedFilePattern || !!debouncedIncludeDir || !!debouncedExcludeDir;

  const filterBarProps = {
    query,
    onQueryChange: (e: React.ChangeEvent<HTMLInputElement>) =>
      setQuery(e.target.value),
    onKeyDown: handleKeyDown,
    onSubmit: handleSubmit,
    caseSensitive,
    useRegex,
    onToggleCaseSensitive: handleToggleCaseSensitive,
    onToggleRegex: handleToggleRegex,
    language,
    onLanguageChange: handleLanguageChange,
    filePattern,
    includeDir,
    excludeDir,
    onFilePatternChange: handleFilePatternChange,
    onIncludeDirChange: handleIncludeDirChange,
    onExcludeDirChange: handleExcludeDirChange,
    filtersOpen,
    onFiltersOpenChange: setFiltersOpen,
    hasActiveFilters,
    regexErrorMessage,
  } as const;

  // ── Loading (initial) ──────────────────────────────────────────────────

  if (isLoading && !data) {
    return (
      <div className="flex flex-col gap-4">
        <FilterBar {...filterBarProps} total={total} isLoading />
        <div className="flex flex-col gap-3">
          {Array.from({ length: 4 }, (_, i) => (
            <SkeletonCard key={i} />
          ))}
        </div>
      </div>
    );
  }

  // ── Error (non-regex) ──────────────────────────────────────────────────

  if (isError && !isRegexError) {
    return (
      <div className="flex flex-col gap-4">
        <FilterBar {...filterBarProps} total={total} />
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertDescription>
            {error?.message ?? "Failed to search code."}
          </AlertDescription>
        </Alert>
        <div>
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            <RefreshCw className="size-4" />
            Retry
          </Button>
        </div>
      </div>
    );
  }

  // ── No snapshot ────────────────────────────────────────────────────────

  if (!isRegexError && submittedQuery && data && !data.snapshot_id) {
    return (
      <div className="flex flex-col gap-4">
        <FilterBar {...filterBarProps} total={total} />
        <div className="flex flex-1 flex-col items-center justify-center rounded-xl border border-dashed p-12">
          <Code className="text-muted-foreground mb-4 size-10" />
          <h3 className="text-lg font-semibold">No index available</h3>
          <p className="text-muted-foreground text-sm">
            Trigger an indexing job to search code.
          </p>
        </div>
      </div>
    );
  }

  // ── No query (initial state) ───────────────────────────────────────────

  if (!submittedQuery && !isRegexError) {
    return (
      <div className="flex flex-col gap-4">
        <FilterBar {...filterBarProps} total={total} />
        <div className="flex flex-1 flex-col items-center justify-center rounded-xl border border-dashed p-12">
          <Search className="text-muted-foreground mb-4 size-10" />
          <h3 className="text-lg font-semibold">Enter a search query</h3>
          <p className="text-muted-foreground text-sm">
            Enter a search query to find code across the project.
          </p>
        </div>
      </div>
    );
  }

  // ── No results ─────────────────────────────────────────────────────────

  if (!isRegexError && total === 0 && submittedQuery) {
    return (
      <div className="flex flex-col gap-4">
        <FilterBar {...filterBarProps} total={total} />
        <div className="flex flex-1 flex-col items-center justify-center rounded-xl border border-dashed p-12">
          <Search className="text-muted-foreground mb-4 size-10" />
          <h3 className="text-lg font-semibold">No code found</h3>
          <p className="text-muted-foreground text-sm">
            No code found matching your search.
          </p>
        </div>
      </div>
    );
  }

  // ── Results ────────────────────────────────────────────────────────────

  return (
    <div className="flex flex-col gap-4">
      <style dangerouslySetInnerHTML={{ __html: SEARCH_CODE_STYLES }} />
      <FilterBar {...filterBarProps} total={total} />

      <div className="flex flex-col gap-3">
        {(data?.items ?? []).map((item) => (
          <CodeSearchResult
            key={item.chunk_id}
            result={item}
            projectId={projectId}
            query={submittedQuery}
            searchMode={searchMode}
          />
        ))}
      </div>

      <Pagination
        page={page}
        pageSize={PAGE_SIZE}
        total={total}
        noun="results"
        onPageChange={setPage}
      />
    </div>
  );
}

// ── Filter bar ───────────────────────────────────────────────────────────────

function FilterBar({
  query,
  onQueryChange,
  onKeyDown,
  onSubmit,
  caseSensitive,
  useRegex,
  onToggleCaseSensitive,
  onToggleRegex,
  language,
  onLanguageChange,
  filePattern,
  includeDir,
  excludeDir,
  onFilePatternChange,
  onIncludeDirChange,
  onExcludeDirChange,
  filtersOpen,
  onFiltersOpenChange,
  hasActiveFilters,
  regexErrorMessage,
  total,
  isLoading,
}: {
  query: string;
  onQueryChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onKeyDown: (e: React.KeyboardEvent) => void;
  onSubmit: () => void;
  caseSensitive: boolean;
  useRegex: boolean;
  onToggleCaseSensitive: () => void;
  onToggleRegex: () => void;
  language: string | undefined;
  onLanguageChange: (value: string) => void;
  filePattern: string;
  includeDir: string;
  excludeDir: string;
  onFilePatternChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onIncludeDirChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onExcludeDirChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  filtersOpen: boolean;
  onFiltersOpenChange: (open: boolean) => void;
  hasActiveFilters: boolean;
  regexErrorMessage: string | null;
  total?: number;
  isLoading?: boolean;
}) {
  return (
    <TooltipProvider>
      <div className="flex flex-col gap-2">
        {/* Row 1: Search input + toggles + language + total */}
        <div className="flex items-center gap-3">
          <div className="flex flex-1 flex-col">
            <div className="flex items-center gap-1">
              <div className="relative flex-1">
                <Search className="text-muted-foreground pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2" />
                <Input
                  placeholder="Search code..."
                  value={query}
                  onChange={onQueryChange}
                  onKeyDown={onKeyDown}
                  className="pl-9"
                />
              </div>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    className={`h-8 w-8 shrink-0 ${
                      caseSensitive
                        ? "bg-accent text-accent-foreground"
                        : "text-muted-foreground hover:text-foreground"
                    }`}
                    onClick={onToggleCaseSensitive}
                  >
                    <span className="text-sm font-semibold">Aa</span>
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Match Case</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    className={`h-8 w-8 shrink-0 ${
                      useRegex
                        ? "bg-accent text-accent-foreground"
                        : "text-muted-foreground hover:text-foreground"
                    }`}
                    onClick={onToggleRegex}
                  >
                    <span className="text-sm font-semibold">.*</span>
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Use Regular Expression</TooltipContent>
              </Tooltip>
            </div>
            {regexErrorMessage && (
              <p className="text-destructive mt-1 text-xs">
                {regexErrorMessage}
              </p>
            )}
          </div>
          <Select
            value={language ?? "__all__"}
            onValueChange={onLanguageChange}
          >
            <SelectTrigger className="w-[180px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {LANGUAGE_OPTIONS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {isLoading && !total ? (
            <Skeleton className="h-5 w-20 shrink-0 rounded-full" />
          ) : total != null && total > 0 ? (
            <Badge variant="secondary" className="shrink-0">
              {total.toLocaleString()} results
            </Badge>
          ) : null}
        </div>

        {/* Row 2: Collapsible filters */}
        <Collapsible open={filtersOpen} onOpenChange={onFiltersOpenChange}>
          <CollapsibleTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="text-muted-foreground hover:text-foreground -ml-2 gap-1.5"
            >
              {filtersOpen ? (
                <ChevronDown className="size-4" />
              ) : (
                <ChevronRight className="size-4" />
              )}
              Filters
              {hasActiveFilters && (
                <span className="text-primary ml-0.5 text-xs">●</span>
              )}
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent className="flex flex-col gap-2 pt-1">
            <div className="relative">
              <FileCode className="text-muted-foreground pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2" />
              <Input
                placeholder="File pattern (e.g. *.go, **/*.test.ts)"
                value={filePattern}
                onChange={onFilePatternChange}
                className="pl-9"
              />
            </div>
            <div className="relative">
              <FolderOpen className="text-muted-foreground pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2" />
              <Input
                placeholder="Include dirs (e.g. src/api, **/test/**)"
                value={includeDir}
                onChange={onIncludeDirChange}
                className="pl-9"
              />
            </div>
            <div className="relative">
              <FolderMinus className="text-muted-foreground pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2" />
              <Input
                placeholder="Exclude dirs (e.g. vendor, **/*.test.*)"
                value={excludeDir}
                onChange={onExcludeDirChange}
                className="pl-9"
              />
            </div>
          </CollapsibleContent>
        </Collapsible>
      </div>
    </TooltipProvider>
  );
}
