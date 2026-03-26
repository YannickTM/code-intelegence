import { betterAuth } from "better-auth";
import { genericOAuth } from "better-auth/plugins";
import { nextCookies } from "better-auth/next-js";
import Database from "better-sqlite3";
import { env } from "~/env";

// Validate OIDC config is all-or-none — fail fast on partial configuration
const oidcVars = [
  env.OIDC_DISCOVERY_URL,
  env.OIDC_CLIENT_ID,
  env.OIDC_CLIENT_SECRET,
];
const oidcSet = oidcVars.filter(Boolean).length;
if (oidcSet > 0 && oidcSet < 3) {
  throw new Error(
    "Partial OIDC configuration: OIDC_DISCOVERY_URL, OIDC_CLIENT_ID, and OIDC_CLIENT_SECRET must all be set or all be unset.",
  );
}

/**
 * BetterAuth server instance.
 *
 * We use BetterAuth **only** for the OIDC login flow (redirect → callback → id_token).
 * Once BetterAuth has the OIDC identity, the bridge endpoint (`/api/auth/oidc/bridge`)
 * calls Go backend `POST /v1/auth/login` to create a real Go session.
 *
 * The Go backend owns sessions and user records — BetterAuth sessions are ephemeral
 * and stored in an in-memory SQLite database (destroyed on restart, which is fine).
 */
export const auth = betterAuth({
  secret: env.BETTER_AUTH_SECRET,
  baseURL: env.BETTER_AUTH_URL,

  // In-memory SQLite — BetterAuth needs *some* DB but we don't persist sessions
  database: new Database(":memory:"),

  plugins: [
    ...(env.OIDC_DISCOVERY_URL && env.OIDC_CLIENT_ID && env.OIDC_CLIENT_SECRET
      ? [
          genericOAuth({
            config: [
              {
                providerId: "oidc",
                discoveryUrl: env.OIDC_DISCOVERY_URL,
                clientId: env.OIDC_CLIENT_ID,
                clientSecret: env.OIDC_CLIENT_SECRET,
                scopes: ["openid", "profile", "email"],
                pkce: true,
              },
            ],
          }),
        ]
      : []),
    nextCookies(), // reads/writes cookies via next/headers
  ],
});
