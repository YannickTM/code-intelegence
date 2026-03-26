"use client";

import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import Link from "next/link";
import { toast } from "sonner";
import { Button } from "~/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";
import { Label } from "~/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select";
import { api } from "~/trpc/react";

const REPO_URL_RE = /^(https?:\/\/.+|ssh:\/\/.+|git@.+:.+)$/;

function isValidRepoUrl(url: string): boolean {
  return REPO_URL_RE.test(url);
}

export function CreateProjectDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [name, setName] = useState("");
  const [repoUrl, setRepoUrl] = useState("");
  const [defaultBranch, setDefaultBranch] = useState("main");
  const [sshKeyId, setSshKeyId] = useState("");

  const sshKeys = api.sshKeys.list.useQuery(undefined, {
    retry: false,
    enabled: open,
  });

  const utils = api.useUtils();

  const createProject = api.projects.create.useMutation({
    onSuccess: () => {
      toast.success("Project created successfully");
      void utils.users.listMyProjects.invalidate();
      onOpenChange(false);
    },
    onError: (error) => {
      toast.error(`Failed to create project: ${error.message}`);
    },
  });

  /* eslint-disable react-hooks/set-state-in-effect -- intentional reset when dialog closes */
  useEffect(() => {
    if (!open) {
      setName("");
      setRepoUrl("");
      setDefaultBranch("main");
      setSshKeyId("");
      createProject.reset();
    }
  }, [open]); // eslint-disable-line react-hooks/exhaustive-deps
  /* eslint-enable react-hooks/set-state-in-effect */

  const trimmedRepoUrl = repoUrl.trim();
  const repoUrlInvalid =
    trimmedRepoUrl.length > 0 && !isValidRepoUrl(trimmedRepoUrl);

  const canSubmit =
    name.trim().length > 0 &&
    trimmedRepoUrl.length > 0 &&
    !repoUrlInvalid &&
    sshKeyId !== "";

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!canSubmit) return;
    createProject.mutate({
      name: name.trim(),
      repo_url: repoUrl.trim(),
      default_branch: defaultBranch.trim() || "main",
      ssh_key_id: sshKeyId,
    });
  };

  const sshKeysAvailable =
    !sshKeys.isError &&
    !sshKeys.isLoading &&
    (sshKeys.data?.items?.length ?? 0) > 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Create Project</DialogTitle>
          <DialogDescription>
            Connect a Git repository to start indexing.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="project-name">Name</Label>
            <Input
              id="project-name"
              placeholder="My Project"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={100}
              required
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="repo-url">Repository URL</Label>
            <Input
              id="repo-url"
              placeholder="https://github.com/org/repo.git"
              value={repoUrl}
              onChange={(e) => setRepoUrl(e.target.value)}
              required
            />
            {repoUrlInvalid && (
              <p className="text-destructive text-xs">
                URL must start with https://, http://, ssh://, or git@
              </p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="default-branch">Default Branch</Label>
            <Input
              id="default-branch"
              placeholder="main"
              value={defaultBranch}
              onChange={(e) => setDefaultBranch(e.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="ssh-key">SSH Key</Label>
            {sshKeys.isLoading && (
              <div className="text-muted-foreground flex items-center gap-2 text-sm">
                <Loader2 className="size-4 animate-spin" />
                Loading SSH keys...
              </div>
            )}
            {sshKeys.isError && (
              <p className="text-muted-foreground text-sm">
                SSH keys are not available yet.{" "}
                <Link
                  href="/settings/ssh-keys"
                  className="text-primary underline"
                  onClick={() => onOpenChange(false)}
                >
                  Manage SSH keys
                </Link>
              </p>
            )}
            {!sshKeys.isLoading &&
              !sshKeys.isError &&
              (sshKeys.data?.items?.length ?? 0) === 0 && (
                <p className="text-muted-foreground text-sm">
                  No SSH keys found.{" "}
                  <Link
                    href="/settings/ssh-keys"
                    className="text-primary underline"
                    onClick={() => onOpenChange(false)}
                  >
                    Add an SSH key
                  </Link>
                </p>
              )}
            {sshKeysAvailable && (
              <Select value={sshKeyId} onValueChange={setSshKeyId}>
                <SelectTrigger id="ssh-key">
                  <SelectValue placeholder="Select an SSH key" />
                </SelectTrigger>
                <SelectContent>
                  {sshKeys.data?.items.map((key) => (
                    <SelectItem key={key.id} value={key.id}>
                      {key.name}{" "}
                      <span className="text-muted-foreground">
                        ({key.fingerprint.slice(0, 16)}...)
                      </span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={!canSubmit || createProject.isPending}
            >
              {createProject.isPending && (
                <Loader2 className="size-4 animate-spin" />
              )}
              Create Project
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
