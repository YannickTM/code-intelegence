"use client";

import Link from "next/link";
import { FolderGit2, Plus } from "lucide-react";
import { Button } from "~/components/ui/button";

export function ProjectsEmptyState() {
  return (
    <div className="flex flex-1 items-center justify-center rounded-xl border border-dashed p-16">
      <div className="flex flex-col items-center gap-4 text-center">
        <FolderGit2
          className="text-muted-foreground size-12"
          aria-hidden="true"
        />
        <div>
          <h3 className="text-lg font-semibold">No projects yet</h3>
          <p className="text-muted-foreground text-sm">
            Connect your first repository to start indexing.
          </p>
        </div>
        <Button asChild>
          <Link href="/project/create">
            <Plus className="size-4" aria-hidden="true" />
            Create Project
          </Link>
        </Button>
      </div>
    </div>
  );
}
