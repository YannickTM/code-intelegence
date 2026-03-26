"use client";

import { useParams } from "next/navigation";
import { CommitsContent } from "~/components/project-detail/commits-content";

export default function ProjectCommitsPage() {
  const params = useParams<{ id: string }>();
  return <CommitsContent projectId={params.id} />;
}
