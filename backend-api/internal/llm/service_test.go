package llm

import (
	"context"
	"strings"
	"testing"

	"myjungle/backend-api/internal/config"
	"myjungle/backend-api/internal/storage/postgres"

	db "myjungle/datastore/postgres/sqlc"
)

func TestBootstrapDefaultConfig_RejectsBlankEndpoint(t *testing.T) {
	database := &postgres.DB{Queries: db.New(nil)}

	err := BootstrapDefaultConfig(context.Background(), database, config.LLMDefaults{
		Provider:    "ollama",
		EndpointURL: "   ",
		Model:       "",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "endpoint_url is required") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "endpoint_url is required")
	}
}
