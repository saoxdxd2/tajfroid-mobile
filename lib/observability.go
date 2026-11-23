// Observability helpers shared by the router and handlers. Everything here is
// deliberately cheap: request ids are bounded, logs are one JSON line written
// with a single stdout write, and metrics are fixed-size in-memory windows.
package lib

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// RequestIDHeader is echoed on every response and stored on the request
	// context so downstream handlers can reference the same id.
	RequestIDHeader = "X-Request-ID"
	maxRequestIDLen = 128

	// metricWindow is the number of most recent samples kept per route+status.
	metricWindow = 1000
)

type requestIDKey struct{}

// WithRequestID stores the verified/generated request id on the request.
func WithRequestID(r *http.Request, id string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
}

// RequestIDFrom returns the id set by the router, or "" when absent.
func RequestIDFrom(r *http.Request) string {
	if r == nil {
		return ""
	}
	id, _ := r.Context().Value(requestIDKey{}).(string)
	return id
}

// RequestID accepts a safe inbound id, otherwise it generates a fresh one.
func RequestID(r *http.Request) string {
	return SafeInboundRequestID(r.Header.Get(RequestIDHeader))
}

// SafeInboundRequestID keeps printable ASCII ids up to 128 bytes. Anything
// else (empty, oversized, control characters) is replaced by a generated id
// so a caller cannot inject headers or unbounded data into logs.
func SafeInboundRequestID(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > maxRequestIDLen {
		return NewID()
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 0x20 || v[i] > 0x7e {
			return NewID()
		}
	}
	return v
}

// SafeRouteLabel reduces a caller-supplied route to a compact label safe to
// log and to use as a metrics key. Unknown shapes collapse to "unknown" rather
// than leaking arbitrary query data.
func SafeRouteLabel(route string) string {
	route = strings.Trim(route, "/")
	if route == "" || len(route) > maxRequestIDLen {
		return "unknown"
	}
	for i := 0; i < len(route); i++ {
		c := route[i]
		letter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		digit := c >= '0' && c <= '9'
		if !letter && !digit && c != '/' && c != '_' && c != '-' && c != ':' {
			return "unknown"
		}
	}
	return route
}

// StatusWriter wraps a ResponseWriter to capture the status actually written.
// It exposes Unwrap so http.ResponseController can still reach the underlying
// writer, preserving existing handler behaviour.
type StatusWriter struct {
	http.ResponseWriter
	status int
}

func NewStatusWriter(w http.ResponseWriter) *StatusWriter {
	return &StatusWriter{ResponseWriter: w}
}

func (s *StatusWriter) WriteHeader(code int) {
	if s.status != 0 {
		return
	}
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *StatusWriter) Write(p []byte) (int, error) {
	if s.status == 0 {
		s.WriteHeader(http.StatusOK)
	}
	return s.ResponseWriter.Write(p)
}

// Status reports the response code, defaulting to 200 when the handler wrote
// no explicit header (net/http's default).
func (s *StatusWriter) Status() int {
	if s.status == 0 {
		return http.StatusOK
	}
	return s.status
}

func (s *StatusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// RequestLog is the structured, single-line record emitted at request
// completion. It never contains request bodies, tokens or raw error strings.
type RequestLog struct {
	Timestamp     string  `json:"timestamp"`
	RequestID     string  `json:"requestId"`
	Route         string  `json:"route"`
	Method        string  `json:"method"`
	Status        int     `json:"status"`
	DurationMS    float64 `json:"duration_ms"`
	AuthKind      string  `json:"auth_kind,omitempty"`
	ErrorCategory string  `json:"error_category,omitempty"`
}

// ErrorCategory maps a status code to a coarse class. No error text is kept.
func ErrorCategory(status int) string {
	switch {
	case status >= 500:
		return "server_error"
	case status == 401 || status == 403:
		return "auth"
	case status == 404:
		return "not_found"
	case status == 409:
		return "conflict"
	case status == 429:
		return "rate_limited"
	case status >= 400:
		return "bad_request"
	default:
		return ""
	}
}

// EncodeRequestLog serialises one log entry without a trailing newline.
func EncodeRequestLog(entry RequestLog) []byte {
	b, _ := json.Marshal(entry)
	return b
}

// WriteRequestLog emits exactly one JSON line. A single small write to stdout
// keeps logging off the request path's critical section (no extra I/O calls).
func WriteRequestLog(entry RequestLog) {
	line := append(EncodeRequestLog(entry), '\n')
	_, _ = os.Stdout.Write(line)
}

// DurationMS converts a duration to a rounded millisecond value.
func DurationMS(d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	ms := float64(d) / float64(time.Millisecond)
	return math.Round(ms*100) / 100
}

// LatencyStats describes the last (up to) 1000 samples for one key.
type LatencyStats struct {
	Count int     `json:"count"`
	P50MS float64 `json:"p50_ms"`
	P95MS float64 `json:"p95_ms"`
	MaxMS float64 `json:"max_ms"`
}

// MetricsSnapshot is the JSON shape exposed by the opt-in metrics route.
type MetricsSnapshot struct {
	Window int                     `json:"window"`
	Total  int                     `json:"total"`
	Routes map[string]LatencyStats `json:"routes"`
}

// LatencyMetrics is a thread-safe rolling window keyed by route+status.
type LatencyMetrics struct {
	mu      sync.Mutex
	samples map[string][]float64
}

func NewLatencyMetrics() *LatencyMetrics {
	return &LatencyMetrics{samples: make(map[string][]float64)}
}

func (m *LatencyMetrics) Record(route string, status int, d time.Duration) {
	if m == nil {
		return
	}
	key := SafeRouteLabel(route) + ":" + strconv.Itoa(status)
	sample := DurationMS(d)

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.samples == nil {
		m.samples = make(map[string][]float64)
	}
	s := append(m.samples[key], sample)
	if len(s) > metricWindow {
		s = s[len(s)-metricWindow:]
	}
	m.samples[key] = s
}

// Snapshot returns a copy safe to encode while requests keep recording.
func (m *LatencyMetrics) Snapshot() MetricsSnapshot {
	out := MetricsSnapshot{Window: metricWindow, Routes: make(map[string]LatencyStats)}
	if m == nil {
		return out
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, samples := range m.samples {
		if len(samples) == 0 {
			continue
		}
		sorted := append([]float64(nil), samples...)
		sort.Float64s(sorted)
		n := len(sorted)
		out.Routes[key] = LatencyStats{
			Count: n,
			P50MS: sorted[(n-1)/2],
			P95MS: sorted[(n-1)*95/100],
			MaxMS: sorted[n-1],
		}
		out.Total += n
	}
	return out
}

// DefaultMetrics is shared by every request served by the router.
var DefaultMetrics = NewLatencyMetrics()
