import { env } from "~/env";

/**
 * GET /api/events/stream
 *
 * Proxies the Go backend SSE endpoint: GET /v1/events/stream
 * Streams real-time events (job lifecycle, membership changes) to the browser.
 */
export async function GET(request: Request) {
  const backendUrl = `${env.API_BASE_URL}/v1/events/stream`;

  let backendRes: Response;
  try {
    backendRes = await fetch(backendUrl, {
      headers: { Cookie: request.headers.get("cookie") ?? "" },
      signal: request.signal,
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") {
      return new Response(null, { status: 499 });
    }
    return new Response("Backend unavailable", { status: 502 });
  }

  if (!backendRes.ok) {
    return new Response(backendRes.statusText, { status: backendRes.status });
  }

  return new Response(backendRes.body, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
    },
  });
}
