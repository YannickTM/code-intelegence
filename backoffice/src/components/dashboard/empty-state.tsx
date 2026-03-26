"use client";

import Link from "next/link";
import { FolderGit2, Plus } from "lucide-react";
import { Button } from "~/components/ui/button";

export function EmptyState() {
  return (
    <div className="flex flex-1 items-center justify-center rounded-xl border border-dashed p-16">
      <div className="flex flex-col items-center gap-4 text-center">
        <div className="rounded-full bg-primary/5 p-4">
          <FolderGit2 className="text-primary/60 size-12" />
        </div>
        <div>
          <h3 className="text-lg font-semibold">
            Connect your first repository
          </h3>
          <p className="text-muted-foreground text-sm">
            Create a project, assign an SSH deploy key, and trigger your first
            index.
          </p>
        </div>
        <Button asChild>
          <Link href="/project/create">
            <Plus className="size-4" />
            Create Project
          </Link>
        </Button>
      </div>
    </div>
  );
}
