"use client";

import { useEffect, useState } from "react";
import { AlertCircle, CheckCircle, Loader2 } from "lucide-react";
import { api } from "~/trpc/react";
import { Avatar, AvatarFallback, AvatarImage } from "~/components/ui/avatar";
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
import { Alert, AlertDescription } from "~/components/ui/alert";
import { Skeleton } from "~/components/ui/skeleton";
import { getInitials } from "~/lib/format";

export function ProfileForm() {
  const { data, isLoading } = api.auth.me.useQuery();
  const utils = api.useUtils();

  const user = data?.user;

  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [avatarUrl, setAvatarUrl] = useState("");
  const [formTouched, setFormTouched] = useState(false);
  const [successMsg, setSuccessMsg] = useState("");
  const [errorMsg, setErrorMsg] = useState("");

  // Sync form state when user data loads, but not while the user is editing
  /* eslint-disable react-hooks/set-state-in-effect -- intentional sync from server data */
  useEffect(() => {
    if (user && !formTouched) {
      setEmail(user.email ?? "");
      setDisplayName(user.display_name ?? "");
      setAvatarUrl(user.avatar_url ?? "");
    }
  }, [user, formTouched]);
  /* eslint-enable react-hooks/set-state-in-effect */

  const mutation = api.users.updateMe.useMutation({
    onSuccess: async () => {
      setErrorMsg("");
      setSuccessMsg("Profile updated successfully.");
      setFormTouched(false);
      await utils.auth.me.invalidate();
    },
    onError: (err) => {
      setSuccessMsg("");
      setErrorMsg(err.message ?? "Failed to update profile.");
    },
  });

  const isDirty =
    user !== undefined &&
    user !== null &&
    (email !== (user.email ?? "") ||
      displayName !== (user.display_name ?? "") ||
      avatarUrl !== (user.avatar_url ?? ""));

  function handleCancel() {
    if (!user) return;
    setEmail(user.email ?? "");
    setDisplayName(user.display_name ?? "");
    setAvatarUrl(user.avatar_url ?? "");
    setFormTouched(false);
    setSuccessMsg("");
    setErrorMsg("");
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSuccessMsg("");
    setErrorMsg("");

    const body: { email?: string; display_name?: string; avatar_url?: string } =
      {};
    if (email !== (user?.email ?? "")) {
      const trimmed = email.trim();
      if (!trimmed || !/^[^\s@]+@[^\s@]+$/.test(trimmed)) {
        setErrorMsg("Please enter a valid email address.");
        return;
      }
      body.email = trimmed;
    }
    if (displayName !== (user?.display_name ?? "")) {
      body.display_name = displayName;
    }
    if (avatarUrl !== (user?.avatar_url ?? "")) {
      body.avatar_url = avatarUrl;
    }

    mutation.mutate(body);
  }

  if (isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <h2 className="text-lg font-semibold">Profile</h2>
        <Card>
          <CardHeader>
            <div className="flex items-center gap-4">
              <Skeleton className="size-16 rounded-full" />
              <div className="space-y-2">
                <Skeleton className="h-5 w-32" />
                <Skeleton className="h-4 w-24" />
              </div>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            <Skeleton className="h-9 w-full" />
            <Skeleton className="h-9 w-full" />
          </CardContent>
        </Card>
      </div>
    );
  }

  if (!user) {
    return (
      <div className="flex flex-col gap-4">
        <h2 className="text-lg font-semibold">Profile</h2>
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>Failed to load profile.</AlertDescription>
        </Alert>
      </div>
    );
  }

  const nameForAvatar = displayName ?? user.display_name ?? user.username;
  const avatarSrc = avatarUrl || undefined;

  return (
    <div className="flex flex-col gap-4">
      <h2 className="text-lg font-semibold">Profile</h2>

      <Card>
        <CardHeader>
          <div className="flex items-center gap-4">
            <Avatar className="size-16">
              <AvatarImage src={avatarSrc} alt={nameForAvatar} />
              <AvatarFallback className="text-lg">
                {getInitials(nameForAvatar)}
              </AvatarFallback>
            </Avatar>
            <div>
              <CardTitle>{user.display_name ?? user.username}</CardTitle>
              <CardDescription>@{user.username}</CardDescription>
              <p className="text-muted-foreground mt-1 text-xs">
                Member since{" "}
                {new Date(user.created_at).toLocaleDateString(undefined, {
                  year: "numeric",
                  month: "long",
                  day: "numeric",
                })}
              </p>
            </div>
          </div>
        </CardHeader>

        <CardContent>
          <form onSubmit={handleSubmit} noValidate className="space-y-4">
            {successMsg && (
              <Alert>
                <CheckCircle className="h-4 w-4" />
                <AlertDescription>{successMsg}</AlertDescription>
              </Alert>
            )}
            {errorMsg && (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>{errorMsg}</AlertDescription>
              </Alert>
            )}

            <div className="space-y-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                type="email"
                placeholder="you@example.com"
                value={email}
                onChange={(e) => {
                  setEmail(e.target.value);
                  setFormTouched(true);
                }}
                disabled={mutation.isPending}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="display_name">Display Name</Label>
              <Input
                id="display_name"
                placeholder="Your display name"
                value={displayName}
                onChange={(e) => {
                  setDisplayName(e.target.value);
                  setFormTouched(true);
                }}
                maxLength={100}
                disabled={mutation.isPending}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="avatar_url">Avatar URL</Label>
              <Input
                id="avatar_url"
                type="url"
                placeholder="https://example.com/avatar.png"
                value={avatarUrl}
                onChange={(e) => {
                  setAvatarUrl(e.target.value);
                  setFormTouched(true);
                }}
                disabled={mutation.isPending}
              />
            </div>

            <div className="flex gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={handleCancel}
                disabled={!isDirty || mutation.isPending}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={!isDirty || mutation.isPending}>
                {mutation.isPending && (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                )}
                Save changes
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
