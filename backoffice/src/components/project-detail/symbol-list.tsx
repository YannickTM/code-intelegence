"use client";

import { useState, useCallback } from "react";
import { keepPreviousData } from "@tanstack/react-query";
import Link from "next/link";
import {
  AlertCircle,
  ChevronDown,
  ChevronRight,
  Code,
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "~/components/ui/table";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "~/components/ui/tooltip";
import { Pagination } from "~/components/pagination";
import { SymbolDetailPanel } from "./symbol-detail-panel";

const PAGE_SIZE = 50;

const KIND_OPTIONS = [
  { label: "All Kinds", value: "__all__" },
  { label: "Function", value: "function" },
  { label: "Class", value: "class" },
  { label: "Interface", value: "interface" },
  { label: "Type Alias", value: "type_alias" },
  { label: "Variable", value: "variable" },
  { label: "Enum", value: "enum" },
  { label: "Method", value: "method" },
  { label: "Namespace", value: "namespace" },
] as const;

const COLUMNS: { label: string; className?: string }[] = [
  { label: "Name" },
  { label: "Kind", className: "w-[120px]" },
  { label: "Type", className: "w-[180px]" },
  { label: "File" },
  { label: "Lines", className: "w-[100px] text-right" },
];

// ── Kind badge ──────────────────────────────────────────────────────────────

function kindBadgeClassName(kind: string): string {
  switch (kind.toLowerCase()) {
    case "function":
      return "bg-blue-500/10 text-blue-600 dark:text-blue-400";
    case "class":
      return "bg-purple-500/10 text-purple-600 dark:text-purple-400";
    case "interface":
      return "bg-green-500/10 text-green-600 dark:text-green-400";
    case "type":
    case "type_alias":
      return "bg-orange-500/10 text-orange-600 dark:text-orange-400";
    case "variable":
      return "bg-gray-500/10 text-gray-600 dark:text-gray-400";
    case "enum":
      return "bg-yellow-500/10 text-yellow-600 dark:text-yellow-400";
    case "method":
      return "border-blue-400/40 text-blue-600 dark:text-blue-400";
    case "namespace":
      return "bg-teal-500/10 text-teal-600 dark:text-teal-400";
    default:
      return "";
  }
}

function SymbolKindBadge({ kind }: { kind: string }) {
  const extra = kindBadgeClassName(kind);
  return (
    <Badge
      variant={kind.toLowerCase() === "method" ? "outline" : "secondary"}
      className={extra}
    >
      {kind}
    </Badge>
  );
}

// ── Flag badges ─────────────────────────────────────────────────────────────

const FLAG_BADGE_CONFIG: {
  key: keyof SymbolFlags;
  label: string;
  className: string;
  variant: "outline" | "secondary";
}[] = [
  {
    key: "is_default_export",
    label: "Default",
    className: "border-green-500/40 text-green-600 dark:text-green-400",
    variant: "outline",
  },
  {
    key: "is_exported",
    label: "Export",
    className: "border-green-500/40 text-green-600 dark:text-green-400",
    variant: "outline",
  },
  {
    key: "is_async",
    label: "async",
    className: "bg-blue-500/10 text-blue-600 dark:text-blue-400",
    variant: "secondary",
  },
  {
    key: "is_static",
    label: "static",
    className: "bg-gray-500/10 text-gray-600 dark:text-gray-400",
    variant: "secondary",
  },
  {
    key: "is_abstract",
    label: "abstract",
    className: "bg-purple-500/10 text-purple-600 dark:text-purple-400",
    variant: "secondary",
  },
  {
    key: "is_react_component_like",
    label: "Component",
    className: "bg-cyan-500/10 text-cyan-600 dark:text-cyan-400",
    variant: "secondary",
  },
  {
    key: "is_hook_like",
    label: "Hook",
    className: "border-cyan-500/40 text-cyan-600 dark:text-cyan-400",
    variant: "outline",
  },
];

const MAX_VISIBLE_BADGES = 3;

function SymbolFlagBadges({ flags }: { flags?: SymbolFlags }) {
  if (!flags) return null;

  const applicable = FLAG_BADGE_CONFIG.filter((cfg) => {
    if (cfg.key === "is_exported" && flags.is_default_export) return false;
    return flags[cfg.key] === true;
  });

  if (applicable.length === 0) return null;

  const visible = applicable.slice(0, MAX_VISIBLE_BADGES);
  const overflow = applicable.length - MAX_VISIBLE_BADGES;

  return (
    <span className="inline-flex items-center gap-1">
      {visible.map((cfg) => (
        <Badge
          key={cfg.key}
          variant={cfg.variant}
          className={`px-1.5 py-0 text-[10px] leading-4 ${cfg.className}`}
        >
          {cfg.label}
        </Badge>
      ))}
      {overflow > 0 && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Badge
              variant="secondary"
              className="cursor-default px-1.5 py-0 text-[10px] leading-4"
            >
              +{overflow}
            </Badge>
          </TooltipTrigger>
          <TooltipContent>
            {applicable
              .slice(MAX_VISIBLE_BADGES)
              .map((cfg) => cfg.label)
              .join(", ")}
          </TooltipContent>
        </Tooltip>
      )}
    </span>
  );
}

