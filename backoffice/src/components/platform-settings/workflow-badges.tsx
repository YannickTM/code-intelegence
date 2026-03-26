import { Badge } from "~/components/ui/badge";

const workflowLabels: Record<string, string> = {
  "full-index": "Full Index",
  "incremental-index": "Incremental",
  "code-analysis": "Code Analysis",
  "rag-file": "RAG File",
  "rag-repo": "RAG Repo",
  "agent-run": "Agent",
};

export function WorkflowBadges({ workflows }: { workflows: string[] }) {
  return (
    <div className="flex flex-wrap gap-1">
      {workflows.map((wf) => (
        <Badge key={wf} variant="secondary" className="text-xs">
          {workflowLabels[wf] ?? wf}
        </Badge>
      ))}
    </div>
  );
}
