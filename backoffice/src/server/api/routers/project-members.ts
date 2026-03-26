import { z } from "zod";
import { TRPCError } from "@trpc/server";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import {
  apiCall,
  ApiError,
  mapHttpStatusToTRPCCode,
} from "~/server/api-client";

// ── Response types (mirrors Go backend) ─────────────────────────────────────

export type ProjectMember = {
  id: string;
  project_id: string;
  user_id: string;
  username: string;
  display_name?: string;
  avatar_url?: string;
  role: "owner" | "admin" | "member";
  created_at: string;
};

type ProjectMemberListResponse = {
  items: ProjectMember[];
};

// ── Router ──────────────────────────────────────────────────────────────────

export const projectMembersRouter = createTRPCRouter({
  /** GET /v1/projects/{id}/members → ProjectMemberListResponse */
  list: protectedProcedure
    .input(z.object({ projectId: z.uuid() }))
    .query(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<ProjectMemberListResponse>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/members`,
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
          message: "Failed to list project members",
          cause: error,
        });
      }
    }),

  /** POST /v1/projects/{id}/members → ProjectMember */
  add: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        user_id: z.uuid(),
        role: z.enum(["owner", "admin", "member"]).optional(),
      }),
    )
    .mutation(async ({ ctx, input }) => {
      try {
        const { projectId, ...body } = input;
        const { data } = await apiCall<ProjectMember>({
          method: "POST",
          path: `/v1/projects/${projectId}/members`,
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
          message: "Failed to add project member",
          cause: error,
        });
      }
    }),

  /** PATCH /v1/projects/{id}/members/{user_id} → ProjectMember */
  updateRole: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        userId: z.uuid(),
        role: z.enum(["owner", "admin", "member"]),
      }),
    )
    .mutation(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<ProjectMember>({
          method: "PATCH",
          path: `/v1/projects/${input.projectId}/members/${input.userId}`,
          body: { role: input.role },
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
          message: "Failed to update member role",
          cause: error,
        });
      }
    }),

  /** DELETE /v1/projects/{id}/members/{user_id} → 204 */
  remove: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        userId: z.uuid(),
      }),
    )
    .mutation(async ({ ctx, input }) => {
      try {
        await apiCall({
          method: "DELETE",
          path: `/v1/projects/${input.projectId}/members/${input.userId}`,
          headers: ctx.headers,
        });
        return { success: true as const };
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
          message: "Failed to remove project member",
          cause: error,
        });
      }
    }),
});