// ── Table header ────────────────────────────────────────────────────────────

function SymbolsTableHeader() {
  return (
    <TableHeader>
      <TableRow>
        {COLUMNS.map((col, i) => (
          <TableHead key={i} className={col.className}>
            {col.label}
          </TableHead>
        ))}
      </TableRow>
    </TableHeader>
  );
}

// ── Skeleton rows ───────────────────────────────────────────────────────────

function SkeletonRow() {
  return (
    <TableRow>
      <TableCell>
        <Skeleton className="h-4 w-40" />
      </TableCell>
      <TableCell>
        <Skeleton className="h-5 w-16 rounded-full" />
      </TableCell>
      <TableCell>
        <Skeleton className="h-4 w-24" />
      </TableCell>
      <TableCell>
        <Skeleton className="h-4 w-48" />
      </TableCell>
      <TableCell className="text-right">
        <Skeleton className="ml-auto h-4 w-12" />
      </TableCell>
    </TableRow>
  );
}

// ── Symbol row ──────────────────────────────────────────────────────────────

type SymbolFlags = {
  is_exported?: boolean;
  is_default_export?: boolean;
  is_async?: boolean;
  is_generator?: boolean;
  is_static?: boolean;
  is_abstract?: boolean;
  is_readonly?: boolean;
  is_optional?: boolean;
  is_arrow_function?: boolean;
  is_react_component_like?: boolean;
  is_hook_like?: boolean;
};

type SymbolItem = {
  id: string;
  name: string;
  qualified_name?: string;
  kind: string;
  signature?: string;
  start_line?: number;
  end_line?: number;
  doc_text?: string;
  file_path: string;
  language?: string;
  flags?: SymbolFlags;
  modifiers?: string[];
  return_type?: string;
  parameter_types?: string[];
};

function SymbolRow({
  symbol,
  projectId,
  isExpanded,
  onToggle,
}: {
  symbol: SymbolItem;
  projectId: string;
  isExpanded: boolean;
  onToggle: () => void;
}) {
  const showQualifiedName =
    symbol.qualified_name && symbol.qualified_name !== symbol.name;

  const fileHref = `/project/${projectId}/file?path=${encodeURIComponent(symbol.file_path)}`;

  const lineRange =
    symbol.start_line != null && symbol.end_line != null
      ? `${symbol.start_line}–${symbol.end_line}`
      : symbol.start_line != null
        ? String(symbol.start_line)
        : "";

  return (
    <>
      <TableRow
        className="cursor-pointer"
        onClick={onToggle}
        data-state={isExpanded ? "selected" : undefined}
      >
        <TableCell>
          <div className="flex flex-col">
            <div className="flex items-center gap-1.5">
              <code className="text-sm font-mono">{symbol.name}</code>
              <SymbolFlagBadges flags={symbol.flags} />
            </div>
            {showQualifiedName && (
              <span className="text-muted-foreground truncate text-xs">
                {symbol.qualified_name}
              </span>
            )}
          </div>
        </TableCell>
        <TableCell>
          <SymbolKindBadge kind={symbol.kind} />
        </TableCell>
        <TableCell>
          {symbol.return_type ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <code className="text-muted-foreground block truncate text-xs font-mono">
                  → {symbol.return_type}
                </code>
              </TooltipTrigger>
              <TooltipContent>
                <code className="text-xs font-mono">→ {symbol.return_type}</code>
              </TooltipContent>
            </Tooltip>
          ) : null}
        </TableCell>
        <TableCell>
          <Link
            href={fileHref}
            className="text-muted-foreground hover:text-foreground truncate text-sm hover:underline"
            onClick={(e) => e.stopPropagation()}
            title={symbol.file_path}
          >
            {symbol.file_path}
          </Link>
        </TableCell>
        <TableCell className="text-muted-foreground text-right text-sm tabular-nums">
          {lineRange}
        </TableCell>
      </TableRow>

      {isExpanded && (
        <SymbolDetailPanel
          symbol={symbol}
          projectId={projectId}
          colSpan={COLUMNS.length}
        />
      )}
    </>
  );
}

