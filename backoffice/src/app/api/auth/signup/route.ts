import { type NextRequest, NextResponse } from "next/server";
import { apiCall, ApiError } from "~/server/api-client";

type RegisterResponse = {
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

type LoginResponse = {
  token: string;
  expires_at: string;
  user: RegisterResponse["user"];
};

/**
 * POST /api/auth/signup
 *
 * Dev-only user registration. Creates a user via the Go backend,
 * then immediately logs them in and sets the session cookie.
 */
export async function POST(req: NextRequest) {
  if (process.env.NODE_ENV !== "development") {
    return NextResponse.json(
      { error: "Registration is disabled.", code: "disabled" },
      { status: 403 },
    );
  }

  try {
    const body = (await req.json().catch(() => null)) as {
      username?: unknown;
      email?: unknown;
      display_name?: unknown;
    } | null;

    if (!body || typeof body.username !== "string" || !body.username.trim()) {
      return NextResponse.json(
        { error: "username is required", code: "validation_error" },
        { status: 400 },
      );
    }

    if (!body.email || typeof body.email !== "string" || !body.email.trim()) {
      return NextResponse.json(
        { error: "email is required", code: "validation_error" },
        { status: 400 },
      );
    }

    if (!/^[^\s@]+@[^\s@]+$/.test(body.email.trim())) {
      return NextResponse.json(
        { error: "email must be a valid email address", code: "validation_error" },
        { status: 400 },
      );
    }

    const registerBody: {
      username: string;
      email: string;
      display_name?: string;
    } = {
      username: body.username.trim(),
      email: body.email.trim(),
    };
    if (typeof body.display_name === "string" && body.display_name.trim()) {
      registerBody.display_name = body.display_name.trim();
    }

    // Step 1: Create user (ignore 409 — user may already exist from a prior partial attempt)
    try {
      await apiCall<RegisterResponse>({
        method: "POST",
        path: "/v1/users",
        body: registerBody,
      });
    } catch (err) {
      if (!(err instanceof ApiError && err.status === 409)) {
        throw err;
      }
    }

    // Step 2: Log in as the new user
    const { data: loginData } = await apiCall<LoginResponse>({
      method: "POST",
      path: "/v1/auth/login",
      body: { username: registerBody.username },
    });

    const response = NextResponse.json({
      user: loginData.user,
      expiresAt: loginData.expires_at,
    });

    response.cookies.set("session", loginData.token, {
      httpOnly: true,
      secure: false,
      path: "/",
      expires: new Date(loginData.expires_at),
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
      { error: "Internal server error", code: "internal_server_error" },
      { status: 500 },
    );
  }
}
