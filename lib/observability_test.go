package lib

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRequestIDInboundAndContext(t *testing.T) {
	const valid = "client-req-123"
	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	r.Header.Set(RequestIDHeader, valid)
	id := RequestID(r)
	if id != valid {
		t.Fatalf("inbound id: got %q, want %q", id, valid)
	}
	r = WithRequestID(r, id)
	if got := RequestIDFrom(r); got != valid {
		t.Fatalf("context id: got %q, want %q", got, valid)
	}
	if got := r.Header.Get(RequestIDHeader); got != valid {
		t.Fatalf("header id: got %q, want %q", got, valid)
	}

	for _, bad := range []string{
		"",
		strings.Repeat("x", 129),
		"a\nb",
		"a\x00b",
		"a\rb",
		"ünicode",
	} {
		got := SafeInboundRequestID(bad)
		if len(got) != 32 {
			t.Errorf("unsafe id %q: generated %q (len %d), want 32-char id", bad, got, len(got))
		}
		if got == strings.TrimSpace(bad) && bad != "" {
			t.Errorf("unsafe id %q was accepted unchanged", bad)
		}
	}
}

func TestStatusWriterCapturesStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := NewStatusWriter(rec)
	if got := sw.Status(); got != http.StatusOK {
		t.Fatalf("default status: got %d, want 200", got)
	}
	if _, err := sw.Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if got := sw.Status(); got != http.StatusOK {
		t.Fatalf("implicit status: got %d, want 200", got)
	}

	rec2 := httptest.NewRecorder()
	sw2 := NewStatusWriter(rec2)
	sw2.WriteHeader(http.StatusNoContent)
	sw2.WriteHeader(http.StatusInternalServerError)
	if got := sw2.Status(); got != http.StatusNoContent {
		t.Fatalf("explicit status overwritten: got %d, want 204", got)
	}
	if sw2.Unwrap() != rec2 {
		t.Fatal("Unwrap must expose the underlying writer")
	}
}

func TestRequestLogShapeAndRedaction(t *testing.T) {
	entry := RequestLog{
		Timestamp:     "2026-11-23T10:00:00Z",
		RequestID:     "req-1",
		Route:         "invoices",
		Method:        http.MethodPost,
		Status:        http.StatusUnauthorized,
		DurationMS:    1.25,
		AuthKind:      "device",
		ErrorCategory: ErrorCategory(http.StatusUnauthorized),
	}
	line := string(EncodeRequestLog(entry))
	if strings.Contains(line, "\n") {
		t.Fatalf("log entry contains a newline: %q", line)
	}
	var fields map[string]interface{}
	if err := json.Unmarshal([]byte(line), &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"timestamp", "requestId", "route", "method", "status", "duration_ms", "auth_kind", "error_category"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("missing field %q in %s", key, line)
		}
	}
	if fields["error_category"] != "auth" {
		t.Errorf("error category: got %v, want auth", fields["error_category"])
	}
	if strings.Contains(line, "Bearer") || strings.Contains(line, "password") {
		t.Errorf("log leaked credentials: %s", line)
	}

	plain := string(EncodeRequestLog(RequestLog{Status: http.StatusOK}))
	if strings.Contains(plain, "auth_kind") || strings.Contains(plain, "error_category") {
		t.Errorf("unknown fields should be omitted: %s", plain)
	}
	for status, want := range map[int]string{
		200: "", 400: "bad_request", 401: "auth", 403: "auth",
		404: "not_found", 409: "conflict", 429: "rate_limited", 500: "server_error",
	} {
		if got := ErrorCategory(status); got != want {
			t.Errorf("ErrorCategory(%d): got %q, want %q", status, got, want)
		}
	}
}

func TestLatencyMetricsRollingWindow(t *testing.T) {
	m := NewLatencyMetrics()
	for i := 0; i < metricWindow+5; i++ {
		m.Record("invoices", http.StatusOK, time.Duration(i)*time.Millisecond)
	}
	m.Record("health", http.StatusInternalServerError, 7*time.Millisecond)

	snap := m.Snapshot()
	if snap.Window != metricWindow {
		t.Fatalf("window: got %d, want %d", snap.Window, metricWindow)
	}
	stats, ok := snap.Routes["invoices:200"]
	if !ok {
		t.Fatalf("missing route+status key: %#v", snap.Routes)
	}
	if stats.Count != metricWindow {
		t.Fatalf("count: got %d, want %d", stats.Count, metricWindow)
	}
	if stats.MaxMS != 1004 || stats.P50MS <= 0 || stats.P95MS <= stats.P50MS || stats.P95MS > stats.MaxMS {
		t.Fatalf("unexpected percentiles: %#v", stats)
	}
	if got := snap.Routes["health:500"]; got.Count != 1 || got.MaxMS != 7 {
		t.Fatalf("health:500 stats: got %#v", got)
	}

	snap.Routes["invoices:200"] = LatencyStats{}
	if again := m.Snapshot(); again.Routes["invoices:200"].Count != metricWindow {
		t.Fatal("snapshot must be a copy of the live metrics")
	}
}

func TestSafeRouteLabel(t *testing.T) {
	cases := map[string]string{
		"invoices":                   "invoices",
		"/agent/scans/:id/status":    "agent/scans/:id/status",
		"products/manage":            "products/manage",
		"invoices?token=secret":      "unknown",
		strings.Repeat("a", 129):     "unknown",
		"":                           "unknown",
		"agent/scans/abc\x00/status": "unknown",
	}
	for input, want := range cases {
		if got := SafeRouteLabel(input); got != want {
			t.Errorf("SafeRouteLabel(%q): got %q, want %q", input, got, want)
		}
	}
}
