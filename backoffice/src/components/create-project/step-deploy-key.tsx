"use client";

import { useState } from "react";
import {
  Check,
  ChevronsUpDown,
  Copy,
  ExternalLink,
  Loader2,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "~/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
import { Checkbox } from "~/components/ui/checkbox";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "~/components/ui/collapsible";
import { Input } from "~/components/ui/input";
import { Label } from "~/components/ui/label";
import { api } from "~/trpc/react";
import {
  detectProvider,
  getDeployKeyUrl,
  getProviderLabel,
} from "~/lib/repo-url";
import type { WizardAction, WizardState } from "~/lib/wizard-state";

export function StepDeployKey({
  state,
  dispatch,
}: {
  state: WizardState;
  dispatch: React.Dispatch<WizardAction>;
}) {
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState("");
  const [instructionsOpen, setInstructionsOpen] = useState(true);

  const utils = api.useUtils();

  const createProject = api.projects.create.useMutation({
    onSuccess: (data) => {
      dispatch({
        type: "SET_CREATED_PROJECT",
        project: { id: data.id, name: data.name },
      });
      void utils.users.listMyProjects.invalidate();
      void utils.dashboard.summary.invalidate();
      dispatch({ type: "NEXT_STEP" });
    },
    onError: (err) => {
      setError(err.message);
      toast.error(`Failed to create project: ${err.message}`);
    },
  });

  const publicKey = state.resolvedKey?.public_key ?? "";
  const fingerprint = state.resolvedKey?.fingerprint ?? "";
  const provider = detectProvider(state.repoUrl);
  const deepLink = getDeployKeyUrl(state.repoUrl, provider);
  const providerLabel = getProviderLabel(provider);

  const sshKeyId =
    state.sshKeyMode === "generate"
      ? (state.resolvedKey?.id ?? "")
      : state.existingKeyId;

  function handleCopy() {
    void navigator.clipboard.writeText(publicKey).then(
      () => {
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      },
      () => {
        toast.error("Failed to copy to clipboard");
      },
    );
  }

  function handleCreateProject() {
    setError("");
    createProject.mutate({
      name: state.projectName.trim(),
      repo_url: state.repoUrl.trim(),
      default_branch: state.defaultBranch.trim() || "main",
      ssh_key_id: sshKeyId,
    });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Add this deploy key to your repository</CardTitle>
        <CardDescription>
          Copy the public key below and add it as a read-only deploy key in your
          Git provider. This lets MYJUNGLE clone your repository.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Public key display */}
        <div className="space-y-2">
          <Label>Public Key</Label>
          <div className="flex gap-2">
            <Input
              readOnly
              value={publicKey}
              className="font-mono text-xs"
              onClick={(e) => (e.target as HTMLInputElement).select()}
            />
            <Button
              variant="outline"
              size="icon"
              onClick={handleCopy}
              title="Copy public key"
            >
              {copied ? (
                <Check className="size-4" />
              ) : (
                <Copy className="size-4" />
              )}
            </Button>
          </div>
          <p className="text-muted-foreground text-xs">
            Fingerprint: {fingerprint}
          </p>
        </div>

        {/* Provider deep link */}
        {deepLink && (
          <Button variant="outline" asChild className="w-full">
            <a href={deepLink} target="_blank" rel="noopener noreferrer">
              <ExternalLink className="size-4" />
              Open {providerLabel} deploy key settings
            </a>
          </Button>
        )}

        {/* Inline instructions */}
        <Collapsible open={instructionsOpen} onOpenChange={setInstructionsOpen}>
          <CollapsibleTrigger asChild>
            <Button variant="ghost" size="sm" className="gap-2 px-0">
              <ChevronsUpDown className="size-4" />
              How to add a deploy key
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <div className="text-muted-foreground mt-2 space-y-1 rounded-md border p-3 text-sm">
              {provider === "github" && (
                <ol className="list-inside list-decimal space-y-1">
                  <li>
                    Click &quot;Open GitHub deploy key settings&quot; above
                  </li>
                  <li>Paste the key into the &quot;Key&quot; field</li>
                  <li>Set a title (e.g. &quot;MYJUNGLE deploy key&quot;)</li>
                  <li>Leave &quot;Allow write access&quot; unchecked</li>
                  <li>Click &quot;Add key&quot;</li>
                </ol>
              )}
              {provider === "gitlab" && (
                <ol className="list-inside list-decimal space-y-1">
                  <li>
                    Click &quot;Open GitLab deploy key settings&quot; above
                  </li>
                  <li>Scroll to the &quot;Deploy keys&quot; section</li>
                  <li>Paste the key and give it a title</li>
                  <li>
                    Ensure &quot;Grant write permissions&quot; is unchecked
                  </li>
                  <li>Click &quot;Add key&quot;</li>
                </ol>
              )}
              {provider === "bitbucket" && (
                <ol className="list-inside list-decimal space-y-1">
                  <li>
                    Click &quot;Open Bitbucket deploy key settings&quot; above
                  </li>
                  <li>Give the key a label</li>
                  <li>Paste the key into the &quot;Key&quot; field</li>
                  <li>Click &quot;Add key&quot;</li>
                </ol>
              )}
              {provider === "unknown" && (
                <ol className="list-inside list-decimal space-y-1">
                  <li>
                    Go to your repository&apos;s settings in your Git provider
                  </li>
                  <li>
                    Find the &quot;Deploy keys&quot; or &quot;Access keys&quot;
                    section
                  </li>
                  <li>Add a new key and paste the public key above</li>
                  <li>Ensure the key has read-only access</li>
                  <li>Save the key</li>
                </ol>
              )}
            </div>
          </CollapsibleContent>
        </Collapsible>

        {/* Confirmation checkbox */}
        <div className="flex items-center space-x-2 pt-2">
          <Checkbox
            id="deploy-key-confirmed"
            checked={state.deployKeyConfirmed}
            onCheckedChange={(checked) =>
              dispatch({
                type: "SET_DEPLOY_KEY_CONFIRMED",
                value: checked === true,
              })
            }
          />
          <Label htmlFor="deploy-key-confirmed" className="text-sm font-normal">
            I&apos;ve added the deploy key to my repository
          </Label>
        </div>

        {/* Error */}
        {error && <p className="text-destructive text-sm">{error}</p>}
      </CardContent>
      <CardFooter className="justify-between">
        <Button
          variant="outline"
          onClick={() => dispatch({ type: "PREV_STEP" })}
        >
          Back
        </Button>
        <Button
          onClick={handleCreateProject}
          disabled={!state.deployKeyConfirmed || createProject.isPending}
        >
          {createProject.isPending && (
            <Loader2 className="size-4 animate-spin" />
          )}
          Create Project
        </Button>
      </CardFooter>
    </Card>
  );
}
