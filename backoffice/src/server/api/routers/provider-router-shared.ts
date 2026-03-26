import { TRPCError } from "@trpc/server";
import { z } from "zod";
import {
  ApiError,
  mapHttpStatusToTRPCCode,
} from "~/server/api-client";
import { isValidProviderEndpointURL } from "~/lib/provider-endpoint-url";

export const providerEndpointUrlSchema = z
  .string()
  .trim()
  .min(1)
  .refine(isValidProviderEndpointURL, {
    error:
      "endpoint_url must be an http/https URL without credentials, query, or fragment",
  });

export function removeUndefinedFields<T extends Record<string, unknown>>(
  value: T,
): Partial<T> | undefined {
  const entries = Object.entries(value).filter(
    ([, entryValue]) => entryValue !== undefined,
  );
  if (entries.length === 0) {
    return undefined;
  }
  return Object.fromEntries(entries) as Partial<T>;
}

export function throwApiErrorAsTRPC(
  error: unknown,
  defaultMessage: string,
): never {
  if (error instanceof ApiError) {
    throw new TRPCError({
      code: mapHttpStatusToTRPCCode(error.status),
      message: error.message,
      cause: error,
    });
  }
  throw new TRPCError({
    code: "INTERNAL_SERVER_ERROR",
    message: defaultMessage,
    cause: error,
  });
}
