import { authRouter } from "~/server/api/routers/auth";
import { dashboardRouter } from "~/server/api/routers/dashboard";
import { projectEmbeddingRouter } from "~/server/api/routers/project-embedding";
import { projectCommitsRouter } from "~/server/api/routers/project-commits";
import { projectIndexingRouter } from "~/server/api/routers/project-indexing";
import { projectKeysRouter } from "~/server/api/routers/project-keys";
import { projectLLMRouter } from "~/server/api/routers/project-llm";
import { projectMembersRouter } from "~/server/api/routers/project-members";
import { projectFilesRouter } from "~/server/api/routers/project-files";
import { projectSearchRouter } from "~/server/api/routers/project-search";
import { projectsRouter } from "~/server/api/routers/projects";
import { providersRouter } from "~/server/api/routers/providers";
import { sshKeysRouter } from "~/server/api/routers/ssh-keys";
import { platformEmbeddingRouter } from "~/server/api/routers/platform-embedding";
import { platformLLMRouter } from "~/server/api/routers/platform-llm";
import { platformUsersRouter } from "~/server/api/routers/platform-users";
import { platformWorkersRouter } from "~/server/api/routers/platform-workers";
import { usersRouter } from "~/server/api/routers/users";
import { createCallerFactory, createTRPCRouter } from "~/server/api/trpc";

/**
 * This is the primary router for your server.
 *
 * All routers added in /api/routers should be manually added here.
 */
export const appRouter = createTRPCRouter({
  auth: authRouter,
  dashboard: dashboardRouter,
  projects: projectsRouter,
  projectMembers: projectMembersRouter,
  providers: providersRouter,
  projectEmbedding: projectEmbeddingRouter,
  projectLLM: projectLLMRouter,
  projectKeys: projectKeysRouter,
  projectCommits: projectCommitsRouter,
  projectFiles: projectFilesRouter,
  projectIndexing: projectIndexingRouter,
  projectSearch: projectSearchRouter,
  users: usersRouter,
  sshKeys: sshKeysRouter,
  platformEmbedding: platformEmbeddingRouter,
  platformLLM: platformLLMRouter,
  platformUsers: platformUsersRouter,
  platformWorkers: platformWorkersRouter,
});

// export type definition of API
export type AppRouter = typeof appRouter;

/**
 * Create a server-side caller for the tRPC API.
 * @example
 * const trpc = createCaller(createContext);
 * const res = await trpc.auth.me();
 */
export const createCaller = createCallerFactory(appRouter);
