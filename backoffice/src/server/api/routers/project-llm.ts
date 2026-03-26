import { z } from "zod";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import { apiCall, ApiError } from "~/server/api-client";
import type {
  AvailableProviderConfigsResponse,
  ConnectivityTestResponse,
  LLMProviderConfig,
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

const customLLMInput = z.object({
  mode: z.literal("custom"),
  name: z.string().trim().min(1),
  provider: z.string().trim().min(1),
  endpoint_url: providerEndpointUrlSchema,
  model: z.string().trim().min(1).optional(),
});

const llmUpdateInput = projectIdInput.and(
  z.discriminatedUnion("mode", [globalSelectionInput, customLLMInput]),
);

const llmTestInput = z.object({
  projectId: z.uuid(),
  provider: z.string().trim().min(1).optional(),
  endpoint_url: providerEndpointUrlSchema.optional(),
  model: z.string().trim().min(1).optional(),
});

export const projectLLMRouter = createTRPCRouter({
  get: protectedProcedure
    .input(projectIdInput)
    .query(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<
          ProjectProviderSetting<LLMProviderConfig>
        >({
          method: "GET",
          path: `/v1/projects/${input.projectId}/settings/llm`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(error, "Failed to fetch LLM provider settings");
      }
    }),

  getAvailable: protectedProcedure
    .input(projectIdInput)
    .query(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<
          AvailableProviderConfigsResponse<LLMProviderConfig>
        >({
          method: "GET",
          path: `/v1/projects/${input.projectId}/settings/llm/available`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(error, "Failed to fetch available LLM configs");
      }
    }),

  put: protectedProcedure
    .input(llmUpdateInput)
    .mutation(async ({ ctx, input }) => {
      try {
        const { projectId, ...body } = input;
        const cleanedBody = removeUndefinedFields(body);
        const { data } = await apiCall<
          ProjectProviderSetting<LLMProviderConfig>
        >({
          method: "PUT",
          path: `/v1/projects/${projectId}/settings/llm`,
          body: cleanedBody,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(error, "Failed to update LLM provider settings");
      }
    }),

  delete: protectedProcedure
    .input(projectIdInput)
    .mutation(async ({ ctx, input }) => {
      try {
        await apiCall({
          method: "DELETE",
          path: `/v1/projects/${input.projectId}/settings/llm`,
          headers: ctx.headers,
        });
        return { success: true };
      } catch (error) {
        throwApiErrorAsTRPC(error, "Failed to reset LLM provider settings");
      }
    }),

  getResolved: protectedProcedure
    .input(projectIdInput)
    .query(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<
          ResolvedProjectProvider<LLMProviderConfig>
        >({
          method: "GET",
          path: `/v1/projects/${input.projectId}/settings/llm/resolved`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        if (error instanceof ApiError) {
          if (error.status === 404) {
            return null;
          }
        }
        throwApiErrorAsTRPC(
          error,
          "Failed to fetch resolved LLM provider settings",
        );
      }
    }),

  test: protectedProcedure
    .input(llmTestInput)
    .mutation(async ({ ctx, input }) => {
      try {
        const { projectId, ...body } = input;
        const cleanedBody = removeUndefinedFields(body);
        const { data } = await apiCall<ConnectivityTestResponse>({
          method: "POST",
          path: `/v1/projects/${projectId}/settings/llm/test`,
          body: cleanedBody,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(error, "Failed to test LLM connectivity");
      }
    }),
});
