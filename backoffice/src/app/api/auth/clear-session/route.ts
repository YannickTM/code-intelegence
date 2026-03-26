import { type NextRequest, NextResponse } from "next/server";

/**
 * GET /api/auth/clear-session
 *
 * Clears a stale session cookie and redirects to /login.
 * Used when the backend has invalidated a session (e.g. user deactivated)
 * but the browser still holds the cookie, which would otherwise cause a
 * redirect loop between (app)/layout and /login.
 */
export function GET(req: NextRequest) {
  const loginUrl = new URL("/login", req.url);
  const response = NextResponse.redirect(loginUrl);
  response.cookies.set("session", "", { maxAge: 0, path: "/" });
  return response;
}
