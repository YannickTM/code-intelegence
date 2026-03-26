package middleware

import "net/http"

// BodyLimit returns middleware that limits the size of request bodies.
// It wraps r.Body with http.MaxBytesReader so that downstream handlers
// receive an error when the body exceeds maxBytes.
func BodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
