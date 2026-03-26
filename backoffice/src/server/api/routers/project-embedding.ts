import { z } from "zod";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import { apiCall, ApiError } from "~/server/api-client";
import type {
  AvailableProviderConfigsResponse,
  ConnectivityTestResponse,
  EmbeddingProviderConfig,
  ProjectProviderSetting,
  ResolvedProjectProvider,
} from "~/lib/provider-settings";
import {
  providerEndpointUrlSchema,
  removeUndefinedFields,
  throwApiErrorAsTRPC,
} from "./provider-router-shared";

const projectIdInput = z.object({ projectId: z.uuid() });

const globalSelectionInput = z.object({
  mode: z.literal("global"),
  global_config_id: z.uuid(),
});

const customEmbeddingInput = z.object({
  mode: z.literal("custom"),
  name: z.string().trim().min(1),
  provider: z.string().trim().min(1),
  endpoint_url: providerEndpointUrlSchema,
  model: z.string().trim().min(1),
  dimensions: z.number().int().min(1).max(65536),
  max_tokens: z.number().int().min(1).max(131072),
});

const embeddingUpdateInput = projectIdInput.and(
  z.discriminatedUnion("mode", [globalSelectionInput, customEmbeddingInput]),
);

const embeddingTestInput = z.object({
  projectId: z.uuid(),
  provider: z.string().trim().min(1).optional(),
  endpoint_url: providerEndpointUrlSchema.optional(),
  model: z.string().trim().min(1).optional(),
  dimensions: z.number().int().min(1).max(65536).optional(),
});

export const projectEmbeddingRouter = createTRPCRouter({
  get: protectedProcedure
    .input(projectIdInput)
    .query(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<
          ProjectProviderSetting<EmbeddingProviderConfig>
        >({
          method: "GET",
          path: `/v1/projects/${input.projectId}/settings/embedding`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(error, "Failed to fetch embedding config");
      }
    }),

  getAvailable: protectedProcedure
    .input(projectIdInput)
    .query(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<
          AvailableProviderConfigsResponse<EmbeddingProviderConfig>
        >({
          method: "GET",
          path: `/v1/projects/${input.projectId}/settings/embedding/available`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(
          error,
          "Failed to fetch available embedding configs",
        );
      }
    }),

  put: protectedProcedure
    .input(embeddingUpdateInput)
    .mutation(async ({ ctx, input }) => {
      try {
        const { projectId, ...body } = input;
        const cleanedBody = removeUndefinedFields(body);
        const { data } = await apiCall<
          ProjectProviderSetting<EmbeddingProviderConfig>
        >({
          method: "PUT",
          path: `/v1/projects/${projectId}/settings/embedding`,
          body: cleanedBody,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(error, "Failed to update embedding config");
      }
    }),

  delete: protectedProcedure
    .input(projectIdInput)
    .mutation(async ({ ctx, input }) => {
      try {
        await apiCall({
          method: "DELETE",
          path: `/v1/projects/${input.projectId}/settings/embedding`,
          headers: ctx.headers,
        });
        return { success: true };
      } catch (error) {
        throwApiErrorAsTRPC(error, "Failed to delete embedding config");
      }
    }),

  getResolved: protectedProcedure
    .input(projectIdInput)
    .query(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<
          ResolvedProjectProvider<EmbeddingProviderConfig>
        >({
          method: "GET",
          path: `/v1/projects/${input.projectId}/settings/embedding/resolved`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        if (error instanceof ApiError) {
          if (error.status === 404) {
            return null;
          }
        }
        throwApiErrorAsTRPC(error, "Failed to fetch resolved embedding config");
      }
    }),

  test: protectedProcedure
    .input(embeddingTestInput)
    .mutation(async ({ ctx, input }) => {
      try {
        const { projectId, ...body } = input;
        const cleanedBody = removeUndefinedFields(body);
        const { data } = await apiCall<ConnectivityTestResponse>({
          method: "POST",
          path: `/v1/projects/${projectId}/settings/embedding/test`,
          body: cleanedBody,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(error, "Failed to test embedding connectivity");
      }
    }),
});
