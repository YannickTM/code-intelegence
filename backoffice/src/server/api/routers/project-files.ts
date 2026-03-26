import { z } from "zod";
import { TRPCError } from "@trpc/server";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import {
  apiCall,
  ApiError,
  mapHttpStatusToTRPCCode,
} from "~/server/api-client";

// ── Response types (mirrors Go backend) ─────────────────────────────────────

export type FileFacts = {
  has_jsx?: boolean;
  has_default_export?: boolean;
  has_named_exports?: boolean;
  has_top_level_side_effects?: boolean;
  has_react_hook_calls?: boolean;
  has_fetch_calls?: boolean;
  has_class_declarations?: boolean;
  has_tests?: boolean;
  has_config_patterns?: boolean;
  jsx_runtime?: string;
};

export type FileIssue = {
  code: string;
  message: string;
  line: number;
  column: number;
  severity: "info" | "warning" | "error";
};

type FileContextResponse = {
  file_path: string;
  language: string;
  size_bytes: number;
  line_count: number;
  content_hash: string;
  content: string;
  snapshot_id: string;
  last_indexed_at: string;
  file_facts?: FileFacts | null;
  issues?: FileIssue[] | null;
  parser_meta?: Record<string, unknown> | null;
  extractor_statuses?: Record<string, unknown> | null;
};

export type FileHistoryEntry = {
  diff_id: string;
  commit_hash: string;
  short_hash: string;
  author_name: string;
  committer_date: string;
  message_subject: string;
  change_type: "added" | "modified" | "deleted" | "renamed" | "copied";
  additions: number;
  deletions: number;
};

type FileHistoryResponse = {
  items: FileHistoryEntry[];
  total: number;
  limit: number;
  offset: number;
};

export type DependencyEdge = {
  id: string;
  source_file_path: string;
  target_file_path: string | null;
  import_name: string;
  import_type: string;
  package_name?: string;
  package_version?: string;
};

export type FileDependenciesResponse = {
  file_path: string;
  imports: DependencyEdge[];
  imported_by: DependencyEdge[];
  snapshot_id: string;
};

export type GraphNode = {
  file_path: string;
  language?: string;
  is_external: boolean;
  depth: number;
};

export type GraphEdge = {
  source: string;
  target: string;
  import_name: string;
  import_type: string;
  package_name?: string;
};

export type DependencyGraphResponse = {
  nodes: GraphNode[];
  edges: GraphEdge[];
  root: string;
  depth: number;
  truncated: boolean;
  snapshot_id: string;
};

export type FileExport = {
  id: string;
  export_kind: string;
  exported_name: string;
  local_name?: string;
  source_module?: string;
  line: number;
  column: number;
};

type FileExportsResponse = {
  file_path: string;
  exports: FileExport[];
  snapshot_id: string;
};

export type SymbolReference = {
  id: string;
  reference_kind: string;
  target_name: string;
  qualified_target_hint?: string;
  raw_text?: string;
  start_line: number;
  end_line: number;
  resolution_scope?: string;
  confidence?: string;
};

type FileReferencesResponse = {
  file_path: string;
  references: SymbolReference[];
  snapshot_id: string;
};

export type JsxUsage = {
  id: string;
  component_name: string;
  is_intrinsic: boolean;
  is_fragment: boolean;
  line: number;
  confidence?: string;
};

type JsxUsagesResponse = {
  file_path: string;
  jsx_usages: JsxUsage[];
  snapshot_id: string;
};

export type NetworkCall = {
  id: string;
  client_kind: string;
  method: string;
  url_literal?: string;
  url_template?: string;
  is_relative: boolean;
  start_line: number;
  confidence?: string;
};

type NetworkCallsResponse = {
  file_path: string;
  network_calls: NetworkCall[];
  snapshot_id: string;
};

// ── Router ──────────────────────────────────────────────────────────────────

