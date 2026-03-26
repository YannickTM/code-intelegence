package vectorstore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func testUUID(b byte) pgtype.UUID {
	var u pgtype.UUID
	u.Bytes[0] = b
	u.Valid = true
	return u
}

// --- CollectionName tests ---

func TestCollectionName(t *testing.T) {
	name := CollectionName(testUUID(0xAB), "ollama-jina/jina-embeddings-v2-base-en-768")
	if !strings.HasPrefix(name, "project_") {
		t.Errorf("name = %q, want prefix 'project_'", name)
	}
	if !strings.Contains(name, "__emb_ollama-jina/jina-embeddings-v2-base-en-768") {
		t.Errorf("name = %q, want to contain '__emb_ollama-jina/jina-embeddings-v2-base-en-768'", name)
	}
}

// --- EnsureCollection tests ---

func TestEnsureCollection_AlreadyExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":{}}`))
	}))
	defer srv.Close()

	client := NewQdrantClient(srv.URL)
	err := client.EnsureCollection(context.Background(), "test_collection", 768)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureCollection_Creates(t *testing.T) {
	var createCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == http.MethodPut {
			createCalled = true
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			vectors := body["vectors"].(map[string]interface{})
			if int(vectors["size"].(float64)) != 768 {
				t.Errorf("size = %v, want 768", vectors["size"])
			}
			if vectors["distance"] != "Cosine" {
				t.Errorf("distance = %v, want Cosine", vectors["distance"])
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Errorf("unexpected method: %s", r.Method)
	}))
	defer srv.Close()

	client := NewQdrantClient(srv.URL)
	err := client.EnsureCollection(context.Background(), "test_collection", 768)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !createCalled {
		t.Error("expected PUT to create collection")
	}
}

func TestEnsureCollection_CreateError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	client := NewQdrantClient(srv.URL)
	err := client.EnsureCollection(context.Background(), "test_collection", 768)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error = %q, want to contain 400", err)
	}
}

// --- UpsertPoints tests ---

func TestUpsertPoints_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/points") {
			t.Errorf("path = %q", r.URL.Path)
		}

		var body qdrantUpsertRequest
		json.NewDecoder(r.Body).Decode(&body)
		if len(body.Points) != 2 {
			t.Errorf("points = %d, want 2", len(body.Points))
		}
		if body.Points[0].ID != "point-1" {
			t.Errorf("point[0].ID = %q", body.Points[0].ID)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewQdrantClient(srv.URL)
	err := client.UpsertPoints(context.Background(), "test_collection", []Point{
		{ID: "point-1", Vector: []float32{0.1, 0.2}, Payload: map[string]interface{}{"key": "val"}},
		{ID: "point-2", Vector: []float32{0.3, 0.4}, Payload: map[string]interface{}{}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpsertPoints_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := NewQdrantClient(srv.URL)
	err := client.UpsertPoints(context.Background(), "test_collection", []Point{
		{ID: "p1", Vector: []float32{0.1}, Payload: map[string]interface{}{}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want to contain 500", err)
	}
}

// --- GetPoints tests ---

func TestGetPoints_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/points") {
			t.Errorf("path = %q", r.URL.Path)
		}

		var body qdrantGetPointsRequest
		json.NewDecoder(r.Body).Decode(&body)
		if len(body.IDs) != 2 {
			t.Errorf("ids = %d, want 2", len(body.IDs))
		}
		if !body.WithVector {
			t.Error("with_vector should be true")
		}
		if !body.WithPayload {
			t.Error("with_payload should be true")
		}

		resp := qdrantGetPointsResponse{
			Result: []qdrantPointResult{
				{ID: "id-1", Vector: []float32{0.1, 0.2}, Payload: map[string]interface{}{"key": "val1"}},
				{ID: "id-2", Vector: []float32{0.3, 0.4}, Payload: map[string]interface{}{"key": "val2"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewQdrantClient(srv.URL)
	points, err := client.GetPoints(context.Background(), "test_collection", []string{"id-1", "id-2"}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("points = %d, want 2", len(points))
	}
	if points[0].ID != "id-1" {
		t.Errorf("points[0].ID = %q, want id-1", points[0].ID)
	}
	if len(points[0].Vector) != 2 {
		t.Errorf("points[0].Vector length = %d, want 2", len(points[0].Vector))
	}
}

func TestGetPoints_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("HTTP request should not be made for empty IDs")
	}))
	defer srv.Close()

	client := NewQdrantClient(srv.URL)
	points, err := client.GetPoints(context.Background(), "test_collection", []string{}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if points != nil {
		t.Errorf("expected nil for empty IDs, got %v", points)
	}
}

func TestGetPoints_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("collection not found"))
	}))
	defer srv.Close()

	client := NewQdrantClient(srv.URL)
	_, err := client.GetPoints(context.Background(), "missing_collection", []string{"id-1"}, true)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want to contain 404", err)
	}
}

func TestUpsertPoints_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("HTTP request should not be made for empty points")
		http.Error(w, "unexpected", http.StatusBadRequest)
	}))
	defer srv.Close()

	client := NewQdrantClient(srv.URL)
	err := client.UpsertPoints(context.Background(), "test_collection", []Point{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
