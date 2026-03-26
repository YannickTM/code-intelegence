"use client";

import { Loader2 } from "lucide-react";
import { Button } from "~/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "~/components/ui/dialog";
import type { APIKeyBase } from "./types";

export interface DeleteKeyDialogProps {
  keyToDelete: APIKeyBase | null;
  onClose: () => void;
  onConfirm: (keyId: string) => void;
  isPending: boolean;
  dialogDescription?: string;
}

export function DeleteKeyDialog({
  keyToDelete,
  onClose,
  onConfirm,
  isPending,
  dialogDescription,
}: DeleteKeyDialogProps) {
  return (
    <Dialog
      open={keyToDelete !== null}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Revoke API Key</DialogTitle>
          <DialogDescription>
            Are you sure you want to revoke{" "}
            <span className="font-semibold">{keyToDelete?.name}</span>?{" "}
            {dialogDescription ??
              "Any applications using this key will immediately lose access."}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={onClose}
            disabled={isPending}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={() => keyToDelete && onConfirm(keyToDelete.id)}
            disabled={isPending}
          >
            {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Revoke
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
