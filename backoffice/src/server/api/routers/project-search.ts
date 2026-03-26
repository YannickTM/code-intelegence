import { z } from "zod";
import { TRPCError } from "@trpc/server";
import { createTRPCRouter, protectedProcedure } from "~/server/api/trpc";
import {
  apiCall,
  ApiError,
  mapHttpStatusToTRPCCode,
} from "~/server/api-client";

// ── Response types (mirrors Go backend) ─────────────────────────────────────

type CodeSearchMatch = {
  chunk_id: string;
  file_path: string;
  language: string | null;
  start_line: number;
  end_line: number;
  content: string;
  match_count: number;
};

type CodeSearchResponse = {
  items: CodeSearchMatch[];
  total: number;
  snapshot_id: string;
  limit: number;
  offset: number;
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
};

type SymbolFlags = {
  is_exported?: boolean;
  is_default_export?: boolean;
  is_async?: boolean;
  is_generator?: boolean;
  is_static?: boolean;
  is_abstract?: boolean;
  is_readonly?: boolean;
  is_optional?: boolean;
  is_arrow_function?: boolean;
  is_react_component_like?: boolean;
  is_hook_like?: boolean;
};

type Symbol = {
  id: string;
  name: string;
  qualified_name?: string;
  kind: string;
  signature?: string;
  start_line?: number;
  end_line?: number;
  doc_text?: string;
  file_path: string;
  language?: string;
  flags?: SymbolFlags;
  modifiers?: string[];
  return_type?: string;
  parameter_types?: string[];
};

type SymbolListResponse = {
  items: Symbol[];
  total: number;
  snapshot_id: string;
  limit: number;
  offset: number;
};

type _DependencyResponse = {
  nodes: { id: string; label: string; type: string }[];
  edges: { source: string; target: string; type: string }[];
};

// ── Router ──────────────────────────────────────────────────────────────────

export const projectSearchRouter = createTRPCRouter({
  /** POST /v1/projects/{id}/query/search → CodeSearchResponse */
  search: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        query: z.string().min(1).max(1000),
        searchMode: z
          .enum(["insensitive", "sensitive", "regex"])
          .optional(),
        language: z.string().max(50).optional(),
        filePattern: z.string().max(500).optional(),
        includeDir: z.string().max(500).optional(),
        excludeDir: z.string().max(500).optional(),
        limit: z.number().int().min(1).max(100).default(20),
        offset: z.number().int().min(0).default(0),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const { projectId, ...body } = input;
        const requestBody = {
          query: body.query,
          search_mode: body.searchMode ?? "insensitive",
          language: body.language,
          file_pattern: body.filePattern,
          include_dir: body.includeDir,
          exclude_dir: body.excludeDir,
          limit: body.limit,
          offset: body.offset,
        };
        const { data } = await apiCall<CodeSearchResponse>({
          method: "POST",
          path: `/v1/projects/${projectId}/query/search`,
          body: requestBody,
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
          message: "Failed to search code",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/files/context?file_path=&line= → FileContextResponse */
  fileContext: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        file_path: z.string(),
        line: z.number().int().optional(),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const params = new URLSearchParams({ file_path: input.file_path });
        if (input.line != null) params.set("line", String(input.line));
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
          message: "Failed to fetch file context",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/files/context (metadata only — strips content + content_hash) */
  fileMetadata: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        file_path: z.string(),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const params = new URLSearchParams({ file_path: input.file_path });
        const { data } = await apiCall<FileContextResponse>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/files/context?${params}`,
          headers: ctx.headers,
        });
        const { content: _content, content_hash: _hash, ...metadata } = data;
        return metadata;
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
          message: "Failed to fetch file metadata",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/symbols → SymbolListResponse */
  listSymbols: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        name: z.string().optional(),
        kind: z.string().optional(),
        searchMode: z
          .enum(["insensitive", "sensitive", "regex"])
          .optional(),
        includeDir: z.string().max(500).optional(),
        excludeDir: z.string().max(500).optional(),
        limit: z.number().int().min(1).max(200).default(50),
        offset: z.number().int().min(0).default(0),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const params = new URLSearchParams();
        if (input.name) params.set("name", input.name);
        if (input.kind) params.set("kind", input.kind);
        if (input.searchMode && input.searchMode !== "insensitive") {
          params.set("search_mode", input.searchMode);
        }
        if (input.includeDir) params.set("include_dir", input.includeDir);
        if (input.excludeDir) params.set("exclude_dir", input.excludeDir);
        params.set("limit", String(input.limit));
        params.set("offset", String(input.offset));
        const { data } = await apiCall<SymbolListResponse>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/symbols?${params}`,
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
          message: "Failed to list symbols",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/symbols/{symbol_id} → Symbol */
  getSymbol: protectedProcedure
    .input(
      z.object({
        projectId: z.uuid(),
        symbolId: z.string().uuid(),
      }),
    )
    .query(async ({ ctx, input }) => {
      try {
        const { data } = await apiCall<Symbol>({
          method: "GET",
          path: `/v1/projects/${input.projectId}/symbols/${input.symbolId}`,
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
          message: "Failed to fetch symbol",
          cause: error,
        });
      }
    }),

  /** GET /v1/projects/{id}/dependencies → DependencyResponse */
  dependencies: protectedProcedure
    .input(z.object({ projectId: z.uuid() }))
    .query(async ({ ctx: _ctx, input: _input }) => {
      // TODO: implement
      // const { data } = await apiCall<DependencyResponse>({
      //   method: "GET",
      //   path: `/v1/projects/${input.projectId}/dependencies`,
      //   headers: ctx.headers,
      // });
      // return data;
      throw new TRPCError({
        code: "NOT_IMPLEMENTED",
        message: "projectSearch.dependencies not implemented",
      });
    }),
});
