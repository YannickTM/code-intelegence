import { z } from "zod";
import { TRPCError } from "@trpc/server";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import {
  apiCall,
  ApiError,
  mapHttpStatusToTRPCCode,
} from "~/server/api-client";

// ── Response types (mirrors Go backend) ─────────────────────────────────────

export type PlatformUser = {
  id: string;
  username: string;
  email: string;
  display_name: string | null;
  avatar_url: string | null;
  is_active: boolean;
  created_at: string;
  updated_at: string;
  project_count: number;
  platform_roles: string[];
};

type PlatformUserListResponse = {
  items: PlatformUser[];
  total: number;
  limit: number;
  offset: number;
};

type ActionResponse = {
  message: string;
  user_id: string;
  role?: string;
};

type GrantRoleResponse = {
  id: string;
  user_id: string;
  role: string;
  granted_by: string | null;
  created_at: string;
} | {
  message: string;
  user_id: string;
  role: string;
};

// ── Router ──────────────────────────────────────────────────────────────────

export const platformUsersRouter = createTRPCRouter({
  /** GET /v1/platform-management/users → PlatformUserListResponse */
  list: protectedProcedure
    .input(
      z.object({
        limit: z.number().int().min(1).max(200).default(20),
        offset: z.number().int().min(0).default(0),
        search: z.string().optional(),
        is_active: z.boolean().optional(),
        sort: z.enum(["created_at", "username"]).optional(),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const params = new URLSearchParams();
        params.set("limit", String(input.limit));
        params.set("offset", String(input.offset));
        if (input.search) params.set("search", input.search);
        if (input.is_active !== undefined)
          params.set("is_active", String(input.is_active));
        if (input.sort) params.set("sort", input.sort);

        const qs = params.toString();
        const { data } = await apiCall<PlatformUserListResponse>({
          method: "GET",
          path: `/v1/platform-management/users${qs ? `?${qs}` : ""}`,
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
          message: "Failed to list users",
          cause: error,
        });
      }
    }),

  /** POST /v1/platform-management/users/{userId}/deactivate → ActionResponse */
  deactivateUser: protectedProcedure
    .input(z.object({ userId: z.string().uuid() }))
    .mutation(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<ActionResponse>({
          method: "POST",
          path: `/v1/platform-management/users/${input.userId}/deactivate`,
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
          message: "Failed to deactivate user",
          cause: error,
        });
      }
    }),

  /** POST /v1/platform-management/users/{userId}/activate → ActionResponse */
  activateUser: protectedProcedure
    .input(z.object({ userId: z.string().uuid() }))
    .mutation(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<ActionResponse>({
          method: "POST",
          path: `/v1/platform-management/users/${input.userId}/activate`,
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
          message: "Failed to activate user",
          cause: error,
        });
      }
    }),

  /** POST /v1/platform-management/platform-roles → GrantRoleResponse */
  grantRole: protectedProcedure
    .input(
      z.object({
        user_id: z.string().uuid(),
        role: z.string(),
      }),
    )
    .mutation(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<GrantRoleResponse>({
          method: "POST",
          path: "/v1/platform-management/platform-roles",
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
          message: "Failed to grant platform role",
          cause: error,
        });
      }
    }),

  /** DELETE /v1/platform-management/platform-roles/{userId}/{role} → ActionResponse */
  revokeRole: protectedProcedure
    .input(
      z.object({
        userId: z.string().uuid(),
        role: z.string(),
      }),
    )
    .mutation(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<ActionResponse>({
          method: "DELETE",
          path: `/v1/platform-management/platform-roles/${encodeURIComponent(input.userId)}/${encodeURIComponent(input.role)}`,
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
          message: "Failed to revoke platform role",
          cause: error,
        });
      }
    }),
});
