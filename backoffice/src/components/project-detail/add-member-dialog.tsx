"use client";

import { useState } from "react";
import { ArrowLeft, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Avatar, AvatarFallback, AvatarImage } from "~/components/ui/avatar";
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
import { getInitials } from "~/lib/format";

interface AddMemberDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projectId: string;
  actorRole: string;
}

type ResolvedUser = {
  id: string;
  username: string;
  email?: string;
  display_name?: string;
  avatar_url?: string;
};

export function AddMemberDialog({
  open,
  onOpenChange,
  projectId,
  actorRole,
}: AddMemberDialogProps) {
  const utils = api.useUtils();

  const [query, setQuery] = useState("");
  const [selectedRole, setSelectedRole] = useState<string>("member");
  const [resolvedUser, setResolvedUser] = useState<ResolvedUser | null>(null);
  const [step, setStep] = useState<"search" | "confirm">("search");
  const [fieldError, setFieldError] = useState<string | null>(null);
  const [isLooking, setIsLooking] = useState(false);

  const addMutation = api.projectMembers.add.useMutation({
    onSuccess: (_data, variables) => {
      toast.success(
        `${resolvedUser?.username ?? "User"} added as ${variables.role ?? "member"}`,
      );
      void utils.projectMembers.list.invalidate({ projectId });
      handleClose();
    },
    onError: (err) => {
      if (err.data?.code === "CONFLICT") {
        toast.error(
          `${resolvedUser?.username ?? "User"} is already a member of this project`,
        );
      } else if (err.data?.code === "FORBIDDEN") {
        toast.error("You don't have permission to add members with that role");
      } else {
        toast.error("Something went wrong. Please try again.");
      }
    },
  });

  function handleClose() {
    onOpenChange(false);
    // Reset state after animation
    setTimeout(() => {
      setQuery("");
      setSelectedRole("member");
      setResolvedUser(null);
      setStep("search");
      setFieldError(null);
      setIsLooking(false);
      addMutation.reset();
    }, 200);
  }

  async function handleLookup() {
    const trimmed = query.trim();
    if (!trimmed) {
      setFieldError("Username or email is required");
      return;
    }

    setFieldError(null);
    setIsLooking(true);

    try {
      const result = await utils.users.lookupUser.fetch({ q: trimmed });
      setResolvedUser(result.user);
      setStep("confirm");
    } catch (err: unknown) {
      const trpcErr = err as { data?: { code?: string } };
      if (trpcErr?.data?.code === "NOT_FOUND") {
        setFieldError("No user found with that username or email");
      } else {
        setFieldError("Something went wrong. Please try again.");
      }
    } finally {
      setIsLooking(false);
    }
  }

  function handleAdd() {
    if (!resolvedUser) return;
    addMutation.mutate({
      projectId,
      user_id: resolvedUser.id,
      role: selectedRole as "owner" | "admin" | "member",
    });
  }

  function handleBack() {
    setResolvedUser(null);
    setStep("search");
  }

  return (
    <Dialog open={open} onOpenChange={(o) => (o ? onOpenChange(true) : handleClose())}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Add Member</DialogTitle>
          <DialogDescription>
            {step === "search"
              ? "Look up a user by username or email to add them to this project."
              : "Confirm the user details before adding them."}
          </DialogDescription>
        </DialogHeader>

        {step === "search" ? (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="member-query">Username or email</Label>
              <Input
                id="member-query"
                placeholder="e.g. jane-doe or jane@example.com"
                value={query}
                onChange={(e) => {
                  setQuery(e.target.value);
                  setFieldError(null);
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    void handleLookup();
                  }
                }}
              />
              {fieldError && (
                <p className="text-destructive text-sm">{fieldError}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="member-role">Role</Label>
              <Select value={selectedRole} onValueChange={setSelectedRole}>
                <SelectTrigger id="member-role">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="member">Member</SelectItem>
                  <SelectItem value="admin">Admin</SelectItem>
                  {actorRole === "owner" && (
                    <SelectItem value="owner">Owner</SelectItem>
                  )}
                </SelectContent>
              </Select>
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={handleClose}
                disabled={isLooking}
              >
                Cancel
              </Button>
              <Button
                type="button"
                onClick={() => void handleLookup()}
                disabled={isLooking || !query.trim()}
              >
                {isLooking && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Look up
              </Button>
            </DialogFooter>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="flex items-center gap-3 rounded-lg border p-3">
              <Avatar size="default">
                {resolvedUser?.avatar_url && (
                  <AvatarImage src={resolvedUser.avatar_url} />
                )}
                <AvatarFallback>
                  {getInitials(
                    resolvedUser?.display_name ?? resolvedUser?.username ?? "?",
                  )}
                </AvatarFallback>
              </Avatar>
              <div className="min-w-0 flex-1">
                <p className="truncate font-medium">
                  {resolvedUser?.display_name ?? resolvedUser?.username}
                </p>
                <p className="text-muted-foreground truncate text-sm">
                  @{resolvedUser?.username}
                </p>
                {resolvedUser?.email && (
                  <p className="text-muted-foreground truncate text-sm">
                    {resolvedUser.email}
                  </p>
                )}
              </div>
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="ghost"
                onClick={handleBack}
                disabled={addMutation.isPending}
              >
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back
              </Button>
              <Button
                type="button"
                onClick={handleAdd}
                disabled={addMutation.isPending}
              >
                {addMutation.isPending && (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                )}
                Add as {selectedRole}
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
