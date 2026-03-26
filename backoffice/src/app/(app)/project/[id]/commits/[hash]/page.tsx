"use client";

import { useParams } from "next/navigation";
import { CommitDetailContent } from "~/components/project-detail/commit-detail-content";

export default function CommitDetailPage() {
  const params = useParams<{ id: string; hash: string }>();
  return (
    <CommitDetailContent projectId={params.id} commitHash={params.hash} />
  );
}
