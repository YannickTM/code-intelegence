// Package middleware provides HTTP middleware for the application.
package middleware

import "net/http"

// NewCORS returns a CORS middleware that validates the request Origin against
// the provided allow-list. Set wildcard to true to emit Access-Control-Allow-Origin: *.
// When wildcard is false only origins present in allowedOrigins are permitted.
func NewCORS(allowedOrigins []string, wildcard bool) func(http.Handler) http.Handler {
	allow := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allow[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if wildcard {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origin != "" && allow[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")

			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
