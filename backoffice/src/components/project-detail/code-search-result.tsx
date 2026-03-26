"use client";

import { useState, useEffect, useMemo } from "react";
import { useTheme } from "next-themes";
import { codeToHtml, type BundledLanguage, type BundledTheme } from "shiki";
import Link from "next/link";
import { ChevronDown, ChevronRight, ExternalLink, FileCode } from "lucide-react";
import { Badge } from "~/components/ui/badge";
import { Skeleton } from "~/components/ui/skeleton";

// ── Types ────────────────────────────────────────────────────────────────────

type CodeSearchMatch = {
  chunk_id: string;
  file_path: string;
  language: string | null;
  start_line: number;
  end_line: number;
  content: string;
  match_count: number;
};

type SearchMode = "insensitive" | "sensitive" | "regex" | undefined;

// ── Shiki match decorations ──────────────────────────────────────────────────

type ShikiDecoration = {
  start: { line: number; character: number };
  end: { line: number; character: number };
  properties: { class: string };
};

function computeMatchDecorations(
  content: string,
  query: string,
  mode: SearchMode,
): ShikiDecoration[] {
  if (!query) return [];

  const lines = content.split("\n");
  const decorations: ShikiDecoration[] = [];

  for (let lineIdx = 0; lineIdx < lines.length; lineIdx++) {
    const line = lines[lineIdx]!;

    if (mode === "regex") {
      try {
        const re = new RegExp(query, "g");
        let match: RegExpExecArray | null;
        while ((match = re.exec(line)) !== null) {
          if (match[0].length === 0) {
            re.lastIndex++;
            continue;
          }
          decorations.push({
            start: { line: lineIdx, character: match.index },
            end: { line: lineIdx, character: match.index + match[0].length },
            properties: { class: "search-match" },
          });
        }
      } catch {
        /* invalid regex — skip highlighting */
      }
    } else {
      const caseSensitive = mode === "sensitive";
      const haystack = caseSensitive ? line : line.toLowerCase();
      const needle = caseSensitive ? query : query.toLowerCase();
      if (!needle) continue;

      let pos = haystack.indexOf(needle);
      while (pos !== -1) {
        decorations.push({
          start: { line: lineIdx, character: pos },
          end: { line: lineIdx, character: pos + needle.length },
          properties: { class: "search-match" },
        });
        pos = haystack.indexOf(needle, pos + needle.length);
      }
    }
  }

  return decorations;
}

// ── Language mapping for Shiki ───────────────────────────────────────────────

const SHIKI_LANG_MAP: Record<string, string> = {
  shell: "bash",
};

function mapLanguage(lang: string | null): string {
  if (!lang) return "text";
  return SHIKI_LANG_MAP[lang] ?? lang;
}

// ── Global styles (injected once) ────────────────────────────────────────────

export const SEARCH_CODE_STYLES = `
.search-result-code code { counter-reset: line; }
.search-result-code code .line {
  display: flex;
  white-space: pre-wrap;
  word-break: break-all;
}
.search-result-code code .line::before {
  counter-increment: line;
  content: counter(line);
  flex-shrink: 0;
  width: 3rem;
  margin-right: 1rem;
  text-align: right;
  color: var(--color-muted-foreground);
  opacity: 0.5;
  user-select: none;
}
.search-match {
  background-color: rgba(234, 179, 8, 0.35);
  border-radius: 2px;
}
.dark .search-match {
  background-color: rgba(234, 179, 8, 0.25);
}
`;

// ── Fallback plain-text rendering ────────────────────────────────────────────

type Segment = { text: string; isMatch: boolean };

function highlightMatches(
  text: string,
  query: string,
  mode: SearchMode,
): Segment[] {
  if (!query) return [{ text, isMatch: false }];

  const segments: Segment[] = [];

  if (mode === "regex") {
    try {
      const re = new RegExp(query, "g");
      let lastIndex = 0;
      let match: RegExpExecArray | null;
      while ((match = re.exec(text)) !== null) {
        if (match[0].length === 0) {
          re.lastIndex++;
          continue;
        }
        if (match.index > lastIndex) {
          segments.push({
            text: text.slice(lastIndex, match.index),
            isMatch: false,
          });
        }
        segments.push({ text: match[0], isMatch: true });
        lastIndex = re.lastIndex;
      }
      if (lastIndex < text.length) {
        segments.push({ text: text.slice(lastIndex), isMatch: false });
      }
      return segments.length > 0 ? segments : [{ text, isMatch: false }];
    } catch {
      return [{ text, isMatch: false }];
    }
  }

  const caseSensitive = mode === "sensitive";
  const haystack = caseSensitive ? text : text.toLowerCase();
  const needle = caseSensitive ? query : query.toLowerCase();
  if (!needle) return [{ text, isMatch: false }];

  let lastIndex = 0;
  let pos = haystack.indexOf(needle, lastIndex);
  while (pos !== -1) {
    if (pos > lastIndex) {
      segments.push({ text: text.slice(lastIndex, pos), isMatch: false });
    }
    segments.push({
      text: text.slice(pos, pos + needle.length),
      isMatch: true,
    });
    lastIndex = pos + needle.length;
    pos = haystack.indexOf(needle, lastIndex);
  }
  if (lastIndex < text.length) {
    segments.push({ text: text.slice(lastIndex), isMatch: false });
  }
  return segments.length > 0 ? segments : [{ text, isMatch: false }];
}

