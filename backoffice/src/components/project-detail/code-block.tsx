"use client";

import { useEffect, useState } from "react";
import { useTheme } from "next-themes";
import { type BundledLanguage, type BundledTheme, codeToHtml } from "shiki";
import { Skeleton } from "~/components/ui/skeleton";
import { cn } from "~/lib/utils";

/** CSS injected once for line-number gutter via CSS counters. */
const LINE_NUMBER_STYLES = `
.shiki-code-block code { counter-reset: line; }
.shiki-code-block code .line {
  display: flex;
  white-space: pre-wrap;
  word-break: break-all;
}
.shiki-code-block code .line::before {
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
`;

export function CodeBlock({
  code,
  language,
  className,
}: {
  code: string;
  language: string;
  className?: string;
}) {
  const { resolvedTheme } = useTheme();
  const [state, setState] = useState<
    { status: "loading" } | { status: "success"; html: string } | { status: "error" }
  >({ status: "loading" });

  const theme: BundledTheme =
    resolvedTheme === "dark" ? "github-dark" : "github-light";

  useEffect(() => {
    let cancelled = false;
    setState({ status: "loading" });

    codeToHtml(code, {
      lang: language as BundledLanguage,
      theme,
      transformers: [
        {
          pre(node) {
            node.properties.style =
              "margin:0;padding:1rem;font-size:0.875rem;line-height:1.5;tab-size:2;white-space:normal;";
          },
          line(node, line) {
            node.properties.id = `L${line}`;
          },
        },
      ],
    })
      .then((result) => {
        if (!cancelled) setState({ status: "success", html: result });
      })
      .catch(() => {
        if (!cancelled) setState({ status: "error" });
      });

    return () => {
      cancelled = true;
    };
  }, [code, language, theme]);

  // Loading
  if (state.status === "loading") {
    return <Skeleton className={cn("h-[400px] w-full rounded-md", className)} />;
  }

  // Fallback for unsupported languages
  if (state.status === "error") {
    return (
      <div
        className={cn(
          "min-h-0 flex-1 overflow-y-auto rounded-md border",
          className,
        )}
      >
        <pre className="p-4 font-mono text-sm leading-relaxed">
          {code.split("\n").map((line, i) => (
            <div key={i} id={`L${i + 1}`} className="flex">
              <span className="mr-4 inline-block w-12 shrink-0 text-right text-muted-foreground opacity-50 select-none">
                {i + 1}
              </span>
              <span className="whitespace-pre-wrap break-all">{line}</span>
            </div>
          ))}
        </pre>
      </div>
    );
  }

  return (
    <>
      <style dangerouslySetInnerHTML={{ __html: LINE_NUMBER_STYLES }} />
      <div
        className={cn(
          "shiki-code-block min-h-0 flex-1 overflow-y-auto rounded-md border",
          className,
        )}
        dangerouslySetInnerHTML={{ __html: state.html }}
      />
    </>
  );
}
