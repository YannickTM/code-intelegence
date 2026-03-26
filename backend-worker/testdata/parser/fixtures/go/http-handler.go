// Package handler provides HTTP handlers for the user service.
package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

// User represents a user record in the system.
type User struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// UserStore defines the interface for user persistence.
type UserStore interface {
	FindAll() ([]User, error)
	FindByID(id int) (*User, error)
	Create(u *User) error
	Delete(id int) error
}

// UserHandler holds dependencies for user HTTP handlers.
type UserHandler struct {
	store  UserStore
	logger *log.Logger
}

// NewUserHandler creates a UserHandler with the given store and logger.
func NewUserHandler(store UserStore, logger *log.Logger) *UserHandler {
	return &UserHandler{store: store, logger: logger}
}

// RegisterRoutes mounts all user routes onto the provided router.
func (h *UserHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/api/users", h.listUsers).Methods(http.MethodGet)
	r.HandleFunc("/api/users/{id:[0-9]+}", h.getUser).Methods(http.MethodGet)
	r.HandleFunc("/api/users", h.createUser).Methods(http.MethodPost)
	r.HandleFunc("/api/users/{id:[0-9]+}", h.deleteUser).Methods(http.MethodDelete)
}

func (h *UserHandler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.FindAll()
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	h.respondJSON(w, http.StatusOK, users)
}

func (h *UserHandler) getUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	user, err := h.store.FindByID(id)
	if err != nil {
		h.respondError(w, http.StatusNotFound, fmt.Sprintf("user %d not found", id))
		return
	}
	h.respondJSON(w, http.StatusOK, user)
}

func (h *UserHandler) createUser(w http.ResponseWriter, r *http.Request) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user.CreatedAt = time.Now().UTC()
	if err := h.store.Create(&user); err != nil {
		h.respondError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	h.respondJSON(w, http.StatusCreated, user)
}

func (h *UserHandler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.store.Delete(id); err != nil {
		h.respondError(w, http.StatusNotFound, fmt.Sprintf("user %d not found", id))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// respondJSON writes a JSON response with the given status code.
func (h *UserHandler) respondJSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Printf("failed to encode response: %v", err)
	}
}

func (h *UserHandler) respondError(w http.ResponseWriter, code int, message string) {
	h.respondJSON(w, code, map[string]string{"error": message})
}

// HealthCheck returns a simple health check handler for readiness probes.
func HealthCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := http.Get("http://localhost:8080/api/users")
		if err != nil {
			http.Error(w, "unhealthy", http.StatusServiceUnavailable)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	}
}
