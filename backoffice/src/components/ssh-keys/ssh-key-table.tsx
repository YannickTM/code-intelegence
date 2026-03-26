"use client";

import { Copy, Eye, MoreHorizontal, XCircle } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
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
import { formatRelativeTime } from "~/lib/format";
import type { SSHKey } from "./types";

interface SSHKeyTableProps {
  keys: SSHKey[];
  onViewDetails: (key: SSHKey) => void;
  onRetire: (key: SSHKey) => void;
}

function StatusBadge({ isActive }: { isActive: boolean }) {
  if (isActive) {
    return (
      <Badge
        variant="outline"
        className="border-success/40 text-success"
      >
        Active
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="text-muted-foreground">
      Retired
    </Badge>
  );
}

function truncateFingerprint(fingerprint: string): string {
  if (fingerprint.length <= 16) return fingerprint;
  return fingerprint.slice(0, 16) + "...";
}

export function SSHKeyTable({ keys, onViewDetails, onRetire }: SSHKeyTableProps) {
  const sorted = [...keys].sort((a, b) => {
    if (a.is_active !== b.is_active) return a.is_active ? -1 : 1;
    return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
  });

  async function handleCopyPublicKey(publicKey: string) {
    try {
      await navigator.clipboard.writeText(publicKey);
      toast.success("Public key copied to clipboard");
    } catch {
      toast.error("Failed to copy to clipboard");
    }
  }

  return (
    <TooltipProvider>
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Fingerprint</TableHead>
              <TableHead>Type</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Created</TableHead>
              <TableHead className="w-[60px]" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {sorted.map((key) => (
              <TableRow
                key={key.id}
                className={`cursor-pointer ${!key.is_active ? "opacity-50" : ""}`}
                onClick={() => onViewDetails(key)}
              >
                <TableCell className="font-medium">{key.name}</TableCell>
                <TableCell>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <code className="text-muted-foreground font-mono text-sm">
                        {truncateFingerprint(key.fingerprint)}
                      </code>
                    </TooltipTrigger>
                    <TooltipContent>{key.fingerprint}</TooltipContent>
                  </Tooltip>
                </TableCell>
                <TableCell className="text-sm">{key.key_type}</TableCell>
                <TableCell>
                  <StatusBadge isActive={key.is_active} />
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
                <TableCell>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild onClick={(e) => e.stopPropagation()}>
                      <Button variant="ghost" size="icon-sm">
                        <MoreHorizontal className="h-4 w-4" />
                        <span className="sr-only">Actions</span>
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem
                        onClick={(e) => {
                          e.stopPropagation();
                          onViewDetails(key);
                        }}
                      >
                        <Eye className="mr-2 h-4 w-4" />
                        View details
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={(e) => {
                          e.stopPropagation();
                          void handleCopyPublicKey(key.public_key);
                        }}
                      >
                        <Copy className="mr-2 h-4 w-4" />
                        Copy public key
                      </DropdownMenuItem>
                      {key.is_active && (
                        <DropdownMenuItem
                          onClick={(e) => {
                            e.stopPropagation();
                            onRetire(key);
                          }}
                          className="text-destructive focus:text-destructive"
                        >
                          <XCircle className="mr-2 h-4 w-4" />
                          Retire
                        </DropdownMenuItem>
                      )}
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </TooltipProvider>
  );
}
