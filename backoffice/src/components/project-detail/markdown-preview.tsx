"use client";

import type { Components } from "react-markdown";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeRaw from "rehype-raw";
import { cn } from "~/lib/utils";
import { MermaidPreview } from "./mermaid-preview";

const components: Components = {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  code({ node, className, children, ...props }) {
    const match = /language-(\w+)/.exec(className ?? "");
    const lang = match?.[1];

    // Render mermaid code blocks as diagrams
    if (lang === "mermaid") {
      const code = String(children).replace(/\n$/, "");
      return <MermaidPreview code={code} />;
    }

    // Inline code vs block code
    const isBlock = className != null;
    if (isBlock) {
      return (
        <code className={className} {...props}>
          {children}
        </code>
      );
    }

    return <code {...props}>{children}</code>;
  },
  // Unwrap the <pre> around mermaid blocks so MermaidPreview renders directly
  pre({ children }) {
    // If the only child is a MermaidPreview (via the code override), unwrap the <pre>
    const child = Array.isArray(children) ? children[0] : children;
    if (
      child &&
      typeof child === "object" &&
      "type" in child &&
      child.type === MermaidPreview
    ) {
      return <>{children}</>;
    }
    return <pre>{children}</pre>;
  },
};

export function MarkdownPreview({
  code,
  className,
}: {
  code: string;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "min-h-0 flex-1 overflow-y-auto rounded-md border p-6",
        "prose dark:prose-invert max-w-none",
        className,
      )}
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeRaw]}
        components={components}
      >
        {code}
      </ReactMarkdown>
    </div>
  );
}
