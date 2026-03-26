"use client";

import { useState } from "react";
import {
  MoreHorizontal,
  Shield,
  ShieldOff,
  UserCheck,
  UserX,
} from "lucide-react";
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "~/components/ui/tooltip";
import { api } from "~/trpc/react";
import type { PlatformUser } from "~/server/api/routers/platform-users";

interface UserActionsProps {
  user: PlatformUser;
  currentUserId: string;
}

export function UserActions({ user, currentUserId }: UserActionsProps) {
  const utils = api.useUtils();
  const [grantDialogOpen, setGrantDialogOpen] = useState(false);
  const [revokeDialogOpen, setRevokeDialogOpen] = useState(false);
  const [deactivateDialogOpen, setDeactivateDialogOpen] = useState(false);

  const isSelf = user.id === currentUserId;
  const isAdmin = user.platform_roles.includes("platform_admin");
  const isActive = user.is_active;
  const displayName = user.display_name ?? user.username;

  // ── Mutations ───────────────────────────────────────────────────────────

  const grantRoleMutation = api.platformUsers.grantRole.useMutation({
    onSuccess: () => {
      toast.success(`Platform Admin role granted to ${displayName}`);
      setGrantDialogOpen(false);
      void utils.platformUsers.list.invalidate();
    },
    onError: (err) => {
      toast.error(err.message ?? "Failed to grant role");
    },
  });

  const revokeRoleMutation = api.platformUsers.revokeRole.useMutation({
    onSuccess: () => {
      toast.success(`Platform Admin role revoked from ${displayName}`);
      setRevokeDialogOpen(false);
      void utils.platformUsers.list.invalidate();
    },
    onError: (err) => {
      if (err.data?.code === "CONFLICT") {
        toast.error("Cannot remove the last platform admin.");
      } else {
        toast.error(err.message ?? "Failed to revoke role");
      }
    },
  });

  const deactivateMutation = api.platformUsers.deactivateUser.useMutation({
    onSuccess: () => {
      toast.success(`${displayName} has been deactivated`);
      setDeactivateDialogOpen(false);
      void utils.platformUsers.list.invalidate();
    },
    onError: (err) => {
      if (err.data?.code === "CONFLICT") {
        toast.error("Cannot deactivate the last active platform admin.");
      } else {
        toast.error(err.message ?? "Failed to deactivate user");
      }
    },
  });

  const activateMutation = api.platformUsers.activateUser.useMutation({
    onSuccess: () => {
      toast.success(`${displayName} has been activated`);
      void utils.platformUsers.list.invalidate();
    },
    onError: (err) => {
      toast.error(err.message ?? "Failed to activate user");
    },
  });

  // ── Dropdown menu ───────────────────────────────────────────────────────

  const showGrant = !isAdmin && isActive;
  const showRevoke = isAdmin;
  const showDeactivate = isActive;
  const showActivate = !isActive;

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            onClick={(e) => e.stopPropagation()}
          >
            <MoreHorizontal className="h-4 w-4" />
            <span className="sr-only">Actions</span>
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {showGrant && (
            <DropdownMenuItem onClick={() => setGrantDialogOpen(true)}>
              <Shield className="mr-2 h-4 w-4" />
              Grant Platform Admin
            </DropdownMenuItem>
          )}
          {showRevoke && (
            isSelf ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <div>
                    <DropdownMenuItem disabled>
                      <ShieldOff className="mr-2 h-4 w-4" />
                      Revoke Platform Admin
                    </DropdownMenuItem>
                  </div>
                </TooltipTrigger>
                <TooltipContent>Cannot revoke your own role</TooltipContent>
              </Tooltip>
            ) : (
              <DropdownMenuItem
                className="text-destructive focus:text-destructive"
                onClick={() => setRevokeDialogOpen(true)}
              >
                <ShieldOff className="mr-2 h-4 w-4" />
                Revoke Platform Admin
              </DropdownMenuItem>
            )
          )}

          {(showGrant || showRevoke) && (showDeactivate || showActivate) && (
            <DropdownMenuSeparator />
          )}

          {showDeactivate && (
            isSelf ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <div>
                    <DropdownMenuItem disabled>
                      <UserX className="mr-2 h-4 w-4" />
                      Deactivate User
                    </DropdownMenuItem>
                  </div>
                </TooltipTrigger>
                <TooltipContent>Cannot deactivate yourself</TooltipContent>
              </Tooltip>
            ) : (
              <DropdownMenuItem
                className="text-destructive focus:text-destructive"
                onClick={() => setDeactivateDialogOpen(true)}
              >
                <UserX className="mr-2 h-4 w-4" />
                Deactivate User
              </DropdownMenuItem>
            )
          )}
          {showActivate && (
            <DropdownMenuItem
              onClick={() => activateMutation.mutate({ userId: user.id })}
              disabled={activateMutation.isPending}
            >
              <UserCheck className="mr-2 h-4 w-4" />
              Activate User
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      {/* Grant Platform Admin Dialog */}
      <Dialog open={grantDialogOpen} onOpenChange={setGrantDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Grant Platform Admin Role</DialogTitle>
            <DialogDescription>
              Grant platform_admin role to{" "}
              <span className="text-foreground font-semibold">
                {displayName}
              </span>{" "}
              (@{user.username})? They will have access to all platform settings
              and user management.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setGrantDialogOpen(false)}
              disabled={grantRoleMutation.isPending}
            >
              Cancel
            </Button>
            <Button
              onClick={() =>
                grantRoleMutation.mutate({
                  user_id: user.id,
                  role: "platform_admin",
                })
              }
              disabled={grantRoleMutation.isPending}
            >
              {grantRoleMutation.isPending ? "Granting..." : "Grant Role"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Revoke Platform Admin Dialog */}
      <Dialog open={revokeDialogOpen} onOpenChange={setRevokeDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Revoke Platform Admin Role</DialogTitle>
            <DialogDescription>
              Revoke platform_admin role from{" "}
              <span className="text-foreground font-semibold">
                {displayName}
              </span>{" "}
              (@{user.username})? They will lose access to platform settings and
              user management.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setRevokeDialogOpen(false)}
              disabled={revokeRoleMutation.isPending}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() =>
                revokeRoleMutation.mutate({
                  userId: user.id,
                  role: "platform_admin",
                })
              }
              disabled={revokeRoleMutation.isPending}
            >
              {revokeRoleMutation.isPending ? "Revoking..." : "Revoke Role"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Deactivate User Dialog */}
      <Dialog
        open={deactivateDialogOpen}
        onOpenChange={setDeactivateDialogOpen}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Deactivate User</DialogTitle>
            <DialogDescription>
              Deactivate{" "}
              <span className="text-foreground font-semibold">
                {displayName}
              </span>{" "}
              (@{user.username})? Their active sessions will be terminated and
              they will not be able to log in.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeactivateDialogOpen(false)}
              disabled={deactivateMutation.isPending}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() =>
                deactivateMutation.mutate({ userId: user.id })
              }
              disabled={deactivateMutation.isPending}
            >
              {deactivateMutation.isPending ? "Deactivating..." : "Deactivate"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
