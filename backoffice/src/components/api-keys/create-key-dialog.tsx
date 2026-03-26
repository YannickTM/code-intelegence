"use client";

import { useEffect, useState } from "react";
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
import { Input } from "~/components/ui/input";
import { Label } from "~/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select";
import { KeyReveal } from "./key-reveal";
import type { CreateAPIKeyResponseBase } from "./types";

export interface CreateKeyDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: {
    name: string;
    role: "read" | "write";
    expires_at?: string;
  }) => void;
  createdKey: CreateAPIKeyResponseBase | null;
  isPending: boolean;
  dialogTitle?: string;
  onDone?: () => void;
}

const EXPIRY_PRESETS = [
  { label: "Never", value: "never" },
  { label: "30 days", value: "30d" },
  { label: "90 days", value: "90d" },
  { label: "1 year", value: "1y" },
  { label: "Custom...", value: "custom" },
] as const;

function computeExpiresAt(preset: string, customDate: string): string | undefined {
  if (preset === "never") return undefined;

  if (preset === "custom") {
    if (!customDate) return undefined;
    // End of the selected day in user's local timezone
    const parts = customDate.split("-").map(Number);
    const year = parts[0]!;
    const month = parts[1]!;
    const day = parts[2]!;
    if (!Number.isFinite(year) || !Number.isFinite(month) || !Number.isFinite(day)) {
      return undefined;
    }
    if (month < 1 || month > 12 || day < 1 || day > 31) {
      return undefined;
    }
    const d = new Date(year, month - 1, day, 23, 59, 59);
    // Round-trip check: catches impossible dates like Feb 31
    if (d.getFullYear() !== year || d.getMonth() !== month - 1 || d.getDate() !== day) {
      return undefined;
    }
    return d.toISOString();
  }

  const daysMap: Record<string, number> = {
    "30d": 30,
    "90d": 90,
    "1y": 365,
  };
  const days = daysMap[preset];
  if (!days) return undefined;

  return new Date(Date.now() + days * 86_400_000).toISOString();
}

function getTomorrowDate(): string {
  const d = new Date();
  d.setDate(d.getDate() + 1);
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

export function CreateKeyDialog({
  open,
  onOpenChange,
  onSubmit,
  createdKey,
  isPending,
  dialogTitle = "Create Personal API Key",
  onDone,
}: CreateKeyDialogProps) {
  const [name, setName] = useState("");
  const [role, setRole] = useState<"read" | "write">("read");
  const [expiryPreset, setExpiryPreset] = useState("never");
  const [customDate, setCustomDate] = useState("");
  const [nameError, setNameError] = useState("");
  const [dateError, setDateError] = useState("");

  // Reset form when dialog opens
  /* eslint-disable react-hooks/set-state-in-effect -- intentional reset on open */
  useEffect(() => {
    if (open) {
      setName("");
      setRole("read");
      setExpiryPreset("never");
      setCustomDate("");
      setNameError("");
      setDateError("");
    }
  }, [open]);
  /* eslint-enable react-hooks/set-state-in-effect */

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();

    const trimmedName = name.trim();
    if (!trimmedName) {
      setNameError("Name is required");
      return;
    }
    if (trimmedName.length > 100) {
      setNameError("Name must be 100 characters or less");
      return;
    }
    setNameError("");

    if (expiryPreset === "custom" && !customDate) {
      setDateError("Expiration date is required");
      return;
    }
    setDateError("");

    const expires_at = computeExpiresAt(expiryPreset, customDate);
    onSubmit({ name: trimmedName, role, expires_at });
  }

  // Key reveal view (after creation)
  if (createdKey) {
    return (
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>API Key Created</DialogTitle>
          </DialogHeader>
          <KeyReveal
            plaintextKey={createdKey.plaintext_key}
            name={createdKey.name}
            role={createdKey.role}
            expiresAt={createdKey.expires_at}
            onDone={() => onDone ? onDone() : onOpenChange(false)}
          />
        </DialogContent>
      </Dialog>
    );
  }

  // Form view
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{dialogTitle}</DialogTitle>
          <DialogDescription>
            Create a new key for programmatic API access.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="key-name">Name</Label>
            <Input
              id="key-name"
              placeholder="e.g. ci-pipeline-key"
              value={name}
              onChange={(e) => {
                setName(e.target.value);
                setNameError("");
              }}
              maxLength={100}
              disabled={isPending}
              aria-invalid={!!nameError}
            />
            {nameError && (
              <p className="text-destructive text-sm">{nameError}</p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="key-role">Role</Label>
            <Select
              value={role}
              onValueChange={(v) => setRole(v as "read" | "write")}
              disabled={isPending}
            >
              <SelectTrigger id="key-role" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="read">Read</SelectItem>
                <SelectItem value="write">Write</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label htmlFor="key-expiry">Expires</Label>
            <Select
              value={expiryPreset}
              onValueChange={(v) => {
                setExpiryPreset(v);
                setDateError("");
              }}
              disabled={isPending}
            >
              <SelectTrigger id="key-expiry" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {EXPIRY_PRESETS.map((p) => (
                  <SelectItem key={p.value} value={p.value}>
                    {p.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {expiryPreset === "custom" && (
              <Input
                type="date"
                value={customDate}
                onChange={(e) => {
                  setCustomDate(e.target.value);
                  setDateError("");
                }}
                min={getTomorrowDate()}
                disabled={isPending}
                aria-invalid={!!dateError}
              />
            )}
            {dateError && (
              <p className="text-destructive text-sm">{dateError}</p>
            )}
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={isPending}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={isPending}>
              {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Create Key
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
