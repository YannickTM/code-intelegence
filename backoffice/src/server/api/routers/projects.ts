import { z } from "zod";
import { TRPCError } from "@trpc/server";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import {
  apiCall,
  ApiError,
  mapHttpStatusToTRPCCode,
} from "~/server/api-client";

// ── Response types (mirrors Go backend) ─────────────────────────────────────

type Project = {
  id: string;
  name: string;
  repo_url: string;
  default_branch: string;
  status: string;
  created_by: string;
  created_at: string;
  updated_at: string;
};

type ProjectDetailResponse = Project & {
  index_git_commit: string | null;
  index_branch: string | null;
  index_activated_at: string | null;
  active_job_id: string | null;
  active_job_status: string | null;
  failed_job_id: string | null;
  failed_job_finished_at: string | null;
  failed_job_type: string | null;
};

type _ProjectListResponse = {
  data: Project[];
  total: number;
  limit: number;
  offset: number;
};

type SSHKeySummary = {
  id: string;
  name: string;
  fingerprint: string;
  public_key: string;
  key_type: string;
  created_at: string;
};

type FileNode = {
  path: string;
  name: string;
  node_type: "file" | "directory";
  children?: FileNode[];
  language?: string;
  size_bytes?: number;
};

type ProjectStructureResponse = {
  root: FileNode;
  snapshot_id: string;
  git_commit: string;
  branch: string;
  file_count: number;
};

// ── Router ──────────────────────────────────────────────────────────────────

export const projectsRouter = createTRPCRouter({
  /** GET /v1/projects?limit=&offset= → ProjectListResponse */
  list: protectedProcedure
    .input(
      z
        .object({
          limit: z.number().int().min(1).max(100).default(20),
          offset: z.number().int().min(0).default(0),
        })
        .optional(),
    )
    .query(async () => {
      // TODO: implement
      throw new TRPCError({
        code: "NOT_IMPLEMENTED",
        message: "projects.list not implemented",
      });
    }),

  /** POST /v1/projects → Project */
  create: protectedProcedure
    .input(
      z.object({
        name: z.string().min(1).max(100),
        repo_url: z.string().min(1),
        default_branch: z.string().default("main"),
        status: z.enum(["active", "paused"]).optional(),
        ssh_key_id: z.uuid(),
      }),
    )
    .mutation(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<Project & { ssh_key: SSHKeySummary }>({
          method: "POST",
          path: "/v1/projects",
          body: input,
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
          message: "Failed to create project",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id} → ProjectDetailResponse (with health fields) */
  get: protectedProcedure
    .input(z.object({ id: z.uuid() }))
    .query(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<ProjectDetailResponse>({
          method: "GET",
          path: `/v1/projects/${input.id}`,
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
          message: "Failed to fetch project",
          cause: error,
        });
      }
    }),

  /** PATCH /v1/projects/{id} → Project */
  update: protectedProcedure
    .input(
      z
        .object({
          id: z.uuid(),
          name: z.string().min(1).max(100).optional(),
          repo_url: z.string().min(1).optional(),
          default_branch: z.string().optional(),
          status: z.enum(["active", "paused"]).optional(),
        })
        .refine(
          (v) =>
            v.name !== undefined ||
            v.repo_url !== undefined ||
            v.default_branch !== undefined ||
            v.status !== undefined,
          { message: "At least one field to update is required" },
        ),
    )
    .mutation(async ({ ctx, input }) => {
      try {
        const { id, ...body } = input;
        const { data } = await apiCall<Project>({
          method: "PATCH",
          path: `/v1/projects/${id}`,
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
          message: "Failed to update project",
          cause: error,
        });
      }
    }),

  /** DELETE /v1/projects/{id} → 204 */
  delete: protectedProcedure
    .input(z.object({ id: z.uuid() }))
    .mutation(async ({ ctx, input }) => {
      try {
        await apiCall({
          method: "DELETE",
          path: `/v1/projects/${input.id}`,
          headers: ctx.headers,
        });
        return { success: true };
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
          message: "Failed to delete project",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/structure → file tree */
  structure: protectedProcedure
    .input(z.object({ id: z.uuid() }))
    .query(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<ProjectStructureResponse>({
          method: "GET",
          path: `/v1/projects/${input.id}/structure`,
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
          message: "Failed to fetch project structure",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/ssh-key → SSHKeySummary | null */
  getSSHKey: protectedProcedure
    .input(z.object({ id: z.uuid() }))
    .query(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<SSHKeySummary>({
          method: "GET",
          path: `/v1/projects/${input.id}/ssh-key`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        if (error instanceof ApiError) {
          // 404 = no key assigned
          if (error.status === 404) return null;
          throw new TRPCError({
            code: mapHttpStatusToTRPCCode(error.status),
            message: error.message,
            cause: error,
          });
        }
        throw new TRPCError({
          code: "INTERNAL_SERVER_ERROR",
          message: "Failed to fetch SSH key",
          cause: error,
        });
      }
    }),

  /** PUT /v1/projects/{id}/ssh-key → SSHKeySummary */
  putSSHKey: protectedProcedure
    .input(
      z.union([
        z.object({
          id: z.uuid(),
          ssh_key_id: z.uuid(),
        }),
        z.object({
          id: z.uuid(),
          generate: z.literal(true),
          name: z.string().min(1).max(100),
        }),
      ]),
    )
    .mutation(async ({ ctx, input }) => {
      try {
        const { id, ...body } = input;
        const { data } = await apiCall<SSHKeySummary>({
          method: "PUT",
          path: `/v1/projects/${id}/ssh-key`,
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
          message: "Failed to update SSH key",
          cause: error,
        });
      }
    }),

  /** DELETE /v1/projects/{id}/ssh-key → 204 */
  deleteSSHKey: protectedProcedure
    .input(z.object({ id: z.uuid() }))
    .mutation(async ({ ctx, input }) => {
      try {
        await apiCall({
          method: "DELETE",
          path: `/v1/projects/${input.id}/ssh-key`,
          headers: ctx.headers,
        });
        return { success: true };
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
          message: "Failed to remove SSH key",
          cause: error,
        });
      }
    }),
});
