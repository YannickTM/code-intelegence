export type MCPToolDefinition = {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
  outputSchema: Record<string, unknown>;
  backend: {
    method: "GET" | "POST";
    path: string;
  };
};

export const TOOL_DEFINITIONS: MCPToolDefinition[] = [
  {
    name: "search_code",
    description: "Semantic code search within one project.",
    backend: { method: "POST", path: "/v1/projects/{project_id}/query/search" },
    inputSchema: {
      type: "object",
      required: ["project_id", "query"],
      properties: {
        project_id: { type: "string", format: "uuid" },
        query: { type: "string" },
        language: { type: "string", enum: ["typescript", "javascript"] },
        symbol_type: { type: "string" },
        file_pattern: { type: "string" },
        limit: { type: "integer", default: 10 }
      }
    },
    outputSchema: {
      type: "object",
      required: ["query_time_ms", "results"],
      properties: {
        query_time_ms: { type: "integer" },
        index_snapshot_id: { type: "string" },
        index_freshness_commit: { type: "string" },
        results: { type: "array" }
      }
    }
  },
  {
    name: "get_symbol_info",
    description: "Read symbol details by symbol id or name.",
    backend: { method: "GET", path: "/v1/projects/{project_id}/symbols/{symbol_id}" },
    inputSchema: {
      type: "object",
      required: ["project_id"],
      properties: {
        project_id: { type: "string", format: "uuid" },
        symbol_id: { type: "string", format: "uuid" },
        symbol_name: { type: "string" }
      }
    },
    outputSchema: {
      type: "object",
      required: ["symbol"],
      properties: {
        query_time_ms: { type: "integer" },
        index_snapshot_id: { type: "string" },
        index_freshness_commit: { type: "string" },
        symbol: { type: "object" }
      }
    }
  },
  {
    name: "get_dependencies",
    description: "Read dependency edges.",
    backend: { method: "GET", path: "/v1/projects/{project_id}/dependencies" },
    inputSchema: {
      type: "object",
      required: ["project_id"],
      properties: {
        project_id: { type: "string", format: "uuid" },
        file_path: { type: "string" },
        symbol_name: { type: "string" },
        limit: { type: "integer", default: 100 }
      }
    },
    outputSchema: {
      type: "object",
      required: ["items"],
      properties: {
        query_time_ms: { type: "integer" },
        index_snapshot_id: { type: "string" },
        index_freshness_commit: { type: "string" },
        items: { type: "array" }
      }
    }
  },
  {
    name: "get_file_context",
    description: "Read contextual code window around a file path and line.",
    backend: { method: "GET", path: "/v1/projects/{project_id}/files/context" },
    inputSchema: {
      type: "object",
      required: ["project_id", "file_path"],
      properties: {
        project_id: { type: "string", format: "uuid" },
        file_path: { type: "string" },
        line: { type: "integer", minimum: 1 },
        radius_lines: { type: "integer", default: 80 }
      }
    },
    outputSchema: {
      type: "object",
      required: ["file_path", "content"],
      properties: {
        query_time_ms: { type: "integer" },
        index_snapshot_id: { type: "string" },
        index_freshness_commit: { type: "string" },
        file_path: { type: "string" },
        start_line: { type: "integer" },
        end_line: { type: "integer" },
        content: { type: "string" }
      }
    }
  },
  {
    name: "get_project_structure",
    description: "Read file tree and module structure.",
    backend: { method: "GET", path: "/v1/projects/{project_id}/structure" },
    inputSchema: {
      type: "object",
      required: ["project_id"],
      properties: {
        project_id: { type: "string", format: "uuid" },
        depth: { type: "integer", default: 6 }
      }
    },
    outputSchema: {
      type: "object",
      required: ["root"],
      properties: {
        query_time_ms: { type: "integer" },
        index_snapshot_id: { type: "string" },
        index_freshness_commit: { type: "string" },
        root: { type: "object" }
      }
    }
  },
  {
    name: "get_conventions",
    description: "Read inferred project conventions.",
    backend: { method: "GET", path: "/v1/projects/{project_id}/conventions" },
    inputSchema: {
      type: "object",
      required: ["project_id"],
      properties: {
        project_id: { type: "string", format: "uuid" },
        language: { type: "string", enum: ["typescript", "javascript"] },
        limit: { type: "integer", default: 20 }
      }
    },
    outputSchema: {
      type: "object",
      required: ["items"],
      properties: {
        query_time_ms: { type: "integer" },
        index_snapshot_id: { type: "string" },
        index_freshness_commit: { type: "string" },
        items: { type: "array" }
      }
    }
  }
];

export const TOOL_BY_NAME = new Map(TOOL_DEFINITIONS.map((tool) => [tool.name, tool]));
