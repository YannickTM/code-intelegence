"use client";

import { useParams } from "next/navigation";
import { JobsContent } from "~/components/project-detail/jobs-content";

export default function ProjectJobsPage() {
  const params = useParams<{ id: string }>();
  return <JobsContent projectId={params.id} />;
}
