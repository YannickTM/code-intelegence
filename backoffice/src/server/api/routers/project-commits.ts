import { z } from "zod";
import { TRPCError } from "@trpc/server";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import {
  apiCall,
  ApiError,
  mapHttpStatusToTRPCCode,
} from "~/server/api-client";

// ── Response types (mirrors Go backend) ─────────────────────────────────────

export type CommitSummary = {
  id: string;
  commit_hash: string;
  short_hash: string;
  author_name: string;
  author_email?: string;
  author_date: string;
  committer_name: string;
  committer_email?: string;
  committer_date: string;
  message: string;
  message_subject: string;
};

type CommitListResponse = {
  items: CommitSummary[];
  total: number;
  limit: number;
  offset: number;
};

export type CommitParent = {
  parent_commit_id: string;
  parent_commit_hash: string;
  parent_short_hash: string;
  ordinal: number;
};

export type CommitDiffStats = {
  files_changed: number;
  total_additions: number;
  total_deletions: number;
};

export type CommitDetail = CommitSummary & {
  parents: CommitParent[];
  diff_stats: CommitDiffStats;
};

export type FileDiff = {
  id: string;
  old_file_path: string | null;
  new_file_path: string | null;
  change_type: string;
  patch?: string | null;
  additions: number;
  deletions: number;
  parent_commit_id: string | null;
};

export type CommitDiffsResponse = {
  commit_hash: string;
  diffs: FileDiff[];
  total: number;
  limit: number;
  offset: number;
};

// ── Router ──────────────────────────────────────────────────────────────────

export const projectCommitsRouter = createTRPCRouter({
  /** GET /v1/projects/{id}/commits → CommitListResponse */
  listCommits: protectedProcedure
    .input(
      z.object({
        projectId: z.string().uuid(),
        search: z.string().max(500).optional(),
        fromDate: z.string().optional(),
        toDate: z.string().optional(),
        limit: z.number().int().min(1).max(100).optional(),
        offset: z.number().int().min(0).optional(),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const params = new URLSearchParams();
        if (input.search) params.set("search", input.search);
        if (input.fromDate) params.set("from_date", input.fromDate);
        if (input.toDate) params.set("to_date", input.toDate);
        if (input.limit != null) params.set("limit", String(input.limit));
        if (input.offset != null) params.set("offset", String(input.offset));
        const qs = params.toString();

        const { data } = await apiCall<CommitListResponse>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/commits${qs ? `?${qs}` : ""}`,
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
          message: "Failed to fetch commits",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/commits/{hash} → CommitDetail */
  getCommit: protectedProcedure
    .input(
      z.object({
        projectId: z.string().uuid(),
        commitHash: z.string().min(1),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<CommitDetail>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/commits/${encodeURIComponent(input.commitHash)}`,
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
          message: "Failed to fetch commit",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/commits/{hash}/diffs → CommitDiffsResponse */
  getCommitDiffs: protectedProcedure
    .input(
      z.object({
        projectId: z.string().uuid(),
        commitHash: z.string().min(1),
        includePatch: z.boolean().optional(),
        diffId: z.string().uuid().optional(),
        limit: z.number().int().min(1).max(500).optional(),
        offset: z.number().int().min(0).optional(),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const params = new URLSearchParams();
        if (input.includePatch) params.set("include_patch", "true");
        if (input.diffId) params.set("diff_id", input.diffId);
        if (input.limit != null) params.set("limit", String(input.limit));
        if (input.offset != null) params.set("offset", String(input.offset));
        const qs = params.toString();

        const { data } = await apiCall<CommitDiffsResponse>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/commits/${encodeURIComponent(input.commitHash)}/diffs${qs ? `?${qs}` : ""}`,
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
          message: "Failed to fetch commit diffs",
          cause: error,
        });
      }
    }),
});
