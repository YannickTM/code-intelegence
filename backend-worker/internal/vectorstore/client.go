package vectorstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// VectorStore defines the interface for vector storage operations.
type VectorStore interface {
	EnsureCollection(ctx context.Context, collection string, dimensions int32) error
	UpsertPoints(ctx context.Context, collection string, points []Point) error
	GetPoints(ctx context.Context, collection string, ids []string, withVector bool) ([]Point, error)
}

// Point represents a single vector to store in Qdrant.
type Point struct {
	ID      string                 // UUID string used as point ID
	Vector  []float32              // embedding vector
	Payload map[string]interface{} // lean metadata
}

// QdrantClient is a REST client for Qdrant.
type QdrantClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewQdrantClient creates a Qdrant REST client.
func NewQdrantClient(baseURL string) *QdrantClient {
	return &QdrantClient{
		httpClient: &http.Client{},
		baseURL:    baseURL,
	}
}

// EnsureCollection creates the collection if it does not exist.
// Uses Cosine distance and the specified vector dimensions.
func (c *QdrantClient) EnsureCollection(ctx context.Context, collection string, dimensions int32) error {
	// Check if collection exists.
	url := fmt.Sprintf("%s/collections/%s", c.baseURL, collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("vectorstore: create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vectorstore: check collection %s: %w", collection, err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil // already exists
	}
	if resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("vectorstore: check collection %s returned unexpected status %d", collection, resp.StatusCode)
	}

	// Collection not found — create it.
	createBody := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     dimensions,
			"distance": "Cosine",
		},
	}
	body, err := json.Marshal(createBody)
	if err != nil {
		return fmt.Errorf("vectorstore: marshal create body: %w", err)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("vectorstore: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err = c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vectorstore: create collection %s: %w", collection, err)
	}
	defer resp.Body.Close()

	// 409 Conflict means another worker created it concurrently — that's fine.
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusConflict {
		return nil
	}

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return fmt.Errorf("vectorstore: create collection %s returned %d: %s", collection, resp.StatusCode, respBody)
}

// qdrantUpsertRequest is the Qdrant PUT /collections/{name}/points request.
type qdrantUpsertRequest struct {
	Points []qdrantPoint `json:"points"`
}

type qdrantPoint struct {
	ID      string                 `json:"id"`
	Vector  []float32              `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
}

// UpsertPoints upserts vectors into the specified Qdrant collection.
func (c *QdrantClient) UpsertPoints(ctx context.Context, collection string, points []Point) error {
	if len(points) == 0 {
		return nil
	}

	qPoints := make([]qdrantPoint, len(points))
	for i, p := range points {
		qPoints[i] = qdrantPoint{
			ID:      p.ID,
			Vector:  p.Vector,
			Payload: p.Payload,
		}
	}

	body, err := json.Marshal(qdrantUpsertRequest{Points: qPoints})
	if err != nil {
		return fmt.Errorf("vectorstore: marshal upsert body: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points", c.baseURL, collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("vectorstore: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vectorstore: upsert points to %s: %w", collection, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return fmt.Errorf("vectorstore: upsert to %s returned %d: %s", collection, resp.StatusCode, respBody)
}

// qdrantGetPointsRequest is the Qdrant POST /collections/{name}/points request.
type qdrantGetPointsRequest struct {
	IDs         []string `json:"ids"`
	WithVector  bool     `json:"with_vector"`
	WithPayload bool     `json:"with_payload"`
}

// qdrantGetPointsResponse wraps the Qdrant response for point retrieval.
type qdrantGetPointsResponse struct {
	Result []qdrantPointResult `json:"result"`
}

type qdrantPointResult struct {
	ID      string                 `json:"id"`
	Vector  []float32              `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
}

// getPointsBatchSize is the maximum number of IDs per GetPoints request.
const getPointsBatchSize = 100

// GetPoints fetches points by their IDs from the specified Qdrant collection.
// If withVector is true, the embedding vectors are included in the result.
func (c *QdrantClient) GetPoints(ctx context.Context, collection string, ids []string, withVector bool) ([]Point, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	var allPoints []Point
	for start := 0; start < len(ids); start += getPointsBatchSize {
		end := start + getPointsBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]

		points, err := c.getPointsBatch(ctx, collection, batch, withVector)
		if err != nil {
			return nil, err
		}
		allPoints = append(allPoints, points...)
	}
	return allPoints, nil
}

func (c *QdrantClient) getPointsBatch(ctx context.Context, collection string, ids []string, withVector bool) ([]Point, error) {
	reqBody := qdrantGetPointsRequest{
		IDs:         ids,
		WithVector:  withVector,
		WithPayload: true,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("vectorstore: marshal get points body: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points", c.baseURL, collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("vectorstore: create get points request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vectorstore: get points from %s: %w", collection, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("vectorstore: get points from %s returned %d: %s", collection, resp.StatusCode, respBody)
	}

	var result qdrantGetPointsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("vectorstore: decode get points response: %w", err)
	}

	points := make([]Point, len(result.Result))
	for i, r := range result.Result {
		points[i] = Point{
			ID:      r.ID,
			Vector:  r.Vector,
			Payload: r.Payload,
		}
	}
	return points, nil
}
