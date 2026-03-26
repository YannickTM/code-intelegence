package embedding

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-worker/internal/execution"
	db "myjungle/datastore/postgres/sqlc"
)

func testUUID(b byte) pgtype.UUID {
	var u pgtype.UUID
	u.Bytes[0] = b
	u.Valid = true
	return u
}

var testEmbCfg = execution.EmbeddingConfig{
	ID:          testUUID(0x10),
	Provider:    "ollama",
	Model:       "jina/jina-embeddings-v2-base-en",
	Dimensions:  768,
	EndpointURL: "http://localhost:11434",
}

// --- VersionLabel tests ---

func TestVersionLabel(t *testing.T) {
	got := VersionLabel("ollama", "jina/jina-embeddings-v2-base-en", 768)
	want := "ollama-jina-jina-embeddings-v2-base-en-768"
	if got != want {
		t.Errorf("VersionLabel() = %q, want %q", got, want)
	}
}

func TestVersionLabel_DifferentDimensions(t *testing.T) {
	got := VersionLabel("openai", "text-embedding-3-large", 3072)
	want := "openai-text-embedding-3-large-3072"
	if got != want {
		t.Errorf("VersionLabel() = %q, want %q", got, want)
	}
}

func TestVersionLabel_SlashInModelName(t *testing.T) {
	got := VersionLabel("ollama", "org/sub/model", 512)
	want := "ollama-org-sub-model-512"
	if got != want {
		t.Errorf("VersionLabel() = %q, want %q", got, want)
	}
}

// --- mockQuerier for version tests ---

type versionMockQuerier struct {
	db.Querier
	getByLabelFn func(ctx context.Context, label string) (db.EmbeddingVersion, error)
	createFn     func(ctx context.Context, arg db.CreateEmbeddingVersionParams) (db.EmbeddingVersion, error)
}

func (m *versionMockQuerier) GetEmbeddingVersionByLabel(ctx context.Context, label string) (db.EmbeddingVersion, error) {
	return m.getByLabelFn(ctx, label)
}

func (m *versionMockQuerier) CreateEmbeddingVersion(ctx context.Context, arg db.CreateEmbeddingVersionParams) (db.EmbeddingVersion, error) {
	return m.createFn(ctx, arg)
}

// --- ResolveVersion tests ---

func TestResolveVersion_ExistingLabel(t *testing.T) {
	want := db.EmbeddingVersion{ID: testUUID(0x20), VersionLabel: "ollama-jina-jina-embeddings-v2-base-en-768"}
	m := &versionMockQuerier{
		getByLabelFn: func(_ context.Context, label string) (db.EmbeddingVersion, error) {
			if label != "ollama-jina-jina-embeddings-v2-base-en-768" {
				t.Errorf("label = %q", label)
			}
			return want, nil
		},
		createFn: func(_ context.Context, _ db.CreateEmbeddingVersionParams) (db.EmbeddingVersion, error) {
			t.Fatal("create should not be called")
			return db.EmbeddingVersion{}, nil
		},
	}

	got, err := ResolveVersion(context.Background(), m, testEmbCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("ID = %v, want %v", got.ID, want.ID)
	}
}

func TestResolveVersion_CreatesNew(t *testing.T) {
	want := db.EmbeddingVersion{ID: testUUID(0x21), VersionLabel: "ollama-jina-jina-embeddings-v2-base-en-768"}
	m := &versionMockQuerier{
		getByLabelFn: func(_ context.Context, _ string) (db.EmbeddingVersion, error) {
			return db.EmbeddingVersion{}, pgx.ErrNoRows
		},
		createFn: func(_ context.Context, arg db.CreateEmbeddingVersionParams) (db.EmbeddingVersion, error) {
			if arg.VersionLabel != "ollama-jina-jina-embeddings-v2-base-en-768" {
				t.Errorf("label = %q", arg.VersionLabel)
			}
			if arg.Dimensions != 768 {
				t.Errorf("dimensions = %d", arg.Dimensions)
			}
			return want, nil
		},
	}

	got, err := ResolveVersion(context.Background(), m, testEmbCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("ID = %v, want %v", got.ID, want.ID)
	}
}

func TestResolveVersion_LookupError(t *testing.T) {
	m := &versionMockQuerier{
		getByLabelFn: func(_ context.Context, _ string) (db.EmbeddingVersion, error) {
			return db.EmbeddingVersion{}, errors.New("db connection lost")
		},
	}

	_, err := ResolveVersion(context.Background(), m, testEmbCfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "lookup version") {
		t.Errorf("error = %q, want to contain %q", err, "lookup version")
	}
}

func TestResolveVersion_CreateError(t *testing.T) {
	m := &versionMockQuerier{
		getByLabelFn: func(_ context.Context, _ string) (db.EmbeddingVersion, error) {
			return db.EmbeddingVersion{}, pgx.ErrNoRows
		},
		createFn: func(_ context.Context, _ db.CreateEmbeddingVersionParams) (db.EmbeddingVersion, error) {
			return db.EmbeddingVersion{}, errors.New("insert failed")
		},
	}

	_, err := ResolveVersion(context.Background(), m, testEmbCfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "create version") {
		t.Errorf("error = %q, want to contain %q", err, "create version")
	}
}
