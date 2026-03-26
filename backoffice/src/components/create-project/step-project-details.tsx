"use client";

import { Button } from "~/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
import { Input } from "~/components/ui/input";
import { Label } from "~/components/ui/label";
import { extractProjectName, isValidRepoUrl } from "~/lib/repo-url";
import type { WizardAction, WizardState } from "~/lib/wizard-state";

export function StepProjectDetails({
  state,
  dispatch,
}: {
  state: WizardState;
  dispatch: React.Dispatch<WizardAction>;
}) {
  const trimmedUrl = state.repoUrl.trim();
  const urlTouched = trimmedUrl.length > 0;
  const urlInvalid = urlTouched && !isValidRepoUrl(trimmedUrl);

  const canContinue =
    trimmedUrl.length > 0 && !urlInvalid && state.projectName.trim().length > 0;

  function handleRepoUrlChange(e: React.ChangeEvent<HTMLInputElement>) {
    const value = e.target.value;
    dispatch({ type: "SET_REPO_URL", value });

    // Auto-populate project name if user hasn't manually edited it
    if (!state.projectNameManuallyEdited) {
      const name = extractProjectName(value);
      if (name) {
        dispatch({ type: "SET_PROJECT_NAME", value: name, manual: false });
      }
    }
  }

  function handleProjectNameChange(e: React.ChangeEvent<HTMLInputElement>) {
    dispatch({
      type: "SET_PROJECT_NAME",
      value: e.target.value,
      manual: true,
    });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Let&apos;s connect your repository</CardTitle>
        <CardDescription>
          Enter the repository URL and choose a project name.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Repository URL */}
        <div className="space-y-2">
          <Label htmlFor="repo-url">Repository URL</Label>
          <Input
            id="repo-url"
            placeholder="git@github.com:org/repo.git or https://github.com/org/repo"
            value={state.repoUrl}
            onChange={handleRepoUrlChange}
            autoFocus
          />
          {urlInvalid && (
            <p className="text-destructive text-xs">
              Must start with git@, https://, http://, or ssh://
            </p>
          )}
        </div>

        {/* Project Name */}
        <div className="space-y-2">
          <Label htmlFor="project-name">Project Name</Label>
          <Input
            id="project-name"
            placeholder="my-project"
            value={state.projectName}
            onChange={handleProjectNameChange}
            maxLength={100}
          />
          <p className="text-muted-foreground text-xs">
            Auto-generated from the URL. You can edit it.
          </p>
        </div>

        {/* Default Branch */}
        <div className="space-y-2">
          <Label htmlFor="default-branch">Default Branch</Label>
          <Input
            id="default-branch"
            placeholder="main"
            value={state.defaultBranch}
            onChange={(e) =>
              dispatch({ type: "SET_DEFAULT_BRANCH", value: e.target.value })
            }
          />
        </div>
      </CardContent>
      <CardFooter className="justify-end">
        <Button
          onClick={() => dispatch({ type: "NEXT_STEP" })}
          disabled={!canContinue}
        >
          Continue
        </Button>
      </CardFooter>
    </Card>
  );
}
