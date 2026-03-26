import { type NextRequest, NextResponse } from "next/server";
import { auth } from "~/lib/auth";
import { apiCall, ApiError } from "~/server/api-client";

type GoLoginResponse = {
  token: string;
  expires_at: string;
  user: {
    id: string;
    username: string;
    email: string;
    display_name?: string;
    avatar_url?: string;
    is_active: boolean;
    created_at: string;
    updated_at: string;
  };
};

/**
 * POST /api/auth/oidc/bridge
 *
 * Bridge between BetterAuth OIDC and Go backend sessions.
 *
 * Flow:
 *   1. Read BetterAuth session (from the OIDC callback that just completed)
 *   2. Extract OIDC user profile (email, name, sub, avatar)
 *   3. Call Go backend POST /v1/auth/login with OIDC identity
 *   4. Set Go session cookie from response
 *   5. Return user JSON
 */
export async function POST(req: NextRequest) {
  try {
    // 1. Read BetterAuth session
    const session = await auth.api.getSession({
      headers: req.headers,
    });

    if (!session?.user) {
      return NextResponse.json(
        {
          error: "No OIDC session found. Please sign in first.",
          code: "no_session",
        },
        { status: 401 },
      );
    }

    const { user } = session;

    // 2. Call Go backend to create/find user and get a Go session
    const { data } = await apiCall<GoLoginResponse>({
      method: "POST",
      path: "/v1/auth/login",
      body: {
        // Standard username login field (Go backend currently expects this)
        username: user.email ?? user.name ?? user.id,
        // OIDC identity fields (for future Go backend support)
        provider: "oidc",
        oidc_sub: user.id,
        email: user.email,
        display_name: user.name,
        avatar_url: user.image,
      },
    });

    // 3. Build response with user data
    const response = NextResponse.json({
      user: data.user,
      expiresAt: data.expires_at,
    });

    // 4. Set Go session cookie
    response.cookies.set("session", data.token, {
      httpOnly: true,
      secure: process.env.NODE_ENV === "production",
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

    console.error("[OIDC Bridge] Unexpected error:", err);
    return NextResponse.json(
      {
        error: "Failed to bridge OIDC session to backend",
        code: "bridge_error",
      },
      { status: 500 },
    );
  }
}
