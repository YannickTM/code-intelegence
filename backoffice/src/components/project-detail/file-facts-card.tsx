"use client";

import { Tags } from "lucide-react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
import { Badge } from "~/components/ui/badge";
import type { FileFacts } from "~/server/api/routers/project-files";

const FACT_BADGES: {
  key: keyof FileFacts;
  label: string;
  className: string;
}[] = [
  { key: "has_jsx", label: "JSX", className: "bg-cyan-100 text-cyan-800 dark:bg-cyan-900/30 dark:text-cyan-300" },
  { key: "has_default_export", label: "Default Export", className: "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300" },
  { key: "has_named_exports", label: "Named Exports", className: "border-green-300 text-green-800 dark:border-green-700 dark:text-green-300" },
  { key: "has_top_level_side_effects", label: "Side Effects", className: "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300" },
  { key: "has_react_hook_calls", label: "React Hooks", className: "border-cyan-300 text-cyan-800 dark:border-cyan-700 dark:text-cyan-300" },
  { key: "has_fetch_calls", label: "Fetch Calls", className: "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300" },
  { key: "has_class_declarations", label: "Classes", className: "bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300" },
  { key: "has_tests", label: "Tests", className: "bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300" },
  { key: "has_config_patterns", label: "Config", className: "bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300" },
];

export function FileFactsCard({
  fileFacts,
}: {
  fileFacts: FileFacts | null | undefined;
}) {
  if (!fileFacts) return null;

  const activeBadges = FACT_BADGES.filter(
    (f) => fileFacts[f.key] === true,
  );
  const hasRuntime = fileFacts.jsx_runtime && fileFacts.jsx_runtime.length > 0;

  if (activeBadges.length === 0 && !hasRuntime) return null;

  const count = activeBadges.length + (hasRuntime ? 1 : 0);

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm font-medium">
          <Tags className="size-4" />
          File Facts
          <Badge variant="secondary" className="text-xs">
            {count}
          </Badge>
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap gap-1.5">
          {activeBadges.map((fact) => (
            <Badge
              key={fact.key}
              variant={fact.className.startsWith("border") ? "outline" : "secondary"}
              className={`text-xs ${fact.className}`}
            >
              {fact.label}
            </Badge>
          ))}
          {hasRuntime && (
            <Badge
              variant="secondary"
              className="bg-cyan-100 text-cyan-800 text-xs dark:bg-cyan-900/30 dark:text-cyan-300"
            >
              {fileFacts.jsx_runtime}
            </Badge>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
