import { z } from "zod";
import { TRPCError } from "@trpc/server";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import {
  apiCall,
  ApiError,
  mapHttpStatusToTRPCCode,
} from "~/server/api-client";

// ── Response types (mirrors Go backend) ─────────────────────────────────────

type SSHKey = {
  id: string;
  name: string;
  fingerprint: string;
  public_key: string;
  key_type: string;
  is_active: boolean;
  created_by: string;
  rotated_at: string | null;
  created_at: string;
};

type SSHKeyListResponse = {
  items: SSHKey[];
};

type SSHKeyProject = {
  id: string;
  name: string;
  repo_url: string;
  default_branch: string;
  status: string;
  created_by: string;
  created_at: string;
  updated_at: string;
};

type SSHKeyProjectListResponse = {
  items: SSHKeyProject[];
  total: number;
};

// ── Router ──────────────────────────────────────────────────────────────────

export const sshKeysRouter = createTRPCRouter({
  /** GET /v1/ssh-keys → SSHKeyListResponse */
  list: protectedProcedure.query(
    async ({ ctx }): Promise<SSHKeyListResponse> => {
      try {
        const { data } = await apiCall<SSHKeyListResponse>({
          method: "GET",
          path: "/v1/ssh-keys",
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
          message: "Failed to list SSH keys",
          cause: error,
        });
      }
    },
  ),

  /** POST /v1/ssh-keys → SSHKey (generate or import) */
  create: protectedProcedure
    .input(
      z.object({
        name: z.string().trim().min(1).max(100),
        private_key: z.string().trim().min(1).optional(), // PEM for import; omit to generate Ed25519
      }),
    )
    .mutation(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<SSHKey>({
          method: "POST",
          path: "/v1/ssh-keys",
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
          message: "Failed to create SSH key",
          cause: error,
        });
      }
    }),

  /** GET /v1/ssh-keys/{id} → SSHKey */
  get: protectedProcedure
    .input(z.object({ id: z.uuid() }))
    .query(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<SSHKey>({
          method: "GET",
          path: `/v1/ssh-keys/${input.id}`,
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
          message: "Failed to get SSH key",
          cause: error,
        });
      }
    }),

  /** GET /v1/ssh-keys/{id}/projects → SSHKeyProjectListResponse */
  getProjects: protectedProcedure
    .input(z.object({ id: z.uuid() }))
    .query(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<SSHKeyProjectListResponse>({
          method: "GET",
          path: `/v1/ssh-keys/${input.id}/projects`,
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
          message: "Failed to list projects for SSH key",
          cause: error,
        });
      }
    }),

  /** POST /v1/ssh-keys/{id}/retire → SSHKey */
  retire: protectedProcedure
    .input(z.object({ id: z.uuid() }))
    .mutation(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<SSHKey>({
          method: "POST",
          path: `/v1/ssh-keys/${input.id}/retire`,
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
          message: "Failed to retire SSH key",
          cause: error,
        });
      }
    }),
});
