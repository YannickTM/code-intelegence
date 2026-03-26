"use client";

import { useParams } from "next/navigation";
import { SymbolList } from "~/components/project-detail/symbol-list";

export default function SymbolsPage() {
  const params = useParams<{ id: string }>();
  return <SymbolList projectId={params.id} />;
}
