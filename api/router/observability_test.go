package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	healthhandler "tajfroid-mobile/handlers/health"
	"tajfroid-mobile/lib"
)

func TestRouterEchoesSafeRequestID(t *testing.T) {
	const inbound = "client-req-987"
	req := httptest.NewRequest(http.MethodGet, "/api/router?__route=health", nil)
	req.Header.Set(lib.RequestIDHeader, inbound)
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if got := rec.Header().Get(lib.RequestIDHeader); got != inbound {
		t.Fatalf("request id: got %q, want %q", got, inbound)
	}

	unsafe := strings.Repeat("x", 200)
	req = httptest.NewRequest(http.MethodGet, "/api/router?__route=health", nil)
	req.Header.Set(lib.RequestIDHeader, unsafe)
	rec = httptest.NewRecorder()
	Handler(rec, req)
	got := rec.Header().Get(lib.RequestIDHeader)
	if got == unsafe || len(got) != 32 {
		t.Fatalf("unsafe request id was echoed or malformed: %q", got)
	}
}

func TestMetricsRouteIsFlagGated(t *testing.T) {
	t.Setenv("OBSERVABILITY_METRICS", "")
	req := httptest.NewRequest(http.MethodGet, "/api/router?__route=metrics", nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("flag off: got %d, want 404", rec.Code)
	}
	if got := rec.Header().Get(lib.RequestIDHeader); got == "" {
		t.Fatal("request id must still be set when metrics is disabled")
	}

	t.Setenv("OBSERVABILITY_METRICS", "1")
	lib.DefaultMetrics.Record("health", http.StatusOK, 2*time.Millisecond)
	req = httptest.NewRequest(http.MethodGet, "/api/router?__route=metrics", nil)
	rec = httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("flag on: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var snap lib.MetricsSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Window != 1000 || snap.Routes == nil {
		t.Fatalf("unexpected metrics shape: %+v", snap)
	}
	if stats, ok := snap.Routes["health:200"]; !ok || stats.Count < 1 {
		t.Fatalf("missing health:200 sample: %#v", snap.Routes)
	}
	for key, stats := range snap.Routes {
		if strings.Contains(key, "Bearer") || strings.Contains(key, "token=") {
			t.Errorf("metrics key leaked caller data: %q", key)
		}
		if stats.P50MS > stats.MaxMS || stats.P95MS > stats.MaxMS {
			t.Errorf("invalid percentiles for %q: %#v", key, stats)
		}
	}

	req = httptest.NewRequest(http.MethodPost, "/api/router?__route=metrics", nil)
	rec = httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST metrics: got %d, want 405", rec.Code)
	}
}

func decodeObservabilityHealth(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("health body is not JSON: %q (%v)", rec.Body.String(), err)
	}
	return body
}

func readBoolField(t *testing.T, m map[string]interface{}, key string) bool {
	t.Helper()
	v, ok := m[key].(bool)
	if !ok {
		t.Fatalf("field %q is not a bool: %#v", key, m[key])
	}
	return v
}

func TestHealthUnconfiguredIsDegraded(t *testing.T) {
	t.Setenv("TURSO_DATABASE_URL", "")
	t.Setenv("TURSO_AUTH_TOKEN", "")
	rec := httptest.NewRecorder()
	healthhandler.Handler(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503 (%s)", rec.Code, rec.Body.String())
	}
	body := decodeObservabilityHealth(t, rec)
	if body["status"] != "degraded" || readBoolField(t, body, "ok") {
		t.Fatalf("expected degraded health: %#v", body)
	}
	for _, key := range []string{"checks", "indexes", "latency_ms", "ts"} {
		if _, ok := body[key]; !ok {
			t.Errorf("missing %q in %#v", key, body)
		}
	}
}

const tursoIndexesOKPayload = `{"results":[
  {"type":"execute","response":{"result":{"columns":[],"rows":[],"affected_row_count":0}}},
  {"type":"execute","response":{"result":{"columns":[{"name":"name","type":"text"}],"rows":[["idx_devices_token_hash"],["idx_agent_tokens_token_hash"]],"affected_row_count":0}}}
]}`

const tursoIndexesMissingPayload = `{"results":[
  {"type":"execute","response":{"result":{"columns":[],"rows":[],"affected_row_count":0}}},
  {"type":"execute","response":{"result":{"columns":[{"name":"name","type":"text"}],"rows":[["idx_devices_token_hash"]],"affected_row_count":0}}}
]}`

const tursoErrorPayload = `{"results":[
  {"type":"execute","response":{"result":{"columns":[],"rows":[],"affected_row_count":0}},"error":{"message":"boom-secret detail"}},
  {"type":"execute","response":{"result":{"columns":[],"rows":[],"affected_row_count":0}}}
]}`

func healthWithTurso(t *testing.T, payload string) *httptest.ResponseRecorder {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body lib.TursoPipelineBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("bad pipeline body: %v", err)
			return
		}
		if len(body.Requests) != 2 || body.Requests[0].Stmt.SQL != "SELECT 1" ||
			!strings.Contains(body.Requests[1].Stmt.SQL, "sqlite_master") {
			t.Errorf("unexpected health statements: %#v", body.Requests)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "super-secret-token")

	rec := httptest.NewRecorder()
	healthhandler.Handler(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if strings.Contains(rec.Body.String(), "super-secret-token") {
		t.Fatalf("health response leaked the token: %s", rec.Body.String())
	}
	return rec
}

func TestHealthConfiguredChecksIndexes(t *testing.T) {
	rec := healthWithTurso(t, tursoIndexesOKPayload)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	body := decodeObservabilityHealth(t, rec)
	if body["status"] != "ok" || !readBoolField(t, body, "ok") {
		t.Fatalf("expected healthy response: %#v", body)
	}
	checks, ok := body["checks"].(map[string]interface{})
	if !ok {
		t.Fatalf("checks missing: %#v", body)
	}
	if !readBoolField(t, checks, "database") || !readBoolField(t, checks, "auth_indexes") {
		t.Fatalf("checks should both pass: %#v", checks)
	}
	indexes, ok := body["indexes"].(map[string]interface{})
	if !ok {
		t.Fatalf("indexes missing: %#v", body)
	}
	for _, name := range []string{"idx_devices_token_hash", "idx_agent_tokens_token_hash"} {
		if !readBoolField(t, indexes, name) {
			t.Errorf("index %q should be present: %#v", name, indexes)
		}
	}
	if _, ok := body["latency_ms"].(float64); !ok {
		t.Errorf("latency_ms is not numeric: %#v", body["latency_ms"])
	}
}

func TestHealthMissingIndexIsDegraded(t *testing.T) {
	rec := healthWithTurso(t, tursoIndexesMissingPayload)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503 (%s)", rec.Code, rec.Body.String())
	}
	body := decodeObservabilityHealth(t, rec)
	if body["status"] != "degraded" || readBoolField(t, body, "ok") {
		t.Fatalf("expected degraded response: %#v", body)
	}
	indexes, _ := body["indexes"].(map[string]interface{})
	if readBoolField(t, indexes, "idx_agent_tokens_token_hash") {
		t.Fatalf("missing index reported as present: %#v", indexes)
	}
}

func TestHealthDatabaseFailureNeverEchoesError(t *testing.T) {
	rec := healthWithTurso(t, tursoErrorPayload)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503 (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "boom-secret detail") {
		t.Fatalf("raw database error leaked: %s", rec.Body.String())
	}
	body := decodeObservabilityHealth(t, rec)
	if body["status"] != "degraded" {
		t.Fatalf("expected degraded response: %#v", body)
	}
}
