import { z } from "zod";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import { apiCall } from "~/server/api-client";
import type {
  ConnectivityTestResponse,
  LLMProviderConfig,
} from "~/lib/provider-settings";
import {
  providerEndpointUrlSchema,
  removeUndefinedFields,
  throwApiErrorAsTRPC,
} from "./provider-router-shared";

const llmCreateInput = z.object({
  name: z.string().trim().min(1),
  provider: z.string().trim().min(1),
  endpoint_url: providerEndpointUrlSchema,
  model: z.string().trim().min(1),
  is_available_to_projects: z.boolean().optional(),
});

const llmPatchInput = z
  .object({
    configId: z.string().uuid(),
    name: z.string().trim().min(1).optional(),
    provider: z.string().trim().min(1).optional(),
    endpoint_url: providerEndpointUrlSchema.optional(),
    model: z.string().trim().min(1).optional(),
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

export const platformLLMRouter = createTRPCRouter({
  list: protectedProcedure.query(async ({ ctx }) => {
    try {
      const { data } = await apiCall<{ items: LLMProviderConfig[] }>({
        method: "GET",
        path: "/v1/platform-management/settings/llm",
        headers: ctx.headers,
      });
      return data;
    } catch (error) {
      throwApiErrorAsTRPC(error, "Failed to fetch platform LLM configs");
    }
  }),

  update: protectedProcedure
    .input(llmCreateInput)
    .mutation(async ({ ctx, input }) => {
      try {
        const cleanedBody = removeUndefinedFields(input);
        const { data } = await apiCall<{ config: LLMProviderConfig }>({
          method: "PUT",
          path: "/v1/platform-management/settings/llm",
          body: cleanedBody,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(error, "Failed to update platform LLM config");
      }
    }),

  test: protectedProcedure.mutation(async ({ ctx }) => {
    try {
      const { data } = await apiCall<ConnectivityTestResponse>({
        method: "POST",
        path: "/v1/platform-management/settings/llm/test",
        headers: ctx.headers,
      });
      return data;
    } catch (error) {
      throwApiErrorAsTRPC(
        error,
        "Failed to test platform LLM connectivity",
      );
    }
  }),

  create: protectedProcedure
    .input(llmCreateInput)
    .mutation(async ({ ctx, input }) => {
      try {
        const cleanedBody = removeUndefinedFields(input);
        const { data } = await apiCall<{ config: LLMProviderConfig }>({
          method: "POST",
          path: "/v1/platform-management/settings/llm",
          body: cleanedBody,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(error, "Failed to create LLM provider");
      }
    }),

  updateById: protectedProcedure
    .input(llmPatchInput)
    .mutation(async ({ ctx, input }) => {
      try {
        const { configId, ...fields } = input;
        const cleanedBody = removeUndefinedFields(fields);
        const { data } = await apiCall<{ config: LLMProviderConfig }>({
          method: "PATCH",
          path: `/v1/platform-management/settings/llm/${configId}`,
          body: cleanedBody,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(error, "Failed to update LLM provider");
      }
    }),

  deleteById: protectedProcedure
    .input(configIdInput)
    .mutation(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<{ message: string; config_id: string }>({
          method: "DELETE",
          path: `/v1/platform-management/settings/llm/${input.configId}`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(error, "Failed to delete LLM provider");
      }
    }),

  promote: protectedProcedure
    .input(configIdInput)
    .mutation(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<{
          message: string;
          config: LLMProviderConfig;
        }>({
          method: "POST",
          path: `/v1/platform-management/settings/llm/${input.configId}/promote`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(
          error,
          "Failed to promote LLM provider to default",
        );
      }
    }),

  testById: protectedProcedure
    .input(configIdInput)
    .mutation(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<ConnectivityTestResponse>({
          method: "POST",
          path: `/v1/platform-management/settings/llm/${input.configId}/test`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        throwApiErrorAsTRPC(
          error,
          "Failed to test LLM provider connectivity",
        );
      }
    }),
});
