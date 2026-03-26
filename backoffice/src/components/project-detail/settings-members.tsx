"use client";

import { useRef, useState } from "react";
import {
  AlertCircle,
  Check,
  LogOut,
  MoreHorizontal,
  UserMinus,
  UserPlus,
} from "lucide-react";
import { toast } from "sonner";
import { Avatar, AvatarFallback, AvatarImage } from "~/components/ui/avatar";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "~/components/ui/dialog";
import { Skeleton } from "~/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "~/components/ui/table";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "~/components/ui/tooltip";
import { api } from "~/trpc/react";
import { formatRelativeTime, getInitials } from "~/lib/format";
import type { ProjectMember } from "~/server/api/routers/project-members";
import { AddMemberDialog } from "./add-member-dialog";
import { RemoveMemberDialog } from "./remove-member-dialog";
import { LeaveProjectDialog } from "./leave-project-dialog";

// ── Helpers ─────────────────────────────────────────────────────────────────

const ROLE_RANK: Record<string, number> = { owner: 3, admin: 2, member: 1 };

function roleBadge(role: string) {
  switch (role) {
    case "owner":
      return (
        <Badge className="bg-purple-100 text-purple-800 hover:bg-purple-100 dark:bg-purple-900/40 dark:text-purple-300">
          Owner
        </Badge>
      );
    case "admin":
      return (
        <Badge className="bg-amber-100 text-amber-800 hover:bg-amber-100 dark:bg-amber-900/40 dark:text-amber-300">
          Admin
        </Badge>
      );
    default:
      return <Badge variant="secondary">Member</Badge>;
  }
}

// ── Component ───────────────────────────────────────────────────────────────

interface MembersSectionProps {
  projectId: string;
  role: string;
}

