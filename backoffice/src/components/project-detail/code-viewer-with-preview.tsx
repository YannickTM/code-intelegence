"use client";

import { useEffect, useState } from "react";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "~/components/ui/tabs";
import { CodeBlock } from "./code-block";
import { HtmlPreview } from "./html-preview";
import { MarkdownPreview } from "./markdown-preview";
import { MermaidPreview } from "./mermaid-preview";

const PREVIEWABLE_LANGUAGES = new Set(["html", "markdown", "md", "mermaid"]);

export function CodeViewerWithPreview({
  code,
  language,
  filePath,
}: {
  code: string;
  language: string;
  filePath: string;
}) {
  const [activeTab, setActiveTab] = useState<"code" | "preview">("code");

  // Reset to code tab when file changes
  useEffect(() => {
    setActiveTab("code");
  }, [filePath]);

  // Non-previewable files: render CodeBlock directly, no tabs
  if (!PREVIEWABLE_LANGUAGES.has(language)) {
    return <CodeBlock code={code} language={language} />;
  }

  return (
    <Tabs
      value={activeTab}
      onValueChange={(v) => setActiveTab(v as "code" | "preview")}
      className="flex min-h-0 flex-1 flex-col"
    >
      <TabsList variant="line">
        <TabsTrigger value="code">Code</TabsTrigger>
        <TabsTrigger value="preview">Preview</TabsTrigger>
      </TabsList>

      <TabsContent value="code" className="min-h-0 flex-1">
        <CodeBlock code={code} language={language} />
      </TabsContent>

      <TabsContent value="preview" className="min-h-0 flex-1">
        <PreviewRenderer code={code} language={language} />
      </TabsContent>
    </Tabs>
  );
}

function PreviewRenderer({
  code,
  language,
}: {
  code: string;
  language: string;
}) {
  switch (language) {
    case "html":
      return <HtmlPreview code={code} />;
    case "markdown":
    case "md":
      return <MarkdownPreview code={code} />;
    case "mermaid":
      return <MermaidPreview code={code} />;
    default:
      return null;
  }
}
