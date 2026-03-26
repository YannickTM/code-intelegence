package metrics

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCollector_Render_UptimeOnly(t *testing.T) {
	c := NewCollector(time.Now().Add(-10 * time.Second))
	out := c.Render()

	if !strings.Contains(out, "# HELP myjungle_api_uptime_seconds") {
		t.Error("missing HELP line for uptime")
	}
	if !strings.Contains(out, "# TYPE myjungle_api_uptime_seconds gauge") {
		t.Error("missing TYPE line for uptime")
	}
	if !strings.Contains(out, "myjungle_api_uptime_seconds ") {
		t.Error("missing uptime metric line")
	}
	if strings.Contains(out, "myjungle_api_requests_total") {
		t.Error("unexpected requests_total with no recorded requests")
	}
}

func TestCollector_Render_WithRequests(t *testing.T) {
	c := NewCollector(time.Now())
	c.RecordRequest("GET", "/health/live", 200)
	c.RecordRequest("GET", "/health/live", 200)
	c.RecordRequest("POST", "/v1/users", 201)

	out := c.Render()

	if !strings.Contains(out, "# HELP myjungle_api_requests_total") {
		t.Error("missing HELP line for requests_total")
	}
	if !strings.Contains(out, "# TYPE myjungle_api_requests_total counter") {
		t.Error("missing TYPE line for requests_total")
	}
	if !strings.Contains(out, `myjungle_api_requests_total{method="GET",path="/health/live",status="200"} 2`) {
		t.Errorf("missing or incorrect counter for GET /health/live 200\n%s", out)
	}
	if !strings.Contains(out, `myjungle_api_requests_total{method="POST",path="/v1/users",status="201"} 1`) {
		t.Errorf("missing or incorrect counter for POST /v1/users 201\n%s", out)
	}
}

func TestCollector_RecordRequest_Concurrent(t *testing.T) {
	c := NewCollector(time.Now())
	var wg sync.WaitGroup

	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.RecordRequest("GET", "/health/live", 200)
		}()
	}
	wg.Wait()

	out := c.Render()
	if !strings.Contains(out, `status="200"} 100`) {
		t.Errorf("expected count 100, got:\n%s", out)
	}
}

func TestFormatSeconds(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  string
	}{
		{0, "0"},
		{1500 * time.Millisecond, "1.5"},
		{100 * time.Second, "100"},
		{time.Millisecond, "0.001"},
		{1100 * time.Millisecond, "1.1"},
		{10 * time.Second, "10"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatSeconds(tt.input); got != tt.want {
				t.Errorf("formatSeconds(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
