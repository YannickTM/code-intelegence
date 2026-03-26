"use client";

import { useParams } from "next/navigation";
import { CodeSearch } from "~/components/project-detail/code-search";

export default function CodeSearchPage() {
  const params = useParams<{ id: string }>();
  return <CodeSearch projectId={params.id} />;
}
