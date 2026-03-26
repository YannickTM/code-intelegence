package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/config"
	"myjungle/backend-api/internal/storage/postgres"

	db "myjungle/datastore/postgres/sqlc"
)

func TestIdentityResolver_NilDB(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, ok := auth.UserFromContext(r.Context())
		if ok {
			t.Error("expected no user in context when DB is nil")
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := IdentityResolver(nil, config.SessionConfig{CookieName: "session"})(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("next handler was not called")
	}
}

func TestIdentityResolver_NoAuth(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, ok := auth.UserFromContext(r.Context())
		if ok {
			t.Error("expected no user in context for unauthenticated request")
		}
	})

	// Use a non-nil DB with non-nil Queries so the test exercises the
	// no-auth branch, not the nil-DB/nil-Queries shortcut.
	fakeDB := &postgres.DB{Queries: db.New(nil)}
	handler := IdentityResolver(fakeDB, config.SessionConfig{CookieName: "session"})(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("next handler was not called")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestIdentityResolver_EmptyBearerToken(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	})

	fakeDB := &postgres.DB{Queries: db.New(nil)}
	handler := IdentityResolver(fakeDB, config.SessionConfig{CookieName: "session"})(next)

	// "Bearer " with trailing space but no token value → should return 401.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Error("next handler should not be called for empty Bearer token")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestIdentityResolver_BearerOnly(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	})

	fakeDB := &postgres.DB{Queries: db.New(nil)}
	handler := IdentityResolver(fakeDB, config.SessionConfig{CookieName: "session"})(next)

	// "Bearer" with no space or token → should return 401.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Error("next handler should not be called for bare Bearer scheme")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
