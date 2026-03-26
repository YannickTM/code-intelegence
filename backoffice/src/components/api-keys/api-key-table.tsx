"use client";

import { Trash2 } from "lucide-react";
import { Button } from "~/components/ui/button";
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
import { formatRelativeTime, formatExpiresAt } from "~/lib/format";
import { RoleBadge } from "./role-badge";
import type { APIKeyBase } from "./types";

export interface ApiKeyTableProps {
  keys: APIKeyBase[];
  onDelete?: (key: APIKeyBase) => void;
}

export function ApiKeyTable({ keys, onDelete }: ApiKeyTableProps) {
  return (
    <TooltipProvider>
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Key</TableHead>
              <TableHead>Role</TableHead>
              <TableHead>Expires</TableHead>
              <TableHead>Last Used</TableHead>
              <TableHead>Created</TableHead>
              {onDelete && <TableHead className="w-[60px]" />}
            </TableRow>
          </TableHeader>
          <TableBody>
            {keys.map((key) => (
              <TableRow key={key.id}>
                <TableCell className="font-medium">{key.name}</TableCell>
                <TableCell>
                  <code className="text-muted-foreground font-mono text-sm">
                    {key.key_prefix}
                  </code>
                </TableCell>
                <TableCell>
                  <RoleBadge role={key.role} />
                </TableCell>
                <TableCell>
                  {key.expires_at ? (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span className="text-muted-foreground text-sm">
                          {formatExpiresAt(key.expires_at)}
                        </span>
                      </TooltipTrigger>
                      <TooltipContent>
                        {new Date(key.expires_at).toLocaleString()}
                      </TooltipContent>
                    </Tooltip>
                  ) : (
                    <span className="text-muted-foreground text-sm">Never</span>
                  )}
                </TableCell>
                <TableCell>
                  {key.last_used_at ? (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span className="text-muted-foreground text-sm">
                          {formatRelativeTime(key.last_used_at)}
                        </span>
                      </TooltipTrigger>
                      <TooltipContent>
                        {new Date(key.last_used_at).toLocaleString()}
                      </TooltipContent>
                    </Tooltip>
                  ) : (
                    <span className="text-muted-foreground text-sm">Never</span>
                  )}
                </TableCell>
                <TableCell>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className="text-muted-foreground text-sm">
                        {formatRelativeTime(key.created_at)}
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>
                      {new Date(key.created_at).toLocaleString()}
                    </TooltipContent>
                  </Tooltip>
                </TableCell>
                {onDelete && (
                <TableCell>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => onDelete(key)}
                    aria-label={`Revoke ${key.name}`}
                  >
                    <Trash2 className="text-destructive h-4 w-4" />
                  </Button>
                </TableCell>
                )}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </TooltipProvider>
  );
}
