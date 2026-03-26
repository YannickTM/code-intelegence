"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "~/components/ui/button";
import { Input } from "~/components/ui/input";
import { Label } from "~/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
import { Alert, AlertDescription } from "~/components/ui/alert";
import { AlertCircle, Loader2, LogIn, User, UserPlus } from "lucide-react";
import { authClient } from "~/lib/auth-client";
import { Logo } from "~/components/logo";

type LoginFormProps = {
  oidcCallback?: boolean;
  errorMessage?: string | null;
  isDev?: boolean;
  oidcConfigured?: boolean;
};

export function LoginForm({
  oidcCallback = false,
  errorMessage,
  isDev = false,
  oidcConfigured = false,
}: LoginFormProps) {
  const router = useRouter();
  const [error, setError] = useState<string | null>(errorMessage ?? null);
  const [isLoading, setIsLoading] = useState(false);
  const [isBridging, setIsBridging] = useState(false);
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [isDevLoading, setIsDevLoading] = useState(false);
  const [mode, setMode] = useState<"signin" | "signup">("signin");

  const providerId = process.env.NEXT_PUBLIC_OIDC_PROVIDER_ID ?? "oidc";

  // When returning from OIDC callback, bridge the session to Go backend
  useEffect(() => {
    if (!oidcCallback) return;

    let cancelled = false;

    async function bridgeSession() {
      setIsBridging(true);
      setError(null);

      try {
        const res = await fetch("/api/auth/oidc/bridge", {
          method: "POST",
          credentials: "include",
        });

        if (!res.ok) {
          const data = (await res.json().catch(() => ({}))) as {
            error?: string;
          };
          if (!cancelled) {
            setError(data.error ?? "Failed to complete sign-in");
            setIsBridging(false);
          }
          return;
        }

        if (!cancelled) {
          router.push("/dashboard");
          router.refresh();
        }
      } catch {
        if (!cancelled) {
          setError("Network error during sign-in. Is the backend running?");
          setIsBridging(false);
        }
      }
    }

    void bridgeSession();

    return () => {
      cancelled = true;
    };
  }, [oidcCallback, router]);

  async function handleDevLogin(e: React.FormEvent) {
    e.preventDefault();
    if (!username.trim()) return;

    setError(null);
    setIsDevLoading(true);

    try {
      const res = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: username.trim() }),
        credentials: "include",
      });

      if (!res.ok) {
        const data = (await res.json().catch(() => ({}))) as { error?: string };
        setError(data.error ?? "Login failed");
        setIsDevLoading(false);
        return;
      }

      router.push("/dashboard");
      router.refresh();
    } catch {
      setError("Network error. Is the backend running?");
      setIsDevLoading(false);
    }
  }

  async function handleDevSignup(e: React.FormEvent) {
    e.preventDefault();
    if (!username.trim()) return;

    setError(null);
    setIsDevLoading(true);

    try {
      const body: { username: string; email: string; display_name?: string } = {
        username: username.trim(),
        email: email.trim(),
      };
      if (displayName.trim()) {
        body.display_name = displayName.trim();
      }

      const res = await fetch("/api/auth/signup", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
        credentials: "include",
      });

      if (!res.ok) {
        const data = (await res.json().catch(() => ({}))) as { error?: string };
        setError(data.error ?? "Signup failed");
        setIsDevLoading(false);
        return;
      }

      router.push("/dashboard");
      router.refresh();
    } catch {
      setError("Network error. Is the backend running?");
      setIsDevLoading(false);
    }
  }

  async function handleSSOLogin() {
    setError(null);
    setIsLoading(true);

    try {
      await authClient.signIn.oauth2({
        providerId,
        callbackURL: "/login?oidc=success",
      });
    } catch {
      setError("Failed to start SSO login. Please try again.");
      setIsLoading(false);
    }
  }

  function toggleMode() {
    setMode((m) => (m === "signin" ? "signup" : "signin"));
    setError(null);
  }

  // Show loading state while bridging OIDC → Go session
  if (isBridging) {
    return (
      <Card className="w-full max-w-sm shadow-lg">
        <CardHeader className="space-y-1">
          <div className="mb-2 flex justify-center">
            <Logo />
          </div>
          <CardTitle className="text-2xl font-bold">Signing in...</CardTitle>
          <CardDescription>Completing authentication</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col items-center gap-4">
          <Loader2 className="text-muted-foreground h-8 w-8 animate-spin" />
          <p className="text-muted-foreground text-sm">
            Setting up your session...
          </p>
        </CardContent>
      </Card>
    );
  }

  const isSignup = mode === "signup";

  return (
    <Card className="w-full max-w-sm shadow-lg">
      <CardHeader className="space-y-1">
        <div className="mb-2 flex justify-center">
          <Logo />
        </div>
        <CardTitle className="text-2xl font-bold">
          {isSignup ? "Sign up" : "Sign in"}
        </CardTitle>
        <CardDescription>
          {isSignup
            ? "Create a development account"
            : isDev
              ? "Development mode — sign in with a username"
              : "Sign in with your organization account"}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {isDev && (
          <form
            onSubmit={isSignup ? handleDevSignup : handleDevLogin}
            className="space-y-3"
          >
            <div className="space-y-2">
              <Label htmlFor="username">Username</Label>
              <Input
                id="username"
                placeholder="admin"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                disabled={isDevLoading}
                autoFocus
              />
            </div>
            {isSignup && (
              <div className="space-y-2">
                <Label htmlFor="email">Email</Label>
                <Input
                  id="email"
                  type="email"
                  placeholder="user@example.com"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  disabled={isDevLoading}
                />
              </div>
            )}
            {isSignup && (
              <div className="space-y-2">
                <Label htmlFor="display-name">
                  Display Name{" "}
                  <span className="text-muted-foreground">(optional)</span>
                </Label>
                <Input
                  id="display-name"
                  placeholder="John Doe"
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  disabled={isDevLoading}
                />
              </div>
            )}
            <Button
              type="submit"
              className="w-full"
              disabled={isDevLoading || !username.trim() || (isSignup && !email.trim())}
              size="lg"
            >
              {isDevLoading ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : isSignup ? (
                <UserPlus className="mr-2 h-4 w-4" />
              ) : (
                <User className="mr-2 h-4 w-4" />
              )}
              {isSignup ? "Create account" : "Sign in as dev"}
            </Button>

            <p className="text-muted-foreground text-center text-sm">
              {isSignup
                ? "Already have an account? "
                : "Don\u2019t have an account? "}
              <button
                type="button"
                onClick={toggleMode}
                className="text-primary underline-offset-4 hover:underline"
              >
                {isSignup ? "Sign in" : "Sign up"}
              </button>
            </p>
          </form>
        )}

        {isDev && !isSignup && (
          <div className="relative">
            <div className="absolute inset-0 flex items-center">
              <span className="w-full border-t" />
            </div>
            <div className="relative flex justify-center text-xs uppercase">
              <span className="bg-card text-muted-foreground px-2">or</span>
            </div>
          </div>
        )}

        {!isSignup && (
          <>
            <Button
              onClick={handleSSOLogin}
              className="w-full"
              variant={isDev ? "outline" : "default"}
              disabled={isLoading || !oidcConfigured}
              size="lg"
            >
              {isLoading ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <LogIn className="mr-2 h-4 w-4" />
              )}
              {oidcConfigured ? "Sign in with SSO" : "SSO not configured"}
            </Button>

            {oidcConfigured && (
              <p className="text-muted-foreground text-center text-xs">
                You will be redirected to your identity provider
              </p>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
