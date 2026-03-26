import { createTRPCRouter, publicProcedure } from "~/server/api/trpc";
import { apiCall } from "~/server/api-client";

type GoUser = {
  id: string;
  username: string;
  email: string;
  display_name?: string;
  avatar_url?: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
  platform_roles?: string[];
};

type MeResponse = {
  user: GoUser;
  platform_roles: string[];
};

export const authRouter = createTRPCRouter({
  /**
   * Get current session user via GET /v1/users/me on the Go backend.
   * Returns null if not authenticated (soft fail for RSC prefetching).
   */
  me: publicProcedure.query(async ({ ctx }) => {
    if (!ctx.sessionToken) {
      return { user: null };
    }
    try {
      const { data } = await apiCall<MeResponse>({
        method: "GET",
        path: "/v1/users/me",
        headers: ctx.headers,
      });
      return {
        user: { ...data.user, platform_roles: data.platform_roles ?? [] },
      };
    } catch {
      return { user: null };
    }
  }),
});
