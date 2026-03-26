import { TRPCError } from "@trpc/server";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import { apiCall, ApiError, mapHttpStatusToTRPCCode } from "~/server/api-client";

// ── Response types ──────────────────────────────────────────────────────────

export type WorkerStatus = {
  worker_id: string;
  status: "starting" | "idle" | "busy" | "draining" | "stopped";
  started_at: string;
  last_heartbeat_at: string;
  supported_workflows: Array<
    | "full-index"
    | "incremental-index"
    | "code-analysis"
    | "rag-file"
    | "rag-repo"
    | "agent-run"
  >;
  hostname?: string;
  version?: string;
  current_job_id?: string;
  current_project_id?: string;
  drain_reason?: string;
};

type WorkerListResponse = {
  items: WorkerStatus[];
  count: number;
};

// ── Router ──────────────────────────────────────────────────────────────────

export const platformWorkersRouter = createTRPCRouter({
  list: protectedProcedure.query(async ({ ctx }) => {
    try {
      const { data } = await apiCall<WorkerListResponse>({
        method: "GET",
        path: "/v1/platform-management/workers",
        headers: ctx.headers,
      });
      return data;
    } catch (error) {
      if (error instanceof ApiError) {
        throw new TRPCError({
          code: mapHttpStatusToTRPCCode(error.status),
          message: error.message,
          cause: error,
        });
      }
      throw new TRPCError({
        code: "INTERNAL_SERVER_ERROR",
        message: "Failed to load worker status",
        cause: error,
      });
    }
  }),
});
