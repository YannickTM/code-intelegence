"use client";

import { useEffect, useState } from "react";
import { Button } from "~/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
import { Input } from "~/components/ui/input";
import { Label } from "~/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select";
import type { ProjectRole, ProjectWithHealth } from "~/lib/dashboard-types";
import { useProjectDetailMutations } from "~/hooks/use-project-detail-mutations";

export function GeneralSettingsSection({
  project,
  projectId,
  role,
}: {
  project: ProjectWithHealth;
  projectId: string;
  role: ProjectRole;
}) {
  const { updateProject } = useProjectDetailMutations(projectId);
  const isReadOnly = role === "member";

  const [name, setName] = useState(project.name);
  const [repoUrl, setRepoUrl] = useState(project.repo_url);
  const [status, setStatus] = useState(project.status);

  // Sync local state when server data changes (e.g. refetch, external update)
  /* eslint-disable react-hooks/set-state-in-effect -- intentional sync from server data */
  useEffect(() => {
    setName(project.name);
    setRepoUrl(project.repo_url);
    setStatus(project.status);
  }, [project.name, project.repo_url, project.status]);
  /* eslint-enable react-hooks/set-state-in-effect */

  const isDirty =
    name !== project.name ||
    repoUrl !== project.repo_url ||
    status !== project.status;

  function handleSave() {
    const updates: Record<string, string> = {};
    if (name !== project.name) updates.name = name;
    if (repoUrl !== project.repo_url) updates.repo_url = repoUrl;
    if (status !== project.status) updates.status = status;

    updateProject.mutate({ id: projectId, ...updates });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>General</CardTitle>
        <CardDescription>
          {isReadOnly
            ? "View project name, repository, and status."
            : "Update your project name, repository, and status."}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="project-name">Name</Label>
          <Input
            id="project-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            readOnly={isReadOnly}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="repo-url">Repository URL</Label>
          <Input
            id="repo-url"
            value={repoUrl}
            onChange={(e) => setRepoUrl(e.target.value)}
            readOnly={isReadOnly}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="default-branch">Branch</Label>
          <Input
            id="default-branch"
            value={project.default_branch}
            readOnly
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="status">Status</Label>
          <Select value={status} onValueChange={setStatus} disabled={isReadOnly}>
            <SelectTrigger id="status" className="w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="active">Active</SelectItem>
              <SelectItem value="paused">Paused</SelectItem>
            </SelectContent>
          </Select>
        </div>
        {!isReadOnly && (
        <div className="flex gap-2 pt-2">
          <Button
            onClick={handleSave}
            disabled={!isDirty || updateProject.isPending}
          >
            {updateProject.isPending ? "Saving..." : "Save"}
          </Button>
          {isDirty && (
            <Button
              variant="ghost"
              onClick={() => {
                setName(project.name);
                setRepoUrl(project.repo_url);
                setStatus(project.status);
              }}
            >
              Cancel
            </Button>
          )}
        </div>
        )}
      </CardContent>
    </Card>
  );
}
