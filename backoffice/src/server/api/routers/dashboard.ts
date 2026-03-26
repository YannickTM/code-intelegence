import { TRPCError } from "@trpc/server";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import {
  apiCall,
  ApiError,
  mapHttpStatusToTRPCCode,
} from "~/server/api-client";
import type { DashboardSummary } from "~/lib/dashboard-types";

// ── Router ──────────────────────────────────────────────────────────────────

export const dashboardRouter = createTRPCRouter({
  /** GET /v1/dashboard/summary → DashboardSummary */
  summary: protectedProcedure.query(async ({ ctx }) => {
    try {
      const { data } = await apiCall<DashboardSummary>({
        method: "GET",
        path: "/v1/dashboard/summary",
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
        message: "Failed to fetch dashboard summary",
        cause: error,
      });
    }
  }),
});
