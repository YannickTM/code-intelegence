"use client";

import { useState } from "react";
import { Search } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";

export function useSearchDialog() {
  const [open, setOpen] = useState(false);
  return { open, setOpen, openSearch: () => setOpen(true) };
}

export function SearchDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Search</DialogTitle>
          <DialogDescription>
            Search across all your projects and chats.
          </DialogDescription>
        </DialogHeader>
        <div className="relative">
          <Search className="text-muted-foreground absolute top-2.5 left-3 size-4" />
          <Input
            placeholder="Type to search..."
            className="pl-9"
            autoFocus
            disabled
          />
        </div>
        <div className="flex flex-col items-center justify-center py-10 text-center">
          <Search className="text-muted-foreground/40 size-10" />
          <p className="text-muted-foreground mt-3 text-sm">
            Search is coming soon.
          </p>
        </div>
      </DialogContent>
    </Dialog>
  );
}
