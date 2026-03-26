"use client";

import { useState } from "react";
import { Key, KeyRound, Loader2 } from "lucide-react";
import Link from "next/link";
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
import { Input } from "~/components/ui/input";
import { Label } from "~/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select";
import { cn } from "~/lib/utils";
import { api } from "~/trpc/react";
import type { WizardAction, WizardState } from "~/lib/wizard-state";

export function StepSSHKey({
  state,
  dispatch,
}: {
  state: WizardState;
  dispatch: React.Dispatch<WizardAction>;
}) {
  const [error, setError] = useState("");

  // Only fetch keys when "existing" option is selected
  const sshKeys = api.sshKeys.list.useQuery(undefined, {
    retry: false,
    enabled: state.sshKeyMode === "existing",
  });

  const createKey = api.sshKeys.create.useMutation({
    onSuccess: (data) => {
      dispatch({
        type: "SET_RESOLVED_KEY",
        key: {
          id: data.id,
          name: data.name,
          public_key: data.public_key,
          fingerprint: data.fingerprint,
        },
      });
      dispatch({ type: "NEXT_STEP" });
    },
    onError: (err) => {
      setError(err.message);
      toast.error("Failed to generate SSH key. Please try again.");
    },
  });

  const activeKeys = sshKeys.data?.items.filter((k) => k.is_active) ?? [];

  const sshKeysAvailable =
    !sshKeys.isError && !sshKeys.isLoading && activeKeys.length > 0;

  const noKeysExist =
    !sshKeys.isLoading && !sshKeys.isError && activeKeys.length === 0;

  function handleContinue() {
    setError("");

    if (state.sshKeyMode === "generate") {
      const name = state.newKeyName.trim() || `${state.projectName}-deploy-key`;
      createKey.mutate({ name });
    } else {
      // Existing key — find full key object from list
      const selected = activeKeys.find((k) => k.id === state.existingKeyId);
      if (!selected) {
        setError("Please select an SSH key.");
        return;
      }
      dispatch({
        type: "SET_RESOLVED_KEY",
        key: {
          id: selected.id,
          name: selected.name,
          public_key: selected.public_key,
          fingerprint: selected.fingerprint,
        },
      });
      dispatch({ type: "NEXT_STEP" });
    }
  }

  const canContinue =
    state.sshKeyMode === "generate" ||
    (state.sshKeyMode === "existing" && state.existingKeyId !== "");

  return (
    <Card>
      <CardHeader>
        <CardTitle>Secure git access with an SSH key</CardTitle>
        <CardDescription>
          MYJUNGLE needs read access to your repository. We use a deploy key — a
          dedicated SSH key you&apos;ll add to your Git provider in the next
          step.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {/* Option A — Generate new key */}
        <div
          className={cn(
            "cursor-pointer rounded-lg border p-4 transition-colors",
            state.sshKeyMode === "generate"
              ? "border-primary bg-primary/5"
              : "border-border hover:border-muted-foreground/40",
          )}
          onClick={() =>
            dispatch({ type: "SET_SSH_KEY_MODE", mode: "generate" })
          }
        >
          <div className="flex items-center gap-3">
            <KeyRound className="text-muted-foreground size-5 shrink-0" />
            <div>
              <p className="font-medium">Generate a new deploy key</p>
              <p className="text-muted-foreground text-sm">
                We&apos;ll create a dedicated Ed25519 key pair for this project.
              </p>
            </div>
          </div>
          {state.sshKeyMode === "generate" && (
            <div className="mt-3 space-y-2 pl-8">
              <Label htmlFor="key-name">Key Name</Label>
              <Input
                id="key-name"
                placeholder={`${state.projectName}-deploy-key`}
                value={state.newKeyName}
                onChange={(e) =>
                  dispatch({ type: "SET_NEW_KEY_NAME", value: e.target.value })
                }
                maxLength={100}
              />
            </div>
          )}
        </div>

        {/* Option B — Use existing key */}
        <div
          className={cn(
            "rounded-lg border p-4 transition-colors",
            noKeysExist && state.sshKeyMode !== "existing"
              ? "cursor-not-allowed opacity-60"
              : "cursor-pointer",
            state.sshKeyMode === "existing"
              ? "border-primary bg-primary/5"
              : "border-border hover:border-muted-foreground/40",
          )}
          onClick={() => {
            if (!noKeysExist || state.sshKeyMode === "existing") {
              dispatch({ type: "SET_SSH_KEY_MODE", mode: "existing" });
            }
          }}
        >
          <div className="flex items-center gap-3">
            <Key className="text-muted-foreground size-5 shrink-0" />
            <div>
              <p className="font-medium">Use an existing key from my library</p>
              <p className="text-muted-foreground text-sm">
                {noKeysExist
                  ? "No keys in your library yet. Generate one above."
                  : "Assign a key you've already created."}
              </p>
            </div>
          </div>
          {state.sshKeyMode === "existing" && (
            <div className="mt-3 pl-8">
              {sshKeys.isLoading && (
                <div className="text-muted-foreground flex items-center gap-2 text-sm">
                  <Loader2 className="size-4 animate-spin" />
                  Loading keys…
                </div>
              )}
              {sshKeys.isError && (
                <p className="text-muted-foreground text-sm">
                  SSH keys are not available yet.{" "}
                  <Link
                    href="/settings/ssh-keys"
                    className="text-primary underline"
                  >
                    Manage SSH keys
                  </Link>
                </p>
              )}
              {sshKeysAvailable && (
                <Select
                  value={state.existingKeyId}
                  onValueChange={(v) =>
                    dispatch({ type: "SET_EXISTING_KEY_ID", value: v })
                  }
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select an SSH key" />
                  </SelectTrigger>
                  <SelectContent>
                    {activeKeys.map((key) => (
                      <SelectItem key={key.id} value={key.id}>
                        {key.name}{" "}
                        <span className="text-muted-foreground">
                          ({key.fingerprint.slice(0, 16)}…)
                        </span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            </div>
          )}
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
          onClick={handleContinue}
          disabled={!canContinue || createKey.isPending}
        >
          {createKey.isPending && <Loader2 className="size-4 animate-spin" />}
          Continue
        </Button>
      </CardFooter>
    </Card>
  );
}
