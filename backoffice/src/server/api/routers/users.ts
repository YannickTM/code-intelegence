import { z } from "zod";
import { TRPCError } from "@trpc/server";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import {
  apiCall,
  ApiError,
  mapHttpStatusToTRPCCode,
} from "~/server/api-client";

// ── Response types (mirrors Go backend) ─────────────────────────────────────

type UserResponse = {
  user: {
    id: string;
    username: string;
    email: string;
    display_name?: string;
    avatar_url?: string;
    is_active: boolean;
    created_at: string;
    updated_at: string;
  };
};

type UserLookupResponse = {
  user: {
    id: string;
    username: string;
    email: string;
    display_name: string;
    avatar_url: string;
  };
};

export type UserProject = {
  id: string;
  name: string;
  repo_url: string;
  default_branch: string;
  status: string;
  role: "owner" | "admin" | "member";
  created_by: string;
  created_at: string;
  updated_at: string;
  // Health fields (nullable)
  index_git_commit: string | null;
  index_branch: string | null;
  index_activated_at: string | null;
  active_job_id: string | null;
  active_job_status: string | null;
  failed_job_id: string | null;
  failed_job_finished_at: string | null;
  failed_job_type: string | null;
};

type UserProjectListResponse = {
  items: UserProject[];
};

type PersonalAPIKey = {
  id: string;
  key_type: "personal";
  key_prefix: string;
  name: string;
  role: "read" | "write";
  is_active: boolean;
  expires_at: string | null;
  last_used_at: string | null;
  created_at: string;
};

type APIKeyListResponse = {
  items: PersonalAPIKey[];
};

type CreateAPIKeyResponse = {
  id: string;
  key_type: "personal";
  key_prefix: string;
  plaintext_key: string;
  name: string;
  role: "read" | "write";
  expires_at: string | null;
  created_at: string;
};

// ── Router ──────────────────────────────────────────────────────────────────

export const usersRouter = createTRPCRouter({
  /** PATCH /v1/users/me → UserResponse */
  updateMe: protectedProcedure
    .input(
      z.object({
        display_name: z.string().max(100).or(z.literal("")).optional(),
        avatar_url: z.string().url().or(z.literal("")).optional(),
        email: z.string().email().optional(),
      }),
    )
    .mutation(async ({ ctx, input }) => {
      const { data } = await apiCall<UserResponse>({
        method: "PATCH",
        path: "/v1/users/me",
        body: input,
        headers: ctx.headers,
      });
      return data;
    }),

  /** GET /v1/users/lookup?q={query} → UserLookupResponse */
  lookupUser: protectedProcedure
    .input(z.object({ q: z.string().min(1) }))
    .query(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<UserLookupResponse>({
          method: "GET",
          path: `/v1/users/lookup?q=${encodeURIComponent(input.q)}`,
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
          message: "Failed to look up user",
          cause: error,
        });
      }
    }),

  /** GET /v1/users/me/projects → UserProjectListResponse */
  listMyProjects: protectedProcedure.query(async ({ ctx }) => {
    const { data } = await apiCall<UserProjectListResponse>({
      method: "GET",
      path: "/v1/users/me/projects",
      headers: ctx.headers,
    });
    return data;
  }),

  /** GET /v1/users/me/keys → APIKeyListResponse */
  listMyKeys: protectedProcedure.query(async ({ ctx }) => {
    try {
      const { data } = await apiCall<APIKeyListResponse>({
        method: "GET",
        path: "/v1/users/me/keys",
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
        message: "Failed to list API keys",
        cause: error,
      });
    }
  }),

  /** POST /v1/users/me/keys → CreateAPIKeyResponse (plaintext_key returned once) */
  createMyKey: protectedProcedure
    .input(
      z.object({
        name: z.string().min(1).max(100),
        role: z.enum(["read", "write"]).default("read"),
        expires_at: z.string().datetime().optional(),
      }),
    )
    .mutation(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<CreateAPIKeyResponse>({
          method: "POST",
          path: "/v1/users/me/keys",
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
          message: "Failed to create API key",
          cause: error,
        });
      }
    }),

  /** DELETE /v1/users/me/keys/{key_id} → 204 */
  deleteMyKey: protectedProcedure
    .input(z.object({ keyId: z.uuid() }))
    .mutation(async ({ ctx, input }) => {
      try {
        await apiCall({
          method: "DELETE",
          path: `/v1/users/me/keys/${input.keyId}`,
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
          message: "Failed to revoke API key",
          cause: error,
        });
      }
    }),
});
