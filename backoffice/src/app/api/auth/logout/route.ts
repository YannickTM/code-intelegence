import { type NextRequest, NextResponse } from "next/server";
import { apiCall } from "~/server/api-client";

export async function POST(req: NextRequest) {
  try {
    await apiCall({
      method: "POST",
      path: "/v1/auth/logout",
      headers: req.headers,
    });
  } catch {
    // Backend call failed — still clear local cookie below
  }

  const response = NextResponse.json({ success: true });
  response.cookies.set("session", "", { maxAge: 0, path: "/" });
  return response;
}
