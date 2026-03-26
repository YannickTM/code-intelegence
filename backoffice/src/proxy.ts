import { type NextRequest, NextResponse } from "next/server";

const publicPaths = ["/login", "/api/auth"];

export function proxy(req: NextRequest) {
  const { pathname } = req.nextUrl;

  // Allow public paths, tRPC, static files
  if (
    publicPaths.some((p) => pathname.startsWith(p)) ||
    pathname.startsWith("/api/trpc") ||
    pathname.startsWith("/_next") ||
    pathname.startsWith("/favicon")
  ) {
    return NextResponse.next();
  }

  // Check for session cookie
  const session = req.cookies.get("session");

  if (!session?.value) {
    const loginUrl = new URL("/login", req.url);
    return NextResponse.redirect(loginUrl);
  }

  return NextResponse.next();
}

export const config = {
  matcher: [
    "/((?!_next/static|_next/image|favicon.ico|.*\\.(?:svg|png|jpg|jpeg|gif|webp|ico|css|js|map|txt|robots\\.txt|sitemap\\.xml)$).*)",
  ],
};
