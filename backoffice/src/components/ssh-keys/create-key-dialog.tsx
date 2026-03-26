"use client";

import { useState } from "react";
import { Check, Copy, Loader2 } from "lucide-react";
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
import { Input } from "~/components/ui/input";
import { Label } from "~/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "~/components/ui/tabs";
import { Textarea } from "~/components/ui/textarea";
import type { SSHKey } from "./types";

interface CreateKeyDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: { name: string; private_key?: string }) => void;
  createdKey: SSHKey | null;
  isPending: boolean;
  onDone: () => void;
}

export function CreateKeyDialog({
  open,
  onOpenChange,
  onSubmit,
  createdKey,
  isPending,
  onDone,
}: CreateKeyDialogProps) {
  const [mode, setMode] = useState<"generate" | "import">("generate");
  const [name, setName] = useState("");
  const [privateKey, setPrivateKey] = useState("");
  const [copied, setCopied] = useState(false);

  function resetForm() {
    setName("");
    setPrivateKey("");
    setMode("generate");
    setCopied(false);
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      resetForm();
    }
    onOpenChange(nextOpen);
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    if (mode === "import" && !privateKey.trim()) return;
    const values: { name: string; private_key?: string } = {
      name: name.trim(),
    };
    if (mode === "import" && privateKey.trim()) {
      values.private_key = privateKey.trim();
    }
    onSubmit(values);
  }

  function handleDone() {
    resetForm();
    onDone();
  }

  async function handleCopyPublicKey(publicKey: string) {
    try {
      await navigator.clipboard.writeText(publicKey);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error("Failed to copy to clipboard");
    }
  }

  // Post-creation view
  if (createdKey) {
    return (
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Key Created</DialogTitle>
            <DialogDescription>
              Copy the public key to add it as a deploy key in your Git
              provider.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Public Key</Label>
              <div className="flex items-start gap-2">
                <code className="bg-muted flex-1 rounded-md border p-3 font-mono text-sm break-all">
                  {createdKey.public_key}
                </code>
              </div>
            </div>

            <div className="text-muted-foreground space-y-1 text-sm">
              <p>
                <span className="font-medium">Fingerprint:</span>{" "}
                {createdKey.fingerprint}
              </p>
              <p>
                <span className="font-medium">Type:</span>{" "}
                {createdKey.key_type}
              </p>
            </div>
          </div>

          <DialogFooter className="gap-2 sm:gap-0">
            <Button
              type="button"
              variant="outline"
              onClick={() => void handleCopyPublicKey(createdKey.public_key)}
            >
              {copied ? (
                <>
                  <Check className="mr-1 h-4 w-4" />
                  Copied
                </>
              ) : (
                <>
                  <Copy className="mr-1 h-4 w-4" />
                  Copy Public Key
                </>
              )}
            </Button>
            <Button type="button" onClick={handleDone}>
              Done
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    );
  }

  // Form view
  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>
              {mode === "generate" ? "Generate SSH Key" : "Import SSH Key"}
            </DialogTitle>
            <DialogDescription>
              {mode === "generate"
                ? "Generate a new Ed25519 deploy key."
                : "Import an existing private key."}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 py-4">
            <Tabs
              value={mode}
              onValueChange={(v) => setMode(v as "generate" | "import")}
            >
              <TabsList className="w-full">
                <TabsTrigger value="generate" className="flex-1">
                  Generate
                </TabsTrigger>
                <TabsTrigger value="import" className="flex-1">
                  Import
                </TabsTrigger>
              </TabsList>

              <TabsContent value="generate" className="mt-4 space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="gen-name">Name</Label>
                  <Input
                    id="gen-name"
                    placeholder="e.g. my-project-deploy-key"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    maxLength={100}
                    required
                  />
                </div>
              </TabsContent>

              <TabsContent value="import" className="mt-4 space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="imp-name">Name</Label>
                  <Input
                    id="imp-name"
                    placeholder="e.g. my-project-deploy-key"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    maxLength={100}
                    required
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="imp-key">Private Key</Label>
                  <Textarea
                    id="imp-key"
                    placeholder="Paste PEM-encoded private key..."
                    value={privateKey}
                    onChange={(e) => setPrivateKey(e.target.value)}
                    className="font-mono text-sm"
                    rows={8}
                    required
                  />
                  <p className="text-muted-foreground text-xs">
                    Unencrypted PEM format only. Passphrase-protected keys are
                    not supported.
                  </p>
                </div>
              </TabsContent>
            </Tabs>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => handleOpenChange(false)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={!name.trim() || (mode === "import" && !privateKey.trim()) || isPending}>
              {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {mode === "generate" ? "Generate" : "Import"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
