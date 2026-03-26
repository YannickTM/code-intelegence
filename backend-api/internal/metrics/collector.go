// Package metrics provides a simple Prometheus-compatible metrics collector.
package metrics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// metricKey identifies a unique request counter.
type metricKey struct {
	Method string
	Path   string
	Status int
}

// Collector accumulates request counts and uptime for Prometheus exposition.
type Collector struct {
	startedAt time.Time
	mu        sync.RWMutex
	counts    map[metricKey]int64
}

// NewCollector creates a Collector that tracks uptime from startedAt.
func NewCollector(startedAt time.Time) *Collector {
	return &Collector{
		startedAt: startedAt,
		counts:    make(map[metricKey]int64),
	}
}

// RecordRequest increments the counter for the given method/path/status triple.
func (c *Collector) RecordRequest(method, path string, status int) {
	key := metricKey{Method: method, Path: path, Status: status}
	c.mu.Lock()
	c.counts[key]++
	c.mu.Unlock()
}

// Render produces Prometheus text exposition format.
func (c *Collector) Render() string {
	var b strings.Builder

	// Uptime gauge.
	b.WriteString("# HELP myjungle_api_uptime_seconds Process uptime in seconds.\n")
	b.WriteString("# TYPE myjungle_api_uptime_seconds gauge\n")
	b.WriteString("myjungle_api_uptime_seconds ")
	b.WriteString(formatSeconds(time.Since(c.startedAt)))
	b.WriteByte('\n')

	// Request counter.
	c.mu.RLock()
	keys := make([]metricKey, 0, len(c.counts))
	snapshot := make(map[metricKey]int64, len(c.counts))
	for k, v := range c.counts {
		keys = append(keys, k)
		snapshot[k] = v
	}
	c.mu.RUnlock()

	if len(keys) > 0 {
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].Method != keys[j].Method {
				return keys[i].Method < keys[j].Method
			}
			if keys[i].Path != keys[j].Path {
				return keys[i].Path < keys[j].Path
			}
			return keys[i].Status < keys[j].Status // numeric comparison
		})
		b.WriteString("# HELP myjungle_api_requests_total Total HTTP requests served.\n")
		b.WriteString("# TYPE myjungle_api_requests_total counter\n")
		for _, k := range keys {
			fmt.Fprintf(&b, "myjungle_api_requests_total{method=\"%s\",path=\"%s\",status=\"%d\"} %d\n",
				escapeLabelValue(k.Method), escapeLabelValue(k.Path), k.Status, snapshot[k])
		}
	}

	return b.String()
}

// formatSeconds formats a duration as a decimal number of seconds,
// trimming trailing zeros and unnecessary decimal points.
func formatSeconds(d time.Duration) string {
	seconds := d.Seconds()
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(seconds, 'f', 3, 64), "0"), ".")
}

// escapeLabelValue escapes a string for use as a Prometheus label value.
// Prometheus requires: \ → \\, " → \", newline → \n.
func escapeLabelValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}
