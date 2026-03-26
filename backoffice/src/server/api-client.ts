import "server-only";
import { env } from "~/env";

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public details?: Record<string, unknown>,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

type RequestOptions = {
  method: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  path: string;
  body?: unknown;
  /** Forward cookies from the incoming request */
  headers?: Headers;
  /** Explicit Bearer token (overrides cookie forwarding) */
  token?: string;
};

/**
 * Call the Go backend API. Returns the parsed JSON response
 * and any Set-Cookie headers from the backend (for forwarding to browser).
 *
 * Throws ApiError on non-2xx responses.
 */
export async function apiCall<T = unknown>(
  opts: RequestOptions,
): Promise<{ data: T; setCookieHeaders: string[] }> {
  const url = `${env.API_BASE_URL}${opts.path}`;

  const fetchHeaders: Record<string, string> = {
    "Content-Type": "application/json",
  };

  // Forward session cookie from the incoming request
  if (opts.headers) {
    const cookie = opts.headers.get("cookie");
    if (cookie) {
      fetchHeaders.Cookie = cookie;
    }
  }

  // Or use explicit Bearer token
  if (opts.token) {
    fetchHeaders.Authorization = `Bearer ${opts.token}`;
  }

  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 15_000);

  let res: Response;
  try {
    res = await fetch(url, {
      method: opts.method,
      headers: fetchHeaders,
      body: opts.body ? JSON.stringify(opts.body) : undefined,
      signal: controller.signal,
    });
  } catch (err) {
    clearTimeout(timeout);
    if (err instanceof DOMException && err.name === "AbortError") {
      throw new ApiError(504, "timeout", "Backend request timed out");
    }
    throw err;
  }
  clearTimeout(timeout);

  // Collect Set-Cookie headers from the Go backend response
  const setCookieHeaders: string[] = [];
  res.headers.forEach((value, key) => {
    if (key.toLowerCase() === "set-cookie") {
      setCookieHeaders.push(value);
    }
  });

  if (!res.ok) {
    const err = (await res.json().catch(() => ({
      error: "unknown",
      code: "unknown",
    }))) as {
      error?: string;
      code?: string;
      details?: Record<string, unknown>;
    };
    throw new ApiError(
      res.status,
      err.code ?? "unknown",
      err.error ?? res.statusText,
      err.details,
    );
  }

  // 204 No Content
  if (res.status === 204) {
    return { data: undefined as T, setCookieHeaders };
  }

  const data = (await res.json()) as T;
  return { data, setCookieHeaders };
}

/**
 * Map an HTTP status code to a tRPC error code.
 * Use in catch blocks when converting ApiError → TRPCError.
 */
export function mapHttpStatusToTRPCCode(status: number) {
  switch (status) {
    case 400:
      return "BAD_REQUEST" as const;
    case 401:
      return "UNAUTHORIZED" as const;
    case 403:
      return "FORBIDDEN" as const;
    case 404:
      return "NOT_FOUND" as const;
    case 409:
      return "CONFLICT" as const;
    case 422:
      return "UNPROCESSABLE_CONTENT" as const;
    case 429:
      return "TOO_MANY_REQUESTS" as const;
    default:
      return "INTERNAL_SERVER_ERROR" as const;
  }
}
