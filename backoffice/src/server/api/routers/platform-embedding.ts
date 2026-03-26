import { z } from "zod";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import { apiCall } from "~/server/api-client";
import type {
  ConnectivityTestResponse,
  EmbeddingProviderConfig,
} from "~/lib/provider-settings";
import {
  providerEndpointUrlSchema,
  removeUndefinedFields,
  throwApiErrorAsTRPC,
} from "./provider-router-shared";

const embeddingUpdateInput = z.object({
  name: z.string().trim().min(1),
  provider: z.string().trim().min(1),
  endpoint_url: providerEndpointUrlSchema,
  model: z.string().trim().min(1),
  dimensions: z.number().int().min(1).max(65536),
  max_tokens: z.number().int().min(1).max(131072),
  is_available_to_projects: z.boolean().optional(),
});

const embeddingPatchInput = z
  .object({
    configId: z.string().uuid(),
    name: z.string().trim().min(1).optional(),
    provider: z.string().trim().min(1).optional(),
    endpoint_url: providerEndpointUrlSchema.optional(),
    model: z.string().trim().min(1).optional(),
    dimensions: z.number().int().min(1).max(65536).optional(),
    max_tokens: z.number().int().min(1).max(131072).optional(),
    is_available_to_projects: z.boolean().optional(),
  })
  .refine(
    (data) => {
      const { configId: _, ...rest } = data;
      return Object.values(rest).some((v) => v !== undefined);
    },
    { message: "At least one field must be provided" },
  );

const configIdInput = z.object({ configId: z.string().uuid() });

export const platformEmbeddingRouter = createTRPCRouter({
  list: protectedProcedure.query(async ({ ctx }) => {
    try {
      const { data } = await apiCall<{ items: EmbeddingProviderConfig[] }>({
        method: "GET",
        path: "/v1/platform-management/settings/embedding",
        headers: ctx.headers,
      });
      return data;
    } catch (error) {
      throwApiErrorAsTRPC(
        error,
        "Failed to fetch platform embedding configs",
      );
    }
  }),

  update: protectedProcedure
    .input(embeddingUpdateInput)
    .mutation(async ({ ctx, input }) => {
      try {
        const cleanedBody = removeUndefinedFields(input);
        const { data } = await apiCall<{ config: EmbeddingProviderConfig }>({
          method: "PUT",
          path: "/v1/platform-management/settings/embedding",
          body: cleanedBody,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(
          error,
          "Failed to update platform embedding config",
        );
      }
    }),

  test: protectedProcedure.mutation(async ({ ctx }) => {
    try {
      const { data } = await apiCall<ConnectivityTestResponse>({
        method: "POST",
        path: "/v1/platform-management/settings/embedding/test",
        headers: ctx.headers,
      });
      return data;
    } catch (error) {
      throwApiErrorAsTRPC(
        error,
        "Failed to test platform embedding connectivity",
      );
    }
  }),

  create: protectedProcedure
    .input(embeddingUpdateInput)
    .mutation(async ({ ctx, input }) => {
      try {
        const cleanedBody = removeUndefinedFields(input);
        const { data } = await apiCall<{ config: EmbeddingProviderConfig }>({
          method: "POST",
          path: "/v1/platform-management/settings/embedding",
          body: cleanedBody,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(error, "Failed to create embedding provider");
      }
    }),

  updateById: protectedProcedure
    .input(embeddingPatchInput)
    .mutation(async ({ ctx, input }) => {
      try {
        const { configId, ...fields } = input;
        const cleanedBody = removeUndefinedFields(fields);
        const { data } = await apiCall<{ config: EmbeddingProviderConfig }>({
          method: "PATCH",
          path: `/v1/platform-management/settings/embedding/${configId}`,
          body: cleanedBody,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(error, "Failed to update embedding provider");
      }
    }),

  deleteById: protectedProcedure
    .input(configIdInput)
    .mutation(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<{ message: string; config_id: string }>({
          method: "DELETE",
          path: `/v1/platform-management/settings/embedding/${input.configId}`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(error, "Failed to delete embedding provider");
      }
    }),

  promote: protectedProcedure
    .input(configIdInput)
    .mutation(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<{
          message: string;
          config: EmbeddingProviderConfig;
        }>({
          method: "POST",
          path: `/v1/platform-management/settings/embedding/${input.configId}/promote`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(
          error,
          "Failed to promote embedding provider to default",
        );
      }
    }),

  testById: protectedProcedure
    .input(configIdInput)
    .mutation(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<ConnectivityTestResponse>({
          method: "POST",
          path: `/v1/platform-management/settings/embedding/${input.configId}/test`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(
          error,
          "Failed to test embedding provider connectivity",
        );
      }
    }),
});
