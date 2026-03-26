import { z } from "zod";
import { TRPCError } from "@trpc/server";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import {
  apiCall,
  ApiError,
  mapHttpStatusToTRPCCode,
} from "~/server/api-client";

// ── Response types (mirrors Go backend) ─────────────────────────────────────

export type IndexJob = {
  id: string;
  project_id: string;
  index_snapshot_id: string | null;
  job_type: "full" | "incremental";
  status: "queued" | "running" | "completed" | "failed";
  files_processed: number;
  chunks_upserted: number;
  vectors_deleted: number;
  error_details: Array<{ category: string; message: string; step: string }>;
  embedding_provider_config_id: string;
  llm_provider_config_id: string | null;
  started_at: string | null;
  finished_at: string | null;
  created_at: string;
};

type IndexJobListResponse = {
  items: IndexJob[];
  total: number;
  limit: number;
  offset: number;
};

// ── Router ──────────────────────────────────────────────────────────────────

export const projectIndexingRouter = createTRPCRouter({
  /** POST /v1/projects/{id}/index → IndexJob (202 Accepted) */
  triggerIndex: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        job_type: z.enum(["full", "incremental"]).optional(),
      }),
    )
    .mutation(async ({ ctx, input }) => {
      const { projectId, ...body } = input;
      try {
        const { data } = await apiCall<IndexJob>({
          method: "POST",
          path: `/v1/projects/${projectId}/index`,
          body,
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
          message: "Failed to trigger indexing",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/jobs → IndexJobListResponse */
  listJobs: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        limit: z.number().min(1).max(100).optional(),
        offset: z.number().min(0).optional(),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const params = new URLSearchParams();
        if (input.limit != null) params.set("limit", String(input.limit));
        if (input.offset != null) params.set("offset", String(input.offset));
        const qs = params.toString();

        const { data } = await apiCall<IndexJobListResponse>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/jobs${qs ? `?${qs}` : ""}`,
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
          message: "Failed to fetch jobs",
          cause: error,
        });
      }
    }),
});
