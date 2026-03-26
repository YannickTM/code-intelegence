"use client";

import Link from "next/link";
import { useParams, usePathname } from "next/navigation";
import { AlertCircle } from "lucide-react";
import { cn } from "~/lib/utils";
import { useProjectDetail } from "~/hooks/use-project-detail";
import { ProjectDetailHeader } from "~/components/project-detail/project-detail-header";
import { ProjectDetailSkeleton } from "~/components/project-detail/project-detail-skeleton";

export default function ProjectDetailLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const params = useParams<{ id: string }>();
  const pathname = usePathname();
  const { project, isLoading, isError, error } =
    useProjectDetail(params.id);

  const projectBase = `/project/${params.id}`;
  const tabs = [
    { label: "Code", href: projectBase, exact: true, alsoMatch: `${projectBase}/file` },
    { label: "Code Search", href: `${projectBase}/search` },
    { label: "Commits", href: `${projectBase}/commits` },
    { label: "Symbols", href: `${projectBase}/symbols` },
    { label: "Indexing", href: `${projectBase}/jobs` },
    { label: "Settings", href: `${projectBase}/settings` },
  ];

  if (isLoading) return <ProjectDetailSkeleton />;

  if (isError || !project) {
    return (
      <div className="text-muted-foreground flex flex-col items-center justify-center gap-2 py-20">
        <AlertCircle className="size-8" />
        <p className="font-medium">Failed to load project</p>
        <p className="text-sm">
          {error?.message ?? "The project could not be found."}
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col gap-0">
      <ProjectDetailHeader project={project} projectId={params.id} />

      {/* Tab Navigation */}
      <nav aria-label="Project tabs" className="border-b">
        <div className="flex gap-0">
          {tabs.map((tab) => {
            const isActive =
              "exact" in tab && tab.exact
                ? pathname === tab.href ||
                  ("alsoMatch" in tab &&
                    !!tab.alsoMatch &&
                    pathname.startsWith(tab.alsoMatch))
                : pathname.startsWith(tab.href);
            return (
              <Link
                key={tab.href}
                href={tab.href}
                aria-current={isActive ? "page" : undefined}
                className={cn(
                  "border-b-2 px-4 py-2 text-sm font-medium transition-colors",
                  isActive
                    ? "border-primary text-foreground"
                    : "text-muted-foreground hover:text-foreground border-transparent",
                )}
              >
                {tab.label}
              </Link>
            );
          })}
        </div>
      </nav>

      {/* Tab Content */}
      <div className="flex flex-1 flex-col pt-8">{children}</div>
    </div>
  );
}