// ── Main component ──────────────────────────────────────────────────────────

export function SymbolList({ projectId }: { projectId: string }) {
  const [nameFilter, setNameFilter] = useState("");
  const [kindFilter, setKindFilter] = useState<string | undefined>();
  const [page, setPage] = useState(0);
  const [expandedSymbolId, setExpandedSymbolId] = useState<string | null>(null);
  const [caseSensitive, setCaseSensitive] = useState(false);
  const [useRegex, setUseRegex] = useState(false);
  const [includeDir, setIncludeDir] = useState("");
  const [excludeDir, setExcludeDir] = useState("");
  const [filtersOpen, setFiltersOpen] = useState(false);

  const searchMode = useRegex
    ? ("regex" as const)
    : caseSensitive
      ? ("sensitive" as const)
      : undefined;

  const debouncedName = useDebounce(nameFilter, 300);
  const debouncedIncludeDir = useDebounce(includeDir, 300);
  const debouncedExcludeDir = useDebounce(excludeDir, 300);

  const { data, isLoading, isError, error, refetch } =
    api.projectSearch.listSymbols.useQuery(
      {
        projectId,
        name: debouncedName || undefined,
        kind: kindFilter,
        searchMode,
        includeDir: debouncedIncludeDir || undefined,
        excludeDir: debouncedExcludeDir || undefined,
        limit: PAGE_SIZE,
        offset: page * PAGE_SIZE,
      },
      { retry: false, placeholderData: keepPreviousData },
    );

  // Detect 422 regex validation errors — show inline, not full error state
  const isRegexError = isError && error?.data?.httpStatus === 422;
  const regexErrorMessage = isRegexError ? error.message : null;

  const total = data?.total ?? 0;

  const resetPagination = useCallback(() => {
    setPage(0);
    setExpandedSymbolId(null);
  }, []);

  const handleNameChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setNameFilter(e.target.value);
      resetPagination();
    },
    [resetPagination],
  );

  const handleKindChange = useCallback(
    (value: string) => {
      setKindFilter(value === "__all__" ? undefined : value);
      resetPagination();
    },
    [resetPagination],
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

  const toggleExpanded = useCallback((id: string) => {
    setExpandedSymbolId((prev) => (prev === id ? null : id));
  }, []);

  const hasActiveFilters =
    !!debouncedName ||
    !!kindFilter ||
    !!debouncedIncludeDir ||
    !!debouncedExcludeDir;

  const filterBarProps = {
    nameFilter,
    kindFilter,
    onNameChange: handleNameChange,
    onKindChange: handleKindChange,
    caseSensitive,
    useRegex,
    onToggleCaseSensitive: handleToggleCaseSensitive,
    onToggleRegex: handleToggleRegex,
    includeDir,
    excludeDir,
    onIncludeDirChange: handleIncludeDirChange,
    onExcludeDirChange: handleExcludeDirChange,
    filtersOpen,
    onFiltersOpenChange: setFiltersOpen,
    regexErrorMessage,
  } as const;

  // ── Loading ─────────────────────────────────────────────────────────────

  if (isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <FilterBar {...filterBarProps} total={total} isLoading />
        <div className="rounded-md border">
          <Table>
            <SymbolsTableHeader />
            <TableBody>
              {Array.from({ length: 8 }, (_, i) => (
                <SkeletonRow key={i} />
              ))}
            </TableBody>
          </Table>
        </div>
      </div>
    );
  }

  // ── Error (non-regex) ──────────────────────────────────────────────────

  if (isError && !isRegexError) {
    return (
      <div className="flex flex-col gap-3">
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertDescription>
            {error?.message ?? "Failed to load symbols."}
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

  // ── Empty: no snapshot ─────────────────────────────────────────────────
  // Skip when regex error — fall through to table view so the filter bar
  // renders the inline error message instead of hiding it.

  if (!isRegexError && !data?.snapshot_id) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center rounded-xl border border-dashed p-12">
        <Code className="text-muted-foreground mb-4 size-10" />
        <h3 className="text-lg font-semibold">No index available</h3>
        <p className="text-muted-foreground text-sm">
          Trigger an indexing job to extract symbols.
        </p>
      </div>
    );
  }

  // ── Empty: no symbols after filters ────────────────────────────────────

  if (!isRegexError && total === 0 && hasActiveFilters) {
    return (
      <div className="flex flex-col gap-4">
        <FilterBar {...filterBarProps} total={total} />
        <div className="flex flex-1 flex-col items-center justify-center rounded-xl border border-dashed p-12">
          <Search className="text-muted-foreground mb-4 size-10" />
          <h3 className="text-lg font-semibold">No symbols found</h3>
          <p className="text-muted-foreground text-sm">
            No symbols found matching your search.
          </p>
        </div>
      </div>
    );
  }

  // ── Empty: indexed but no symbols ──────────────────────────────────────

  if (!isRegexError && total === 0) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center rounded-xl border border-dashed p-12">
        <Code className="text-muted-foreground mb-4 size-10" />
        <h3 className="text-lg font-semibold">No symbols extracted</h3>
        <p className="text-muted-foreground text-sm">
          No symbols were extracted from the indexed files.
        </p>
      </div>
    );
  }

  // ── Table ──────────────────────────────────────────────────────────────

  return (
    <div className="flex flex-col gap-4">
      <FilterBar {...filterBarProps} total={total} />

      <TooltipProvider>
        <div className="rounded-md border">
          <Table>
            <SymbolsTableHeader />
            <TableBody>
              {(data?.items ?? []).map((symbol) => (
                <SymbolRow
                  key={symbol.id}
                  symbol={symbol}
                  projectId={projectId}
                  isExpanded={expandedSymbolId === symbol.id}
                  onToggle={() => toggleExpanded(symbol.id)}
                />
              ))}
            </TableBody>
          </Table>
        </div>
      </TooltipProvider>

      <Pagination
        page={page}
        pageSize={PAGE_SIZE}
        total={total}
        noun="symbols"
        onPageChange={(p) => {
          setPage(p);
          setExpandedSymbolId(null);
        }}
      />
    </div>
  );
}

