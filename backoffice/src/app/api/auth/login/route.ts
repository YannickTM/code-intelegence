import { type NextRequest, NextResponse } from "next/server";
import { apiCall, ApiError } from "~/server/api-client";

type LoginResponse = {
  token: string;
  expires_at: string;
  user: {
    id: string;
    username: string;
    display_name?: string;
    avatar_url?: string;
    is_active: boolean;
    created_at: string;
    updated_at: string;
  };
};

/**
 * POST /api/auth/login
 *
 * Direct username login (bypasses OIDC). Only available in development mode.
 * In production, use the SSO flow via BetterAuth OIDC.
 */
export async function POST(req: NextRequest) {
  // Gate behind development mode — production uses OIDC via /api/auth/oidc/bridge
  if (process.env.NODE_ENV !== "development") {
    return NextResponse.json(
      { error: "Direct login is disabled. Use SSO.", code: "disabled" },
      { status: 403 },
    );
  }

  try {
    const body = (await req.json().catch(() => null)) as {
      username?: unknown;
    } | null;

    if (!body || typeof body.username !== "string" || !body.username.trim()) {
      return NextResponse.json(
        { error: "username is required", code: "validation_error" },
        { status: 400 },
      );
    }

    const { data } = await apiCall<LoginResponse>({
      method: "POST",
      path: "/v1/auth/login",
      body: { username: body.username.trim() },
    });

    const response = NextResponse.json({
      user: data.user,
      expiresAt: data.expires_at,
    });

    // Set cookie explicitly using the token from the Go backend response
    // Note: secure=false is fine here since this route is dev-only
    response.cookies.set("session", data.token, {
      httpOnly: true,
      secure: false,
      path: "/",
      expires: new Date(data.expires_at),
      sameSite: "lax",
    });

    return response;
  } catch (err) {
    if (err instanceof ApiError) {
      return NextResponse.json(
        { error: err.message, code: err.code },
        { status: err.status },
      );
    }
    return NextResponse.json(
      { error: "Internal server error" },
      { status: 500 },
    );
  }
}
