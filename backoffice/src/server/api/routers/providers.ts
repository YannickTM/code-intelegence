import { TRPCError } from "@trpc/server";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import {
  apiCall,
  ApiError,
  mapHttpStatusToTRPCCode,
} from "~/server/api-client";
import type { SupportedProvidersResponse } from "~/lib/provider-settings";

export const providersRouter = createTRPCRouter({
  listSupported: protectedProcedure.query(async ({ ctx }) => {
    try {
      const { data } = await apiCall<SupportedProvidersResponse>({
        method: "GET",
        path: "/v1/settings/providers",
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
        message: "Failed to fetch supported providers",
        cause: error,
      });
    }
  }),
});
