package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mustDecodeJSON decodes the recorder body into a map and fails the test on error.
func mustDecodeJSON(t *testing.T, body *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(body).Decode(&m); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	return m
}

// newJSONRequest creates an HTTP request with a JSON body.
func newJSONRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("failed to encode request body: %v", err)
		}
	}
	return httptest.NewRequest(method, path, &buf)
}
