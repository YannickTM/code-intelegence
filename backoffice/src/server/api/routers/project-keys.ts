import { z } from "zod";
import { TRPCError } from "@trpc/server";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import {
  apiCall,
  ApiError,
  mapHttpStatusToTRPCCode,
} from "~/server/api-client";

// ── Response types (mirrors Go backend) ─────────────────────────────────────

type APIKey = {
  id: string;
  key_type: "project";
  key_prefix: string;
  name: string;
  role: "read" | "write";
  is_active: boolean;
  project_id: string;
  expires_at: string | null;
  last_used_at: string | null;
  created_by: string;
  created_at: string;
};

type APIKeyListResponse = {
  items: APIKey[];
};

type CreateAPIKeyResponse = {
  id: string;
  key_type: "project";
  key_prefix: string;
  plaintext_key: string;
  name: string;
  role: "read" | "write";
  project_id: string;
  expires_at: string | null;
  created_at: string;
};

// ── Router ──────────────────────────────────────────────────────────────────

export const projectKeysRouter = createTRPCRouter({
  /** GET /v1/projects/{id}/keys → APIKeyListResponse */
  list: protectedProcedure
    .input(z.object({ projectId: z.uuid() }))
    .query(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<APIKeyListResponse>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/keys`,
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
          message: "Failed to list project API keys",
          cause: error,
        });
      }
    }),

  /** POST /v1/projects/{id}/keys → CreateAPIKeyResponse (plaintext_key returned once) */
  create: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        name: z.string().min(1).max(100),
        role: z.enum(["read", "write"]).default("read"),
        expires_at: z.string().datetime().optional(),
      }),
    )
    .mutation(async ({ ctx, input }) => {
      try {
        const { projectId, ...body } = input;
        const { data } = await apiCall<CreateAPIKeyResponse>({
          method: "POST",
          path: `/v1/projects/${projectId}/keys`,
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
          message: "Failed to create project API key",
          cause: error,
        });
      }
    }),

  /** DELETE /v1/projects/{id}/keys/{key_id} → 204 */
  delete: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        keyId: z.uuid(),
      }),
    )
    .mutation(async ({ ctx, input }) => {
      try {
        await apiCall({
          method: "DELETE",
          path: `/v1/projects/${input.projectId}/keys/${input.keyId}`,
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
          message: "Failed to revoke project API key",
          cause: error,
        });
      }
    }),
});
