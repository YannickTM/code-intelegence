"use client";

import { useSSEConnection } from "~/hooks/use-sse-connection";
import { api } from "~/trpc/react";

export function AppShellClient() {
  const utils = api.useUtils();
  useSSEConnection(utils);
  return null;
}
