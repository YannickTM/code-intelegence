"use client";

import { memo } from "react";
import { Handle, Position, type NodeProps } from "@xyflow/react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "~/components/ui/tooltip";

export type DependencyNodeData = {
  label: string;
  fullPath: string;
  isExternal: boolean;
  isRoot: boolean;
  depth: number;
};

function DependencyGraphNodeComponent({
  data,
}: NodeProps & { data: DependencyNodeData }) {
  const opacity = Math.max(0.5, 1 - data.depth * 0.15);

  return (
    <>
      <Handle type="target" position={Position.Top} className="!bg-border" />
      <Tooltip>
        <TooltipTrigger asChild>
          <div
            className={`rounded-md border px-3 py-1.5 text-xs shadow-sm transition-colors ${
              data.isRoot
                ? "border-primary bg-primary/5 font-semibold"
                : data.isExternal
                  ? "border-dashed border-muted-foreground/40 bg-muted text-muted-foreground"
                  : "border-border bg-background"
            }`}
            style={{ opacity }}
          >
            {data.label}
          </div>
        </TooltipTrigger>
        <TooltipContent side="bottom">
          <code className="text-xs">{data.fullPath}</code>
        </TooltipContent>
      </Tooltip>
      <Handle type="source" position={Position.Bottom} className="!bg-border" />
    </>
  );
}

export const DependencyGraphNode = memo(DependencyGraphNodeComponent);
