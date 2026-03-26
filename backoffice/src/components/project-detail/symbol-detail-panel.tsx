"use client";

import { useState } from "react";
import Link from "next/link";
import { Check, ChevronDown, ChevronRight, ExternalLink, X } from "lucide-react";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "~/components/ui/collapsible";
import { TableCell, TableRow } from "~/components/ui/table";

type SymbolFlags = {
  is_exported?: boolean;
  is_default_export?: boolean;
  is_async?: boolean;
  is_generator?: boolean;
  is_static?: boolean;
  is_abstract?: boolean;
  is_readonly?: boolean;
  is_optional?: boolean;
  is_arrow_function?: boolean;
  is_react_component_like?: boolean;
  is_hook_like?: boolean;
};

type SymbolDetail = {
  id: string;
  name: string;
  qualified_name?: string;
  kind: string;
  signature?: string;
  start_line?: number;
  end_line?: number;
  doc_text?: string;
  file_path: string;
  language?: string;
  flags?: SymbolFlags;
  modifiers?: string[];
  return_type?: string;
  parameter_types?: string[];
};

const FLAG_LABELS: Record<string, string> = {
  is_exported: "Exported",
  is_default_export: "Default export",
  is_async: "Async",
  is_generator: "Generator",
  is_static: "Static",
  is_abstract: "Abstract",
  is_readonly: "Readonly",
  is_optional: "Optional",
  is_arrow_function: "Arrow function",
  is_react_component_like: "React component",
  is_hook_like: "React hook",
};

export function SymbolDetailPanel({
  symbol,
  projectId,
  colSpan,
}: {
  symbol: SymbolDetail;
  projectId: string;
  colSpan: number;
}) {
  const [flagsOpen, setFlagsOpen] = useState(false);

  const showQualifiedName =
    symbol.qualified_name && symbol.qualified_name !== symbol.name;

  const hasTypeSignature =
    symbol.return_type ||
    (symbol.parameter_types && symbol.parameter_types.length > 0);

  const fileHref = `/project/${projectId}/file?path=${encodeURIComponent(symbol.file_path)}${symbol.start_line != null ? `&line=${symbol.start_line}` : ""}`;

  return (
    <TableRow className="bg-muted/30 hover:bg-muted/30">
      <TableCell colSpan={colSpan} className="px-6 py-4">
        <div className="flex flex-col gap-3">
          {showQualifiedName && (
            <div>
              <p className="text-muted-foreground text-xs font-medium uppercase tracking-wide">
                Qualified Name
              </p>
              <code className="text-sm font-mono">
                {symbol.qualified_name}
              </code>
            </div>
          )}

          {symbol.modifiers && symbol.modifiers.length > 0 && (
            <div>
              <p className="text-muted-foreground text-xs font-medium uppercase tracking-wide">
                Modifiers
              </p>
              <div className="mt-1 flex flex-wrap gap-1">
                {symbol.modifiers.map((mod) => (
                  <Badge key={mod} variant="secondary" className="text-xs">
                    {mod}
                  </Badge>
                ))}
              </div>
            </div>
          )}

          {hasTypeSignature && (
            <div>
              <p className="text-muted-foreground text-xs font-medium uppercase tracking-wide">
                Type Signature
              </p>
              <div className="bg-muted mt-1 overflow-x-auto rounded-md p-3 font-mono text-sm">
                {symbol.parameter_types &&
                  symbol.parameter_types.length > 0 && (
                    <div>
                      <span className="text-muted-foreground">
                        ({symbol.parameter_types.join(", ")})
                      </span>
                    </div>
                  )}
                {symbol.return_type && (
                  <div>
                    <span className="text-muted-foreground">→ </span>
                    {symbol.return_type}
                  </div>
                )}
              </div>
            </div>
          )}

          {symbol.signature && (
            <div>
              <p className="text-muted-foreground text-xs font-medium uppercase tracking-wide">
                Signature
              </p>
              <pre className="bg-muted mt-1 overflow-x-auto rounded-md p-3 font-mono text-sm">
                {symbol.signature}
              </pre>
            </div>
          )}

          {symbol.doc_text && (
            <div>
              <p className="text-muted-foreground text-xs font-medium uppercase tracking-wide">
                Documentation
              </p>
              <p className="text-muted-foreground mt-1 whitespace-pre-wrap text-sm">
                {symbol.doc_text}
              </p>
            </div>
          )}

          {symbol.flags && (
            <Collapsible open={flagsOpen} onOpenChange={setFlagsOpen}>
              <CollapsibleTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-muted-foreground hover:text-foreground -ml-2 gap-1.5"
                >
                  {flagsOpen ? (
                    <ChevronDown className="size-4" />
                  ) : (
                    <ChevronRight className="size-4" />
                  )}
                  Symbol properties
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="pt-1">
                <div className="grid grid-cols-2 gap-x-6 gap-y-1 text-sm">
                  {Object.entries(FLAG_LABELS).map(([key, label]) => {
                    const value =
                      symbol.flags?.[key as keyof SymbolFlags];
                    return (
                      <div
                        key={key}
                        className="flex items-center gap-2"
                      >
                        {value ? (
                          <Check className="size-3.5 text-green-600 dark:text-green-400" />
                        ) : (
                          <X className="text-muted-foreground/50 size-3.5" />
                        )}
                        <span
                          className={
                            value
                              ? "text-foreground"
                              : "text-muted-foreground"
                          }
                        >
                          {label}
                        </span>
                      </div>
                    );
                  })}
                </div>
              </CollapsibleContent>
            </Collapsible>
          )}

          <div>
            <Button variant="outline" size="sm" asChild>
              <Link href={fileHref}>
                <ExternalLink className="size-3.5" />
                View source
              </Link>
            </Button>
          </div>
        </div>
      </TableCell>
    </TableRow>
  );
}
