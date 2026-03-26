import { createAuthClient } from "better-auth/client";
import { genericOAuthClient } from "better-auth/client/plugins";

/**
 * BetterAuth client — used in React components to trigger OIDC sign-in.
 *
 * Usage:
 *   import { authClient } from "~/lib/auth-client";
 *   authClient.signIn.oauth2({ providerId: "oidc", callbackURL: "/login?oidc=success" });
 */
export const authClient = createAuthClient({
  plugins: [genericOAuthClient()],
});
