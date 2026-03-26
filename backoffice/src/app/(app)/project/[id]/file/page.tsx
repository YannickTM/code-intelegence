"use client";

import { useParams, useSearchParams } from "next/navigation";
import { FileX } from "lucide-react";
import { Button } from "~/components/ui/button";
import Link from "next/link";
import { FileViewerContent } from "~/components/project-detail/file-viewer-content";

export default function FileViewerPage() {
  const params = useParams<{ id: string }>();
  const searchParams = useSearchParams();
  const filePath = searchParams.get("path");

  if (!filePath) {
    return (
      <div className="flex flex-col items-center justify-center gap-4 py-20">
        <FileX className="text-muted-foreground size-12" />
        <h2 className="text-lg font-semibold">No file specified</h2>
        <p className="text-muted-foreground text-sm">
          Select a file from the Code tab to view its contents.
        </p>
        <Button variant="outline" asChild>
          <Link href={`/project/${params.id}`}>Back to files</Link>
        </Button>
      </div>
    );
  }

  return <FileViewerContent projectId={params.id} filePath={filePath} />;
}
