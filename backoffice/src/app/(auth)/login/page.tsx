import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { LoginForm } from "~/components/auth/login-form";

export const metadata = {
  title: "Login - MYJUNGLE Backoffice",
};

type SearchParams = Promise<{
  oidc?: string;
  error?: string;
  expired?: string;
}>;

export default async function LoginPage({
  searchParams,
}: {
  searchParams: SearchParams;
}) {
  const cookieStore = await cookies();
  const session = cookieStore.get("session");
  const params = await searchParams;

  // If the session cookie exists but was flagged as invalid (e.g. user
  // deactivated, session expired server-side), clear it via the logout
  // route to break the redirect loop between (app)/layout → /login.
  if (session?.value && params.expired === "1") {
    redirect("/api/auth/clear-session");
  }

  if (session?.value) {
    redirect("/dashboard");
  }

  const oidcCallback = params.oidc === "success";
  const errorMessage = params.error ?? null;

  const isDev = process.env.NODE_ENV === "development";
  const oidcConfigured = !!process.env.OIDC_DISCOVERY_URL;

  return (
    <LoginForm
      oidcCallback={oidcCallback}
      errorMessage={errorMessage}
      isDev={isDev}
      oidcConfigured={oidcConfigured}
    />
  );
}
