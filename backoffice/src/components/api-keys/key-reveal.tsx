"use client";

import { useState } from "react";
import { AlertTriangle, Check, Copy } from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription } from "~/components/ui/alert";
import { Button } from "~/components/ui/button";
import { RoleBadge } from "./role-badge";

export interface KeyRevealProps {
  plaintextKey: string;
  name: string;
  role: "read" | "write";
  expiresAt: string | null;
  onDone: () => void;
}

export function KeyReveal({
  plaintextKey,
  name,
  role,
  expiresAt,
  onDone,
}: KeyRevealProps) {
  const [copied, setCopied] = useState(false);

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(plaintextKey);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error("Failed to copy to clipboard");
    }
  }

  return (
    <div className="space-y-4">
      <Alert className="border-amber-500/40 bg-amber-50 text-amber-800 dark:bg-amber-950/30 dark:text-amber-300">
        <AlertTriangle className="h-4 w-4" />
        <AlertDescription>
          Copy your key now. You won&apos;t be able to see it again.
        </AlertDescription>
      </Alert>

      <div className="flex items-center gap-2">
        <code className="bg-muted flex-1 rounded-md border p-3 font-mono text-sm break-all">
          {plaintextKey}
        </code>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={handleCopy}
          className="shrink-0"
        >
          {copied ? (
            <>
              <Check className="mr-1 h-4 w-4" />
              Copied
            </>
          ) : (
            <>
              <Copy className="mr-1 h-4 w-4" />
              Copy key
            </>
          )}
        </Button>
      </div>

      <div className="text-muted-foreground space-y-1 text-sm">
        <p>
          <span className="font-medium">Name:</span> {name}
        </p>
        <p>
          <span className="font-medium">Role:</span>{" "}
          <RoleBadge role={role} />
        </p>
        <p>
          <span className="font-medium">Expires:</span>{" "}
          {expiresAt
            ? new Date(expiresAt).toLocaleDateString(undefined, {
                year: "numeric",
                month: "long",
                day: "numeric",
              })
            : "Never"}
        </p>
      </div>

      <div className="flex justify-end">
        <Button type="button" onClick={onDone}>
          Done
        </Button>
      </div>
    </div>
  );
}
