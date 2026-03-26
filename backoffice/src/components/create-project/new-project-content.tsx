"use client";

import { useEffect, useReducer, useState } from "react";
import { useRouter } from "next/navigation";
import { X } from "lucide-react";
import { Button } from "~/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "~/components/ui/dialog";

import { StepIndicator } from "./step-indicator";
import { StepProjectDetails } from "./step-project-details";
import { StepSSHKey } from "./step-ssh-key";
import { StepDeployKey } from "./step-deploy-key";
import { StepDone } from "./step-done";
import { initialWizardState, wizardReducer } from "~/lib/wizard-state";

export function NewProjectContent() {
  const router = useRouter();
  const [state, dispatch] = useReducer(wizardReducer, initialWizardState);
  const [cancelDialogOpen, setCancelDialogOpen] = useState(false);

  // Auto-populate key name when entering step 2
  useEffect(() => {
    if (state.step === 2 && state.newKeyName === "" && state.projectName) {
      dispatch({
        type: "SET_NEW_KEY_NAME",
        value: `${state.projectName}-deploy-key`,
      });
    }
  }, [state.step, state.newKeyName, state.projectName]);

  function handleCancel() {
    // If SSH key was created but no project yet, warn
    if (state.resolvedKey && !state.createdProject) {
      setCancelDialogOpen(true);
      return;
    }

    // If project was already created, go to project page
    if (state.createdProject) {
      router.push(`/project/${state.createdProject.id}`);
      return;
    }

    // No side effects — safe to leave
    router.push("/project");
  }

  function confirmCancel() {
    setCancelDialogOpen(false);
    router.push("/project");
  }

  // Hide cancel button on step 4 (project already created)
  const showCancel = state.step < 4;

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">New Project</h1>
          <p className="text-muted-foreground text-sm">
            Set up a new repository connection.
          </p>
        </div>
        {showCancel && (
          <Button variant="ghost" size="icon" onClick={handleCancel}>
            <X className="size-5" />
            <span className="sr-only">Cancel</span>
          </Button>
        )}
      </div>

      {/* Step indicator */}
      <StepIndicator currentStep={state.step} />

      {/* Step content */}
      {state.step === 1 && (
        <StepProjectDetails state={state} dispatch={dispatch} />
      )}
      {state.step === 2 && <StepSSHKey state={state} dispatch={dispatch} />}
      {state.step === 3 && <StepDeployKey state={state} dispatch={dispatch} />}
      {state.step === 4 && <StepDone state={state} dispatch={dispatch} />}

      {/* Cancel confirmation dialog */}
      <Dialog open={cancelDialogOpen} onOpenChange={setCancelDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Leave project setup?</DialogTitle>
            <DialogDescription>
              An SSH key was already generated. It will remain in your key
              library. You can use it later.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setCancelDialogOpen(false)}
            >
              Keep editing
            </Button>
            <Button variant="destructive" onClick={confirmCancel}>
              Leave
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
