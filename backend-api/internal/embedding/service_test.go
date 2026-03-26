package embedding

import (
	"context"
	"strings"
	"testing"

	"myjungle/backend-api/internal/config"
	"myjungle/backend-api/internal/storage/postgres"

	db "myjungle/datastore/postgres/sqlc"
)

func TestBootstrapDefaultConfig_RejectsBlankEndpointOrModel(t *testing.T) {
	database := &postgres.DB{Queries: db.New(nil)}

	tests := []struct {
		name     string
		defaults config.EmbeddingDefaults
		want     string
	}{
		{
			name: "blank endpoint",
			defaults: config.EmbeddingDefaults{
				Provider:    "ollama",
				EndpointURL: "   ",
				Model:       "jina/jina-embeddings-v2-base-en",
				Dimensions:  768,
			},
			want: "endpoint_url is required",
		},
		{
			name: "blank model",
			defaults: config.EmbeddingDefaults{
				Provider:    "ollama",
				EndpointURL: "http://host.docker.internal:11434",
				Model:       "   ",
				Dimensions:  768,
			},
			want: "model is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := BootstrapDefaultConfig(context.Background(), database, tt.defaults)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}