export function MembersSection({ projectId, role }: MembersSectionProps) {
  const utils = api.useUtils();
  const meQuery = api.auth.me.useQuery();
  const membersQuery = api.projectMembers.list.useQuery({ projectId });

  const isReadOnly = role === "member";

  // Dialog state
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [memberToRemove, setMemberToRemove] = useState<ProjectMember | null>(
    null,
  );
  const [promoteConfirm, setPromoteConfirm] = useState<{
    member: ProjectMember;
    newRole: string;
  } | null>(null);
  const [leaveDialogOpen, setLeaveDialogOpen] = useState(false);

  // Tracks the username for the in-flight role mutation (avoids stale closure over members)
  const roleTargetUsername = useRef("User");

  // Mutations
  const updateRoleMutation = api.projectMembers.updateRole.useMutation({
    onSuccess: (_data, variables) => {
      toast.success(
        `${roleTargetUsername.current} is now ${variables.role}`,
      );
      void utils.projectMembers.list.invalidate({ projectId });
    },
    onError: (err) => {
      toast.error(err.message ?? "Failed to update role");
    },
  });

  const removeMutation = api.projectMembers.remove.useMutation({
    onSuccess: () => {
      toast.success(
        `${memberToRemove?.username ?? "User"} removed from project`,
      );
      setMemberToRemove(null);
      void utils.projectMembers.list.invalidate({ projectId });
    },
    onError: (err) => {
      if (err.message?.includes("last owner")) {
        toast.error("Cannot remove the last project owner");
      } else if (err.data?.code === "FORBIDDEN") {
        toast.error("Cannot remove a project owner");
      } else {
        toast.error("Failed to remove member. Please try again.");
      }
    },
  });

  const currentUserId = meQuery.data?.user?.id;
  const actorRank = ROLE_RANK[role] ?? 0;
  const members = membersQuery.data?.items ?? [];

  function handleRoleChange(member: ProjectMember, newRole: string) {
    if (newRole === member.role) return;
    if (updateRoleMutation.isPending) return;
    // Owner promotion requires confirmation
    if (newRole === "owner") {
      setPromoteConfirm({ member, newRole });
      return;
    }
    roleTargetUsername.current = member.username;
    updateRoleMutation.mutate({
      projectId,
      userId: member.user_id,
      role: newRole as "owner" | "admin" | "member",
    });
  }

  function confirmPromotion() {
    if (!promoteConfirm) return;
    if (updateRoleMutation.isPending) return;
    roleTargetUsername.current = promoteConfirm.member.username;
    updateRoleMutation.mutate({
      projectId,
      userId: promoteConfirm.member.user_id,
      role: promoteConfirm.newRole as "owner" | "admin" | "member",
    });
    setPromoteConfirm(null);
  }

  // Determine which actions are available for a given member row
  function getAvailableActions(member: ProjectMember) {
    // No actions on self
    if (member.user_id === currentUserId) return { canChangeRole: false, canRemove: false, availableRoles: [] as string[] };

    const targetRank = ROLE_RANK[member.role] ?? 0;

    // Actor can only act on targets with lower rank (except owner who can act on other owners)
    if (role === "owner") {
      // Owner can change any non-self member's role and remove them
      const allRoles = ["owner", "admin", "member"];
      return { canChangeRole: true, canRemove: true, availableRoles: allRoles };
    }

    if (role === "admin") {
      // Admin can only modify members (rank 1), not other admins or owners
      if (targetRank >= actorRank) {
        return { canChangeRole: false, canRemove: false, availableRoles: [] as string[] };
      }
      // Admin cannot assign owner
      const adminRoles = ["admin", "member"];
      return { canChangeRole: true, canRemove: true, availableRoles: adminRoles };
    }

    return { canChangeRole: false, canRemove: false, availableRoles: [] as string[] };
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="space-y-1">
            <CardTitle>Members</CardTitle>
            <CardDescription>
              {isReadOnly
                ? "View project members and their roles."
                : "Manage project members and their roles."}
            </CardDescription>
          </div>
          {!isReadOnly && (
            <Button onClick={() => setAddDialogOpen(true)} size="sm">
              <UserPlus className="mr-2 h-4 w-4" />
              Add Member
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent>
        {membersQuery.isLoading ? (
          <div className="space-y-3">
            {[1, 2, 3].map((i) => (
              <div key={i} className="flex items-center gap-3">
                <Skeleton className="h-6 w-6 rounded-full" />
                <div className="flex-1 space-y-1">
                  <Skeleton className="h-4 w-32" />
                  <Skeleton className="h-3 w-24" />
                </div>
                <Skeleton className="h-5 w-16 rounded-full" />
                <Skeleton className="h-4 w-16" />
              </div>
            ))}
          </div>
        ) : membersQuery.isError ? (
          <div className="text-destructive flex items-center gap-2 text-sm">
            <AlertCircle className="size-4" />
            <span>Failed to load members</span>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => void membersQuery.refetch()}
            >
              Retry
            </Button>
          </div>
        ) : members.length === 0 ? (
          <div className="flex items-center justify-center rounded-lg border border-dashed p-8">
            <p className="text-muted-foreground text-sm">No members found</p>
          </div>
        ) : (
          <TooltipProvider>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Member</TableHead>
                  <TableHead>Role</TableHead>
                  <TableHead>Joined</TableHead>
                  <TableHead className="w-10" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {members.map((member) => {
                  const actions = getAvailableActions(member);
                  const hasActions = actions.canChangeRole || actions.canRemove;
                  const isSelf = member.user_id === currentUserId;
                  const displayName =
                    member.display_name ?? member.username;

                  return (
                    <TableRow key={member.id}>
                      {/* Member */}
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <Avatar size="sm">
                            {member.avatar_url && (
                              <AvatarImage src={member.avatar_url} />
                            )}
                            <AvatarFallback>
                              {getInitials(displayName)}
                            </AvatarFallback>
                          </Avatar>
                          <div className="min-w-0">
                            <div className="flex items-center gap-1.5">
                              <span className="truncate text-sm font-medium">
                                {displayName}
                              </span>
                              {isSelf && (
                                <span className="text-muted-foreground text-xs">
                                  (you)
                                </span>
                              )}
                            </div>
                            {member.display_name && (
                              <p className="text-muted-foreground truncate text-xs">
                                @{member.username}
                              </p>
                            )}
                          </div>
                        </div>
                      </TableCell>

                      {/* Role */}
                      <TableCell>{roleBadge(member.role)}</TableCell>

                      {/* Joined */}
                      <TableCell>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span className="text-muted-foreground text-sm">
                              {formatRelativeTime(member.created_at)}
                            </span>
                          </TooltipTrigger>
                          <TooltipContent>
                            {new Date(member.created_at).toLocaleString()}
                          </TooltipContent>
                        </Tooltip>
                      </TableCell>

                      {/* Actions */}
                      <TableCell>
                        {isSelf ? (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-muted-foreground h-8"
                            onClick={() => setLeaveDialogOpen(true)}
                          >
                            <LogOut className="mr-1.5 h-3.5 w-3.5" />
                            Leave
                          </Button>
                        ) : hasActions ? (
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button variant="ghost" size="icon" className="h-8 w-8">
                                <MoreHorizontal className="h-4 w-4" />
                                <span className="sr-only">Actions</span>
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end">
                              {actions.canChangeRole && (
                                <DropdownMenuSub>
                                  <DropdownMenuSubTrigger>
                                    Change role
                                  </DropdownMenuSubTrigger>
                                  <DropdownMenuSubContent>
                                    {actions.availableRoles.map((r) => (
                                      <DropdownMenuItem
                                        key={r}
                                        disabled={r === member.role || updateRoleMutation.isPending}
                                        onClick={() =>
                                          handleRoleChange(member, r)
                                        }
                                      >
                                        <span className="flex items-center gap-2">
                                          {r === member.role && (
                                            <Check className="h-3 w-3" />
                                          )}
                                          <span
                                            className={
                                              r === member.role ? "" : "pl-5"
                                            }
                                          >
                                            {r.charAt(0).toUpperCase() +
                                              r.slice(1)}
                                          </span>
                                        </span>
                                      </DropdownMenuItem>
                                    ))}
                                  </DropdownMenuSubContent>
                                </DropdownMenuSub>
                              )}
                              {actions.canChangeRole && actions.canRemove && (
                                <DropdownMenuSeparator />
                              )}
                              {actions.canRemove && (
                                <DropdownMenuItem
                                  className="text-destructive focus:text-destructive"
                                  onClick={() => setMemberToRemove(member)}
                                >
                                  <UserMinus className="mr-2 h-4 w-4" />
                                  Remove from project
                                </DropdownMenuItem>
                              )}
                            </DropdownMenuContent>
                          </DropdownMenu>
                        ) : null}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </TooltipProvider>
        )}
      </CardContent>

      {/* Add Member Dialog */}
      {!isReadOnly && (
        <AddMemberDialog
          open={addDialogOpen}
          onOpenChange={setAddDialogOpen}
          projectId={projectId}
          actorRole={role}
        />
      )}

      {/* Remove Member Dialog */}
      {!isReadOnly && (
        <RemoveMemberDialog
          member={memberToRemove}
          onClose={() => setMemberToRemove(null)}
          onConfirm={(m) =>
            removeMutation.mutate({ projectId, userId: m.user_id })
          }
          isPending={removeMutation.isPending}
        />
      )}

      {/* Owner Promotion Confirmation Dialog */}
      {!isReadOnly && (
        <Dialog
          open={promoteConfirm !== null}
          onOpenChange={(open) => {
            if (!open) setPromoteConfirm(null);
          }}
        >
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>Promote to Owner</DialogTitle>
              <DialogDescription>
                Are you sure you want to make{" "}
                <span className="font-semibold text-foreground">
                  {promoteConfirm?.member.username}
                </span>{" "}
                an owner? Owners have full control over the project, including
                managing all members and deleting the project.
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => setPromoteConfirm(null)}
                disabled={updateRoleMutation.isPending}
              >
                Cancel
              </Button>
              <Button
                onClick={confirmPromotion}
                disabled={updateRoleMutation.isPending}
              >
                {updateRoleMutation.isPending ? "Promoting..." : "Promote"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}

      {/* Leave Project Dialog */}
      <LeaveProjectDialog
        open={leaveDialogOpen}
        onOpenChange={setLeaveDialogOpen}
        projectId={projectId}
      />
    </Card>
  );
}
