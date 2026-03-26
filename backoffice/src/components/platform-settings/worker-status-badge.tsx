import { Badge } from "~/components/ui/badge";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "~/components/ui/tooltip";

const statusConfig = {
  starting: {
    label: "Starting",
    className: "bg-amber-500/10 text-amber-600 dark:text-amber-400",
  },
  idle: {
    label: "Idle",
    className: "bg-success/10 text-success",
  },
  busy: {
    label: "Busy",
    className: "bg-blue-500/10 text-blue-600 dark:text-blue-400",
  },
  draining: {
    label: "Draining",
    className: "bg-orange-500/10 text-orange-600 dark:text-orange-400",
  },
  stopped: {
    label: "Stopped",
    className: "text-muted-foreground bg-muted",
  },
} as const;

export function WorkerStatusBadge({
  status,
  drainReason,
}: {
  status: keyof typeof statusConfig;
  drainReason?: string;
}) {
  const config = statusConfig[status] ?? statusConfig.stopped;

  const badge = (
    <Badge variant="secondary" className={config.className}>
      {config.label}
    </Badge>
  );

  if (status === "draining" && drainReason) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>{badge}</TooltipTrigger>
        <TooltipContent>{drainReason}</TooltipContent>
      </Tooltip>
    );
  }

  return badge;
}
