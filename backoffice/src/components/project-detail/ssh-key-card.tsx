"use client";

import { AlertCircle, Copy, ExternalLink, KeyRound } from "lucide-react";
import { toast } from "sonner";
import { Button } from "~/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card";
import { Skeleton } from "~/components/ui/skeleton";
import { api } from "~/trpc/react";

export function SSHKeyCard({ projectId }: { projectId: string }) {
  const sshKeyQuery = api.projects.getSSHKey.useQuery(
    { id: projectId },
    { retry: false },
  );

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-medium">SSH Deploy Key</CardTitle>
      </CardHeader>
      <CardContent>
        {sshKeyQuery.isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-4 w-56" />
          </div>
        ) : sshKeyQuery.isError ? (
          <div className="text-destructive flex flex-col items-center gap-2 py-4">
            <AlertCircle className="size-6" />
            <p className="text-sm">Failed to load SSH key</p>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => void sshKeyQuery.refetch()}
            >
              Retry
            </Button>
          </div>
        ) : sshKeyQuery.data ? (
          <div className="space-y-3 text-sm">
            <div className="flex justify-between">
              <span className="text-muted-foreground">Name</span>
              <span>{sshKeyQuery.data.name}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Fingerprint</span>
              <code className="max-w-[200px] truncate font-mono text-xs">
                {sshKeyQuery.data.fingerprint}
              </code>
            </div>
            <div>
              <label className="text-muted-foreground mb-1 block">
                Public Key
              </label>
              <div className="flex items-start gap-2">
                <code className="bg-muted flex-1 rounded-md p-2 font-mono text-xs break-all">
                  {sshKeyQuery.data.public_key}
                </code>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-8 shrink-0"
                  onClick={() => {
                    navigator.clipboard
                      .writeText(sshKeyQuery.data!.public_key)
                      .then(
                        () => toast.success("Public key copied"),
                        () => toast.error("Failed to copy to clipboard"),
                      );
                  }}
                >
                  <Copy className="size-3.5" />
                  <span className="sr-only">Copy</span>
                </Button>
              </div>
            </div>
            <div className="flex flex-col gap-1 pt-1">
              <span className="text-muted-foreground text-xs">
                Add as deploy key:
              </span>
              <div className="flex flex-wrap gap-2">
                {[
                  {
                    label: "GitHub",
                    href: "https://docs.github.com/en/authentication/connecting-to-github-with-ssh/managing-deploy-keys#deploy-keys",
                  },
                  {
                    label: "GitLab",
                    href: "https://docs.gitlab.com/ee/user/project/deploy_keys/",
                  },
                  {
                    label: "Bitbucket",
                    href: "https://support.atlassian.com/bitbucket-cloud/docs/add-access-keys/",
                  },
                ].map((link) => (
                  <a
                    key={link.label}
                    href={link.href}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-muted-foreground hover:text-foreground inline-flex items-center gap-1 text-xs"
                  >
                    {link.label}
                    <ExternalLink className="size-3" />
                  </a>
                ))}
              </div>
            </div>
          </div>
        ) : (
          <div className="text-muted-foreground flex flex-col items-center gap-2 py-4">
            <KeyRound className="size-6" />
            <p className="text-sm">No SSH key assigned</p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