// ── Filter bar ──────────────────────────────────────────────────────────────

function FilterBar({
  nameFilter,
  kindFilter,
  onNameChange,
  onKindChange,
  caseSensitive,
  useRegex,
  onToggleCaseSensitive,
  onToggleRegex,
  includeDir,
  excludeDir,
  onIncludeDirChange,
  onExcludeDirChange,
  filtersOpen,
  onFiltersOpenChange,
  regexErrorMessage,
  total,
  isLoading,
}: {
  nameFilter: string;
  kindFilter: string | undefined;
  onNameChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onKindChange: (value: string) => void;
  caseSensitive: boolean;
  useRegex: boolean;
  onToggleCaseSensitive: () => void;
  onToggleRegex: () => void;
  includeDir: string;
  excludeDir: string;
  onIncludeDirChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onExcludeDirChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  filtersOpen: boolean;
  onFiltersOpenChange: (open: boolean) => void;
  regexErrorMessage: string | null;
  total?: number;
  isLoading?: boolean;
}) {
  const hasDirFilters = !!includeDir || !!excludeDir;

  return (
    <TooltipProvider>
      <div className="flex flex-col gap-2">
        {/* Row 1: Search input + toggles + kind + total */}
        <div className="flex items-center gap-3">
          <div className="flex flex-1 flex-col">
            <div className="flex items-center gap-1">
              <div className="relative flex-1">
                <Search className="text-muted-foreground pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2" />
                <Input
                  placeholder="Search symbols..."
                  value={nameFilter}
                  onChange={onNameChange}
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
            value={kindFilter ?? "__all__"}
            onValueChange={onKindChange}
          >
            <SelectTrigger className="w-[160px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {KIND_OPTIONS.map((opt) => (
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
              {total.toLocaleString()} symbols
            </Badge>
          ) : null}
        </div>

        {/* Row 2: Collapsible directory filters */}
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
              {hasDirFilters && (
                <span className="text-primary ml-0.5 text-xs">●</span>
              )}
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent className="flex flex-col gap-2 pt-1">
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
