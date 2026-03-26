"use client";

import { Avatar, AvatarFallback, AvatarImage } from "~/components/ui/avatar";
import { Badge } from "~/components/ui/badge";
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
import { formatRelativeTime, getInitials } from "~/lib/format";
import type { PlatformUser } from "~/server/api/routers/platform-users";
import { UserActions } from "./user-actions";

interface UserTableProps {
  items: PlatformUser[];
  currentUserId: string;
}

export function UserTable({ items, currentUserId }: UserTableProps) {
  return (
    <TooltipProvider>
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>User</TableHead>
              <TableHead>Email</TableHead>
              <TableHead>Role</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Joined</TableHead>
              <TableHead className="w-12">
                <span className="sr-only">Actions</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={6}
                  className="text-muted-foreground py-8 text-center"
                >
                  No users match your filters
                </TableCell>
              </TableRow>
            ) : (
              items.map((user) => {
                const isSelf = user.id === currentUserId;
                const displayName = user.display_name ?? user.username;
                const isAdmin =
                  user.platform_roles.includes("platform_admin");

                return (
                  <TableRow key={user.id}>
                    {/* User */}
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Avatar size="sm">
                          {user.avatar_url && (
                            <AvatarImage src={user.avatar_url} />
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
                          <p className="text-muted-foreground truncate text-xs">
                            @{user.username}
                          </p>
                        </div>
                      </div>
                    </TableCell>

                    {/* Email */}
                    <TableCell>
                      <span className="text-muted-foreground text-sm">
                        {user.email}
                      </span>
                    </TableCell>

                    {/* Role */}
                    <TableCell>
                      {isAdmin ? (
                        <Badge className="bg-amber-100 text-amber-800 hover:bg-amber-100 dark:bg-amber-900/40 dark:text-amber-300">
                          Platform Admin
                        </Badge>
                      ) : (
                        <Badge variant="secondary">User</Badge>
                      )}
                    </TableCell>

                    {/* Status */}
                    <TableCell>
                      {user.is_active ? (
                        <Badge
                          variant="outline"
                          className="border-success/40 text-success"
                        >
                          Active
                        </Badge>
                      ) : (
                        <Badge variant="secondary">Inactive</Badge>
                      )}
                    </TableCell>

                    {/* Joined */}
                    <TableCell>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span className="text-muted-foreground text-sm">
                            {formatRelativeTime(user.created_at)}
                          </span>
                        </TooltipTrigger>
                        <TooltipContent>
                          {new Date(user.created_at).toLocaleString()}
                        </TooltipContent>
                      </Tooltip>
                    </TableCell>

                    {/* Actions */}
                    <TableCell>
                      <UserActions
                        user={user}
                        currentUserId={currentUserId}
                      />
                    </TableCell>
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>
      </div>
    </TooltipProvider>
  );
}