function FallbackCodeBlock({
  content,
  startLine,
  query,
  searchMode,
}: {
  content: string;
  startLine: number;
  query: string;
  searchMode: SearchMode;
}) {
  const lines = content.split("\n");
  const gutterWidth = String(startLine + lines.length).length;

  return (
    <pre className="text-sm">
      <code>
        {lines.map((line, i) => {
          const lineNum = startLine + i;
          const segments = highlightMatches(line, query, searchMode);
          return (
            <div key={i} className="flex">
              <span
                className="text-muted-foreground/60 select-none border-r px-3 py-0 text-right"
                style={{ minWidth: `${gutterWidth + 2}ch` }}
              >
                {lineNum}
              </span>
              <span className="flex-1 whitespace-pre px-4">
                {segments.map((seg, j) =>
                  seg.isMatch ? (
                    <mark
                      key={j}
                      className="rounded-sm bg-yellow-200/50 dark:bg-yellow-700/30"
                    >
                      {seg.text}
                    </mark>
                  ) : (
                    <span key={j}>{seg.text}</span>
                  ),
                )}
              </span>
            </div>
          );
        })}
      </code>
    </pre>
  );
}

// ── Result card ──────────────────────────────────────────────────────────────

export function CodeSearchResult({
  result,
  projectId,
  query,
  searchMode,
  defaultOpen = false,
}: {
  result: CodeSearchMatch;
  projectId: string;
  query: string;
  searchMode: SearchMode;
  defaultOpen?: boolean;
}) {
  const [isOpen, setIsOpen] = useState(defaultOpen);
  const { resolvedTheme } = useTheme();
  const [highlightedHtml, setHighlightedHtml] = useState<string | null>(null);
  const [highlightError, setHighlightError] = useState(false);

  const fileHref = `/project/${projectId}/file?path=${encodeURIComponent(result.file_path)}${result.start_line != null ? `&line=${result.start_line}` : ""}`;
  const shikiLang = mapLanguage(result.language);
  const theme: BundledTheme =
    resolvedTheme === "dark" ? "github-dark" : "github-light";

  const decorations = useMemo(
    () => computeMatchDecorations(result.content, query, searchMode),
    [result.content, query, searchMode],
  );

  // Run Shiki highlighting when the card is expanded
  useEffect(() => {
    if (!isOpen) return;

    let cancelled = false;
    setHighlightedHtml(null);
    setHighlightError(false);

    codeToHtml(result.content, {
      lang: shikiLang as BundledLanguage,
      theme,
      decorations,
      transformers: [
        {
          pre(node) {
            node.properties.style =
              "margin:0;padding:0.5rem 0;font-size:0.875rem;line-height:1.5;tab-size:2;white-space:normal;background:transparent;";
          },
          code(node) {
            // Offset CSS counter to match the actual file line numbers
            node.properties.style = `counter-reset: line ${result.start_line - 1};`;
          },
        },
      ],
    })
      .then((html) => {
        if (!cancelled) setHighlightedHtml(html);
      })
      .catch(() => {
        if (!cancelled) setHighlightError(true);
      });

    return () => {
      cancelled = true;
    };
  }, [isOpen, result.content, result.start_line, shikiLang, theme, decorations]);

  return (
    <div className="rounded-md border">
      {/* Clickable header */}
      <div
        className="bg-muted/50 hover:bg-muted/80 flex cursor-pointer items-center gap-2 px-4 py-2 transition-colors"
        onClick={() => setIsOpen((prev) => !prev)}
      >
        {isOpen ? (
          <ChevronDown className="text-muted-foreground size-4 shrink-0" />
        ) : (
          <ChevronRight className="text-muted-foreground size-4 shrink-0" />
        )}
        <FileCode className="text-muted-foreground size-4 shrink-0" />
        <span
          className="truncate text-sm font-medium"
          title={result.file_path}
        >
          {result.file_path}
        </span>
        {result.language && (
          <Badge variant="secondary" className="shrink-0 text-xs">
            {result.language}
          </Badge>
        )}
        <span className="text-muted-foreground shrink-0 text-xs tabular-nums">
          lines {result.start_line}–{result.end_line}
        </span>
        {result.match_count > 0 && (
          <Badge variant="outline" className="text-muted-foreground shrink-0 text-xs font-normal">
            {result.match_count} {result.match_count === 1 ? "match" : "matches"}
          </Badge>
        )}
        <div className="flex-1" />
        <Link
          href={fileHref}
          className="text-muted-foreground hover:text-foreground flex shrink-0 items-center gap-1 text-xs"
          onClick={(e) => e.stopPropagation()}
        >
          View file
          <ExternalLink className="size-3" />
        </Link>
      </div>

      {/* Collapsible code block */}
      {isOpen && (
        <div className="search-result-code overflow-x-auto border-t">
          {highlightedHtml ? (
            <div dangerouslySetInnerHTML={{ __html: highlightedHtml }} />
          ) : highlightError ? (
            <FallbackCodeBlock
              content={result.content}
              startLine={result.start_line}
              query={query}
              searchMode={searchMode}
            />
          ) : (
            <div className="p-4">
              <Skeleton className="h-24 w-full" />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
