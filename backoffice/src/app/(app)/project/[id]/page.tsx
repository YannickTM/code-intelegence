"use client";

import { useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import { useProjectDetail } from "~/hooks/use-project-detail";
import { IndexSummaryCard } from "~/components/project-detail/index-summary-card";
import { SSHKeyCard } from "~/components/project-detail/ssh-key-card";
import { QuickActionsCard } from "~/components/project-detail/quick-actions-card";
import { FileBrowser } from "~/components/project-detail/file-browser";

export default function ProjectOverviewPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const { project } = useProjectDetail(params.id);

  const handleFileSelect = useCallback(
    (path: string | null) => {
      if (path) {
        router.push(
          `/project/${params.id}/file?path=${encodeURIComponent(path)}`,
        );
      }
    },
    [router, params.id],
  );

  if (!project) return null; // layout handles loading/error

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1fr_340px]">
      {/* Left Column: File Tree */}
      <FileBrowser
        projectId={params.id}
        selectedFilePath={null}
        onFileSelect={handleFileSelect}
      />

      {/* Right Column */}
      <div className="flex flex-col gap-4">
        <IndexSummaryCard project={project} />
        <SSHKeyCard projectId={params.id} />
        <QuickActionsCard project={project} />
      </div>
    </div>
  );
}
