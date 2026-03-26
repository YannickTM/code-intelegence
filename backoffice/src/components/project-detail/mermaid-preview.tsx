"use client";

import { useEffect, useState } from "react";
import { useTheme } from "next-themes";
import { AlertCircle } from "lucide-react";
import mermaid from "mermaid";
import { Alert, AlertDescription } from "~/components/ui/alert";
import { Skeleton } from "~/components/ui/skeleton";
import { cn } from "~/lib/utils";

export function MermaidPreview({
  code,
  className,
}: {
  code: string;
  className?: string;
}) {
  const { resolvedTheme } = useTheme();
  const [state, setState] = useState<
    | { status: "loading" }
    | { status: "success"; svg: string }
    | { status: "error"; message: string }
  >({ status: "loading" });

  const theme = resolvedTheme === "dark" ? "dark" : "default";

  useEffect(() => {
    let cancelled = false;

    async function render() {
      setState({ status: "loading" });

      mermaid.initialize({
        startOnLoad: false,
        theme,
      });

      const id = `mermaid-${crypto.randomUUID()}`;
      try {
        const { svg } = await mermaid.render(id, code);
        if (!cancelled) {
          setState({ status: "success", svg });
        }
      } catch (err) {
        if (!cancelled) {
          setState({
            status: "error",
            message:
              err instanceof Error ? err.message : "Failed to render diagram",
          });
        }
      } finally {
        // mermaid.render() inserts a temporary SVG into the DOM — clean it up
        document.getElementById(id)?.remove();
      }
    }

    void render();

    return () => {
      cancelled = true;
    };
  }, [code, theme]);

  const containerClasses = cn(
    "min-h-0 flex-1 overflow-y-auto rounded-md border",
    className,
  );

  if (state.status === "loading") {
    return <Skeleton className={cn("h-[400px] w-full rounded-md", className)} />;
  }

  if (state.status === "error") {
    return (
      <div className={cn(containerClasses, "p-4")}>
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>
            <p className="font-medium">Invalid Mermaid syntax</p>
            <pre className="mt-2 whitespace-pre-wrap text-xs">
              {state.message}
            </pre>
          </AlertDescription>
        </Alert>
      </div>
    );
  }

  // Render the SVG inside a sandboxed iframe to prevent XSS if mermaid's
  // built-in DOMPurify is ever bypassed — matching html-preview.tsx approach.
  const srcdoc = `<!DOCTYPE html>
<html><head><style>
  body { margin: 0; display: flex; justify-content: center; align-items: center; min-height: 100vh;
    background: ${resolvedTheme === "dark" ? "#262626" : "#ffffff"}; }
  svg { max-width: 100%; height: auto; }
</style></head><body>${state.svg}</body></html>`;

  return (
    <iframe
      sandbox=""
      srcDoc={srcdoc}
      title="Mermaid Diagram Preview"
      className={cn(containerClasses, "p-0")}
      style={{ width: "100%", minHeight: "400px" }}
    />
  );
}
