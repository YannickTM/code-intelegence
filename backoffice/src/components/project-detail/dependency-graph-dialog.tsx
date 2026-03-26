"use client";

import { useState, useMemo, useCallback } from "react";
import { useRouter } from "next/navigation";
import { AlertCircle, AlertTriangle, RefreshCw } from "lucide-react";
import {
  ReactFlow,
  Controls,
  Background,
  type Node,
  type Edge,
  type NodeMouseHandler,
} from "@xyflow/react";
import dagre from "@dagrejs/dagre";
import "@xyflow/react/dist/style.css";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "~/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select";
import { Button } from "~/components/ui/button";
import { Skeleton } from "~/components/ui/skeleton";
import { Alert, AlertDescription } from "~/components/ui/alert";
import {
  TooltipProvider,
} from "~/components/ui/tooltip";
import { api } from "~/trpc/react";
import {
  DependencyGraphNode,
  type DependencyNodeData,
} from "./dependency-graph-node";
import type {
  DependencyGraphResponse,
} from "~/server/api/routers/project-files";

const NODE_WIDTH = 180;
const NODE_HEIGHT = 40;

const nodeTypes = { dependency: DependencyGraphNode };

function buildLayout(graphData: DependencyGraphResponse) {
  const g = new dagre.graphlib.Graph();
  g.setGraph({
    rankdir: "TB",
    nodesep: 50,
    ranksep: 80,
    edgesep: 20,
  });
  g.setDefaultEdgeLabel(() => ({}));

  for (const node of graphData.nodes) {
    g.setNode(node.file_path, { width: NODE_WIDTH, height: NODE_HEIGHT });
  }

  for (const edge of graphData.edges) {
    g.setEdge(edge.source, edge.target);
  }

  dagre.layout(g);

  const nodes: Node<DependencyNodeData>[] = graphData.nodes.map((node) => {
    const pos = g.node(node.file_path);
    const isExternal = node.is_external;
    const displayName = isExternal
      ? node.file_path.replace(/^ext:/, "")
      : node.file_path.split("/").pop() ?? node.file_path;

    return {
      id: node.file_path,
      type: "dependency",
      position: {
        x: (pos?.x ?? 0) - NODE_WIDTH / 2,
        y: (pos?.y ?? 0) - NODE_HEIGHT / 2,
      },
      data: {
        label: displayName,
        fullPath: node.file_path,
        isExternal,
        isRoot: node.file_path === graphData.root,
        depth: node.depth,
      },
    };
  });

  const edges: Edge[] = graphData.edges.map((edge, i) => ({
    id: `e-${i}`,
    source: edge.source,
    target: edge.target,
    label: edge.import_name,
    animated: true,
    style: { strokeWidth: 1.5, stroke: "var(--color-muted-foreground)" },
    labelStyle: { fontSize: 10, fill: "var(--color-muted-foreground)" },
    labelBgStyle: { fill: "var(--color-background)", opacity: 0.8 },
  }));

  return { nodes, edges };
}

export function DependencyGraphDialog({
  projectId,
  filePath,
  open,
  onOpenChange,
}: {
  projectId: string;
  filePath: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const router = useRouter();
  const [graphDepth, setGraphDepth] = useState(2);

  const graphQuery = api.projectFiles.dependencyGraph.useQuery(
    { projectId, root: filePath, depth: graphDepth },
    { enabled: open, retry: false },
  );

  const layout = useMemo(() => {
    if (!graphQuery.data) return null;
    return buildLayout(graphQuery.data);
  }, [graphQuery.data]);

  const onNodeClick: NodeMouseHandler = useCallback(
    (_event, node) => {
      const data = node.data as DependencyNodeData;
      if (data.isExternal) return;
      onOpenChange(false);
      router.push(
        `/project/${projectId}/file?path=${encodeURIComponent(data.fullPath)}`,
      );
    },
    [projectId, router, onOpenChange],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-7xl">
        <DialogHeader>
          <DialogTitle>Dependency Graph</DialogTitle>
          <DialogDescription asChild>
            <div className="flex flex-wrap items-center gap-3">
              <code className="text-xs">{filePath}</code>
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground text-xs">Depth:</span>
                <Select
                  value={String(graphDepth)}
                  onValueChange={(v) => setGraphDepth(Number(v))}
                >
                  <SelectTrigger className="h-7 w-16">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {[1, 2, 3, 4, 5].map((d) => (
                      <SelectItem key={d} value={String(d)}>
                        {d}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              {graphQuery.data?.truncated && (
                <span className="text-muted-foreground flex items-center gap-1 text-xs">
                  <AlertTriangle className="size-3" />
                  Graph capped at 200 nodes
                </span>
              )}
            </div>
          </DialogDescription>
        </DialogHeader>

        <div className="h-[60vh] rounded-md border">
          {/* Loading */}
          {graphQuery.isLoading && (
            <div className="flex h-[60vh] flex-col items-center justify-center gap-4">
              <Skeleton className="h-48 w-48 rounded-full" />
              <Skeleton className="h-4 w-32" />
            </div>
          )}

          {/* Error */}
          {graphQuery.isError && (
            <div className="flex h-[60vh] flex-col items-center justify-center gap-3 px-4">
              <Alert variant="destructive" className="max-w-sm">
                <AlertCircle className="size-4" />
                <AlertDescription>
                  Failed to load dependency graph.
                </AlertDescription>
              </Alert>
              <Button
                variant="outline"
                size="sm"
                onClick={() => graphQuery.refetch()}
              >
                <RefreshCw className="size-4" />
                Retry
              </Button>
            </div>
          )}

          {/* Graph */}
          {layout && (
            <TooltipProvider>
              <div className="dark:[&_.react-flow__controls-button]:bg-background dark:[&_.react-flow__controls-button]:border-border dark:[&_.react-flow__controls-button]:fill-foreground dark:[&_.react-flow__controls-button:hover]:bg-muted dark:[&_.react-flow__edge-path]:stroke-muted-foreground dark:[&_.react-flow__background]:opacity-20 h-full">
                <ReactFlow
                  nodes={layout.nodes}
                  edges={layout.edges}
                  nodeTypes={nodeTypes}
                  onNodeClick={onNodeClick}
                  fitView
                  fitViewOptions={{ padding: 0.2 }}
                  minZoom={0.1}
                  maxZoom={2}
                  proOptions={{ hideAttribution: true }}
                  nodesDraggable
                  nodesConnectable={false}
                  colorMode="system"
                >
                  <Controls />
                  <Background gap={16} size={1} />
                </ReactFlow>
              </div>
            </TooltipProvider>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