export const projectFilesRouter = createTRPCRouter({
  /** GET /v1/projects/{id}/files/context?file_path=... → FileContextResponse */
  fileContent: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        filePath: z.string(),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const params = new URLSearchParams({ file_path: input.filePath });
        const { data } = await apiCall<FileContextResponse>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/files/context?${params}`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        if (error instanceof ApiError) {
          throw new TRPCError({
            code: mapHttpStatusToTRPCCode(error.status),
            message: error.message,
            cause: error,
          });
        }
        throw new TRPCError({
          code: "INTERNAL_SERVER_ERROR",
          message: "Failed to fetch file content",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/files/history?file_path=...&limit=...&offset=... → FileHistoryResponse */
  fileHistory: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        filePath: z.string(),
        limit: z.number().int().min(1).max(50).default(10),
        offset: z.number().int().min(0).default(0),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const params = new URLSearchParams({
          file_path: input.filePath,
          limit: String(input.limit),
          offset: String(input.offset),
        });
        const { data } = await apiCall<FileHistoryResponse>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/files/history?${params}`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        if (error instanceof ApiError) {
          throw new TRPCError({
            code: mapHttpStatusToTRPCCode(error.status),
            message: error.message,
            cause: error,
          });
        }
        throw new TRPCError({
          code: "INTERNAL_SERVER_ERROR",
          message: "Failed to fetch file history",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/files/dependencies?file_path=... → FileDependenciesResponse */
  fileDependencies: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        filePath: z.string(),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const params = new URLSearchParams({ file_path: input.filePath });
        const { data } = await apiCall<FileDependenciesResponse>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/files/dependencies?${params}`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        if (error instanceof ApiError) {
          throw new TRPCError({
            code: mapHttpStatusToTRPCCode(error.status),
            message: error.message,
            cause: error,
          });
        }
        throw new TRPCError({
          code: "INTERNAL_SERVER_ERROR",
          message: "Failed to fetch file dependencies",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/dependencies/graph?root=...&depth=... → DependencyGraphResponse */
  dependencyGraph: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        root: z.string(),
        depth: z.number().int().min(0).max(5).default(2),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const params = new URLSearchParams({
          root: input.root,
          depth: String(input.depth),
        });
        const { data } = await apiCall<DependencyGraphResponse>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/dependencies/graph?${params}`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        if (error instanceof ApiError) {
          throw new TRPCError({
            code: mapHttpStatusToTRPCCode(error.status),
            message: error.message,
            cause: error,
          });
        }
        throw new TRPCError({
          code: "INTERNAL_SERVER_ERROR",
          message: "Failed to fetch dependency graph",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/files/exports?file_path=... → FileExportsResponse */
  fileExports: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        filePath: z.string(),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const params = new URLSearchParams({ file_path: input.filePath });
        const { data } = await apiCall<FileExportsResponse>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/files/exports?${params}`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        if (error instanceof ApiError) {
          throw new TRPCError({
            code: mapHttpStatusToTRPCCode(error.status),
            message: error.message,
            cause: error,
          });
        }
        throw new TRPCError({
          code: "INTERNAL_SERVER_ERROR",
          message: "Failed to fetch file exports",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/files/references?file_path=... → FileReferencesResponse */
  fileReferences: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        filePath: z.string(),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const params = new URLSearchParams({ file_path: input.filePath });
        const { data } = await apiCall<FileReferencesResponse>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/files/references?${params}`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        if (error instanceof ApiError) {
          throw new TRPCError({
            code: mapHttpStatusToTRPCCode(error.status),
            message: error.message,
            cause: error,
          });
        }
        throw new TRPCError({
          code: "INTERNAL_SERVER_ERROR",
          message: "Failed to fetch file references",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/files/jsx-usages?file_path=... → JsxUsagesResponse */
  fileJsxUsages: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        filePath: z.string(),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const params = new URLSearchParams({ file_path: input.filePath });
        const { data } = await apiCall<JsxUsagesResponse>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/files/jsx-usages?${params}`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        if (error instanceof ApiError) {
          throw new TRPCError({
            code: mapHttpStatusToTRPCCode(error.status),
            message: error.message,
            cause: error,
          });
        }
        throw new TRPCError({
          code: "INTERNAL_SERVER_ERROR",
          message: "Failed to fetch JSX usages",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/files/network-calls?file_path=... → NetworkCallsResponse */
  fileNetworkCalls: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        filePath: z.string(),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const params = new URLSearchParams({ file_path: input.filePath });
        const { data } = await apiCall<NetworkCallsResponse>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/files/network-calls?${params}`,
          headers: ctx.headers,
        });
        return data;
      } catch (error) {
        if (error instanceof ApiError) {
          throw new TRPCError({
            code: mapHttpStatusToTRPCCode(error.status),
            message: error.message,
            cause: error,
          });
        }
        throw new TRPCError({
          code: "INTERNAL_SERVER_ERROR",
          message: "Failed to fetch network calls",
          cause: error,
        });
      }
    }),
});
