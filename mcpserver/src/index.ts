import http, { IncomingMessage, ServerResponse } from "node:http";
import { TOOL_BY_NAME, TOOL_DEFINITIONS } from "./tools.js";

const transport = process.env.MCP_TRANSPORT ?? "sse";
const port = Number(process.env.MCP_HTTP_PORT ?? 3000);
const backendBaseURL = (process.env.BACKEND_BASE_URL ?? "http://backend-api:8080").replace(/\/+$/, "");
const timeoutMs = Number(process.env.MCP_REQUEST_TIMEOUT_MS ?? 10000);
const defaultProjectID = process.env.MCP_DEFAULT_PROJECT_ID ?? "";

type JsonMap = Record<string, unknown>;

function sendJSON(res: ServerResponse, status: number, payload: unknown): void {
  const body = JSON.stringify(payload);
  res.writeHead(status, {
    "content-type": "application/json",
    "content-length": Buffer.byteLength(body)
  });
  res.end(body);
}

async function readJSON(req: IncomingMessage): Promise<JsonMap> {
  const chunks: string[] = [];
  for await (const chunk of req) {
    chunks.push(typeof chunk === "string" ? chunk : chunk.toString("utf8"));
  }
  const raw = chunks.join("").trim();
  if (!raw) return {};
  return JSON.parse(raw) as JsonMap;
}

function healthPayload(): JsonMap {
  return {
    status: "ok",
    service: "mcp-server",
    transport,
    timestamp: new Date().toISOString()
  };
}

function resolveProjectID(input: JsonMap): string {
  const projectID = String(input.project_id ?? "").trim();
  if (projectID !== "") return projectID;
  return defaultProjectID;
}

async function invokeBackend(toolName: string, input: JsonMap, authHeader: string): Promise<unknown> {
  const tool = TOOL_BY_NAME.get(toolName);
  if (!tool) {
    throw new Error(`unknown tool: ${toolName}`);
  }

  const projectID = resolveProjectID(input);
  if (!projectID) {
    throw new Error("project_id is required");
  }

  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);
  try {
    if (toolName === "search_code") {
      const path = `/v1/projects/${projectID}/query/search`;
      const response = await fetch(`${backendBaseURL}${path}`, {
        method: "POST",
        headers: {
          "content-type": "application/json",
          authorization: authHeader
        },
        body: JSON.stringify({
          query: input.query,
          language: input.language,
          symbol_type: input.symbol_type,
          file_pattern: input.file_pattern,
          limit: input.limit
        }),
        signal: controller.signal
      });
      return await response.json();
    }

    if (toolName === "get_symbol_info") {
      if (input.symbol_id) {
        const path = `/v1/projects/${projectID}/symbols/${String(input.symbol_id)}`;
        const response = await fetch(`${backendBaseURL}${path}`, {
          method: "GET",
          headers: { authorization: authHeader },
          signal: controller.signal
        });
        return await response.json();
      }

      const name = String(input.symbol_name ?? "");
      const query = name ? `?name=${encodeURIComponent(name)}` : "";
      const path = `/v1/projects/${projectID}/symbols${query}`;
      const response = await fetch(`${backendBaseURL}${path}`, {
        method: "GET",
        headers: { authorization: authHeader },
        signal: controller.signal
      });
      return await response.json();
    }

    if (toolName === "get_dependencies") {
      const path = `/v1/projects/${projectID}/dependencies`;
      const response = await fetch(`${backendBaseURL}${path}`, {
        method: "GET",
        headers: { authorization: authHeader },
        signal: controller.signal
      });
      return await response.json();
    }

    if (toolName === "get_file_context") {
      const filePath = String(input.file_path ?? "");
      const lineParam = input.line ? `&line=${encodeURIComponent(String(input.line))}` : "";
      const path = `/v1/projects/${projectID}/files/context?file_path=${encodeURIComponent(filePath)}${lineParam}`;
      const response = await fetch(`${backendBaseURL}${path}`, {
        method: "GET",
        headers: { authorization: authHeader },
        signal: controller.signal
      });
      return await response.json();
    }

    if (toolName === "get_project_structure") {
      const path = `/v1/projects/${projectID}/structure`;
      const response = await fetch(`${backendBaseURL}${path}`, {
        method: "GET",
        headers: { authorization: authHeader },
        signal: controller.signal
      });
      return await response.json();
    }

    if (toolName === "get_conventions") {
      const path = `/v1/projects/${projectID}/conventions`;
      const response = await fetch(`${backendBaseURL}${path}`, {
        method: "GET",
        headers: { authorization: authHeader },
        signal: controller.signal
      });
      return await response.json();
    }

    throw new Error(`tool not wired: ${toolName}`);
  } finally {
    clearTimeout(timeout);
  }
}

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url ?? "/", "http://localhost");
  const path = url.pathname;

  if (req.method === "GET" && path === "/health/live") {
    return sendJSON(res, 200, healthPayload());
  }

  if (req.method === "GET" && path === "/health/ready") {
    return sendJSON(res, 200, healthPayload());
  }

  if (req.method === "GET" && path === "/tools") {
    return sendJSON(res, 200, { version: "1.0.0", tools: TOOL_DEFINITIONS });
  }

  if (req.method === "POST" && path.startsWith("/tools/")) {
    const toolName = path.slice("/tools/".length);
    const authHeader = req.headers.authorization ?? "";

    try {
      const input = await readJSON(req);
      const data = await invokeBackend(toolName, input, authHeader);
      return sendJSON(res, 200, { tool: toolName, data });
    } catch (error) {
      return sendJSON(res, 400, {
        error: "tool_invocation_failed",
        message: String(error)
      });
    }
  }

  return sendJSON(res, 404, { error: "not_found" });
});

server.listen(port, () => {
  console.log(`mcp-server listening on :${port} (transport=${transport})`);
});
