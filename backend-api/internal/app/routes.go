package app

import (
	"net/http"

	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/handler"
	"myjungle/backend-api/internal/middleware"

	"github.com/go-chi/chi/v5"
)

// probeMethods lists the HTTP methods we check when building the Allow header
// for 405 responses.
var probeMethods = []string{
	http.MethodGet, http.MethodHead, http.MethodPost,
	http.MethodPut, http.MethodPatch, http.MethodDelete,
}

func (a *App) registerRoutes() {
	r := a.Router

	// Custom 404 returning JSON instead of chi's plain text default.
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		handler.NotFound(w)
	})

	// Health (outside /v1)
	r.Get("/health/live", a.health.HandleLive)
	r.Get("/health/ready", a.health.HandleReady)
	r.Get("/metrics", a.health.HandleMetrics)

	// API v1 — identity resolution runs only for /v1 requests.
	// Identity is resolved from session tokens (Bearer header or cookie)
	// or API keys (Bearer tokens starting with "mj_").
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.IdentityResolver(a.DB, a.Config.Session))

		// --- Public routes (no auth required) ---
		r.Post("/users", a.user.HandleRegister)
		r.Post("/auth/login", a.auth.HandleLogin)

		// --- User-only authenticated routes ---
		// These require a real user session (API keys are rejected by RequireUser).
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireUser())

			// Auth
			r.Post("/auth/logout", a.auth.HandleLogout)

			// User profile
			r.Get("/users/me", a.user.HandleGetMe)
			r.Patch("/users/me", a.user.HandleUpdateMe)
			r.Get("/users/me/projects", a.user.HandleMyProjects)
			r.Get("/users/lookup", a.user.HandleLookupUser)

			// Projects (collection)
			r.Get("/projects", a.project.HandleList)
			r.Post("/projects", a.project.HandleCreate)

			// SSH Keys (user-scoped)
			r.Get("/ssh-keys", a.sshKey.HandleList)
			r.Post("/ssh-keys", a.sshKey.HandleCreate)
			r.Route("/ssh-keys/{keyID}", func(r chi.Router) {
				r.Get("/", a.sshKey.HandleGet)
				r.Get("/projects", a.sshKey.HandleListProjects)
				r.Post("/retire", a.sshKey.HandleRetire)
			})

			// Personal API Keys
			r.Get("/users/me/keys", a.apiKey.HandleListPersonalKeys)
			r.Post("/users/me/keys", a.apiKey.HandleCreatePersonalKey)
			r.Delete("/users/me/keys/{keyID}", a.apiKey.HandleDeletePersonalKey)

			// Supported providers
			r.Get("/settings/providers", a.provider.HandleListProviders)

			// Dashboard
			r.Get("/dashboard/summary", a.dashboard.HandleSummary)

			// Events
			r.Get("/events/stream", a.event.HandleStream)
		})

		// --- Platform management routes (platform admin only) ---
		r.Route("/platform-management", func(r chi.Router) {
			r.Use(middleware.RequireUser())
			r.Use(middleware.RequirePlatformAdmin(a.DB))

			// User management
			r.Get("/users", a.admin.HandleListUsers)
			r.Get("/users/{userId}", a.admin.HandleGetUser)
			r.Patch("/users/{userId}", a.admin.HandleUpdateUser)
			r.Post("/users/{userId}/deactivate", a.admin.HandleDeactivateUser)
			r.Post("/users/{userId}/activate", a.admin.HandleActivateUser)

			// Platform role management
			r.Get("/platform-roles", a.admin.HandleListPlatformRoles)
			r.Post("/platform-roles", a.admin.HandleGrantPlatformRole)
			r.Delete("/platform-roles/{userId}/{role}", a.admin.HandleRevokePlatformRole)

			// Global settings
			r.Route("/settings/embedding", func(r chi.Router) {
				r.Get("/", a.embedding.HandleGlobalGet)
				r.Put("/", a.embedding.HandleGlobalUpdate)
				r.Post("/", a.embedding.HandleCreateGlobalConfig)
				r.Post("/test", a.embedding.HandleGlobalTest)
				r.Route("/{configId}", func(r chi.Router) {
					r.Patch("/", a.embedding.HandleUpdateGlobalConfigByID)
					r.Delete("/", a.embedding.HandleDeleteGlobalConfig)
					r.Post("/promote", a.embedding.HandlePromoteGlobalConfigToDefault)
					r.Post("/test", a.embedding.HandleTestGlobalConfigByID)
				})
			})
			r.Route("/settings/llm", func(r chi.Router) {
				r.Get("/", a.llm.HandleGlobalGet)
				r.Put("/", a.llm.HandleGlobalUpdate)
				r.Post("/", a.llm.HandleCreateGlobalConfig)
				r.Post("/test", a.llm.HandleGlobalTest)
				r.Route("/{configId}", func(r chi.Router) {
					r.Patch("/", a.llm.HandleUpdateGlobalConfigByID)
					r.Delete("/", a.llm.HandleDeleteGlobalConfig)
					r.Post("/promote", a.llm.HandlePromoteGlobalConfigToDefault)
					r.Post("/test", a.llm.HandleTestGlobalConfigByID)
				})
			})

			// Worker status (read-only, requires Redis)
			if a.worker != nil {
				r.Get("/workers", a.worker.HandleListWorkers)
			}
		})

		// --- Project-scoped routes ---
		// Some routes support dual-auth (user session OR API key),
		// others are user-only. RequireProjectRole handles both identity types.
		// User-only routes use r.With(middleware.RequireUser()) inline.
		r.Route("/projects/{projectID}", func(r chi.Router) {
			// Shorthand middleware stacks.
			memberAccess := middleware.RequireProjectRole(a.DB, domain.RoleMember)
			adminAccess := middleware.RequireProjectRole(a.DB, domain.RoleAdmin)
			ownerAccess := middleware.RequireProjectRole(a.DB, domain.RoleOwner)
			userOnly := middleware.RequireUser()

			// --- Dual-auth: data-access routes (user session OR API key) ---

			// member-level data access (read)
			r.Group(func(r chi.Router) {
				r.Use(memberAccess)
				r.Get("/", a.project.HandleGet)
				r.Get("/ssh-key", a.project.HandleGetSSHKey)
				r.Get("/members", a.membership.HandleList)

				// SSE log stream (stub — keepalive only)
				r.Get("/logs/stream", a.event.HandleLogStream)

				// Indexing jobs (read)
				r.Get("/jobs", a.project.HandleListJobs)

				// Query
				r.Post("/query/search", a.project.HandleSearch)

				// Symbols
				r.Get("/symbols", a.project.HandleListSymbols)
				r.Get("/symbols/{symbolID}", a.project.HandleGetSymbol)

				// Dependencies & structure
				r.Get("/dependencies", a.project.HandleDependencies)
				r.Get("/dependencies/graph", a.project.HandleDependencyGraph)
				r.Get("/structure", a.project.HandleStructure)

				// Files & conventions
				r.Get("/files/context", a.project.HandleFileContext)
				r.Get("/files/dependencies", a.project.HandleFileDependencies)
				r.Get("/files/exports", a.project.HandleFileExports)
				r.Get("/files/references", a.project.HandleFileReferences)
				r.Get("/files/jsx-usages", a.project.HandleFileJsxUsages)
				r.Get("/files/network-calls", a.project.HandleFileNetworkCalls)
				r.Get("/files/history", a.project.HandleFileHistory)
				r.Get("/conventions", a.project.HandleConventions)

				// Embedding settings (read)
				r.Get("/settings/embedding/available", a.embedding.HandleProjectAvailable)
				r.Get("/settings/embedding", a.embedding.HandleProjectGet)
				r.Get("/settings/embedding/resolved", a.embedding.HandleProjectResolved)

				// LLM settings (read)
				r.Get("/settings/llm/available", a.llm.HandleProjectAvailable)
				r.Get("/settings/llm", a.llm.HandleProjectGet)
				r.Get("/settings/llm/resolved", a.llm.HandleProjectResolved)

				// Commit history
				r.Get("/commits", a.commits.HandleList)
				r.Get("/commits/{commitHash}", a.commits.HandleGet)
				r.Get("/commits/{commitHash}/diffs", a.commits.HandleDiffs)
			})

			// admin-level data mutations (dual-auth)
			r.With(adminAccess).Post("/index", a.project.HandleIndex)

			// --- User-only: project management routes ---

			// member-level (user-only)
			r.With(userOnly, memberAccess).Delete("/members/{userID}", a.membership.HandleRemove) // allows self-removal; service enforces authorization for removing others

			// admin-level (user-only)
			r.Group(func(r chi.Router) {
				r.Use(userOnly, adminAccess)
				r.Patch("/", a.project.HandleUpdate)
				r.Put("/ssh-key", a.project.HandleSetSSHKey)
				r.Delete("/ssh-key", a.project.HandleRemoveSSHKey)
				r.Post("/members", a.membership.HandleAdd)
				r.Patch("/members/{userID}", a.membership.HandleUpdate)

				// Embedding settings (write)
				r.Put("/settings/embedding", a.embedding.HandleProjectUpdate)
				r.Delete("/settings/embedding", a.embedding.HandleProjectDelete)
				r.Post("/settings/embedding/test", a.embedding.HandleProjectTest)

				// LLM settings (write)
				r.Put("/settings/llm", a.llm.HandleProjectUpdate)
				r.Delete("/settings/llm", a.llm.HandleProjectDelete)
				r.Post("/settings/llm/test", a.llm.HandleProjectTest)

				// Project API Keys (list + manage)
				r.Get("/keys", a.apiKey.HandleListProjectKeys)
				r.Post("/keys", a.apiKey.HandleCreateProjectKey)
				r.Delete("/keys/{keyID}", a.apiKey.HandleDeleteProjectKey)
			})

			// owner-level (user-only)
			r.With(userOnly, ownerAccess).Delete("/", a.project.HandleDelete)
		})
	})

	// Custom 405 returning JSON with an Allow header (RFC 7231 §6.5.5).
	// We probe the router with each method to find which ones are supported
	// for the requested path, then unconditionally include OPTIONS because
	// the CORS middleware handles it for all paths before routing.
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		var allowed []string
		for _, m := range probeMethods {
			rctx := chi.NewRouteContext()
			if a.Router.Match(rctx, m, req.URL.Path) {
				allowed = append(allowed, m)
			}
		}
		allowed = append(allowed, http.MethodOptions)
		handler.MethodNotAllowed(w, allowed...)
	})
}
