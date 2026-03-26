import { toNextJsHandler } from "better-auth/next-js";
import { auth } from "~/lib/auth";

/**
 * BetterAuth catch-all route handler.
 *
 * Serves all BetterAuth routes:
 *   - /api/auth/sign-in/oauth2
 *   - /api/auth/oauth2/callback/*
 *   - /api/auth/get-session
 *   - etc.
 *
 * Note: Explicit routes like /api/auth/login and /api/auth/logout
 * take priority over this catch-all (Next.js routing rules).
 */
export const { GET, POST } = toNextJsHandler(auth);
