package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"tajfroid-mobile/lib"
)

func healthPipelinePayload(t *testing.T, indexRows [][]interface{}) []byte {
	t.Helper()
	results := []interface{}{
		map[string]interface{}{
			"type": "execute",
			"response": map[string]interface{}{
				"result": map[string]interface{}{
					"columns": []interface{}{}, "rows": []interface{}{},
					"affected_row_count": 0,
				},
			},
		},
		map[string]interface{}{
			"type": "execute",
			"response": map[string]interface{}{
				"result": map[string]interface{}{
					"columns":            []interface{}{},
					"rows":               indexRows,
					"affected_row_count": 0,
				},
			},
		},
	}
	b, err := json.Marshal(map[string]interface{}{"results": results})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func healthMockServer(t *testing.T, indexRows [][]interface{}, posts *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*posts++
		var body lib.TursoPipelineBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if len(body.Requests) != 2 {
			t.Errorf("health sent %d statements, want 2", len(body.Requests))
		}
		if !strings.Contains(body.Requests[0].Stmt.SQL, "SELECT 1") {
			t.Errorf("first health statement = %q", body.Requests[0].Stmt.SQL)
		}
		if !strings.Contains(body.Requests[1].Stmt.SQL, "sqlite_master") {
			t.Errorf("second health statement = %q", body.Requests[1].Stmt.SQL)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(healthPipelinePayload(t, indexRows))
	}))
}

func TestHealthReportsIndexesWithOneRoundTrip(t *testing.T) {
	posts := 0
	srv := healthMockServer(t, [][]interface{}{
		{"idx_devices_token_hash"},
		{"idx_agent_tokens_token_hash"},
	}, &posts)
	defer srv.Close()

	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "health-test-token")
	db := lib.NewTurso()
	if tr, ok := db.HTTP.Transport.(*http.Transport); ok {
		defer tr.CloseIdleConnections()
	}

	rec := httptest.NewRecorder()
	Handler(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if posts != 1 {
		t.Fatalf("pipeline posts = %d, want 1", posts)
	}
	var out healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "ok" || !out.OK || !out.Checks["database"] || !out.Checks["auth_indexes"] {
		t.Fatalf("unexpected health response: %+v", out)
	}
	if !out.Indexes[deviceTokenIndex] || !out.Indexes[agentTokenIndex] {
		t.Fatalf("indexes not reported: %+v", out.Indexes)
	}
	if strings.Contains(rec.Body.String(), "health-test-token") {
		t.Fatal("health response leaked the Turso token")
	}
}

func TestHealthDegradesWhenIndexMissing(t *testing.T) {
	posts := 0
	srv := healthMockServer(t, [][]interface{}{{"idx_devices_token_hash"}}, &posts)
	defer srv.Close()

	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "health-test-token-2")
	db := lib.NewTurso()
	if tr, ok := db.HTTP.Transport.(*http.Transport); ok {
		defer tr.CloseIdleConnections()
	}

	rec := httptest.NewRecorder()
	Handler(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
	var out healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "degraded" || out.OK || !out.Checks["database"] || out.Checks["auth_indexes"] {
		t.Fatalf("unexpected health response: %+v", out)
	}
}

func healthChaosServer(t *testing.T, payload string, posts *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		posts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "health-chaos-token")
	if tr, ok := lib.NewTurso().HTTP.Transport.(*http.Transport); ok {
		t.Cleanup(tr.CloseIdleConnections)
	}
	return srv
}

func TestHealthConcurrentCallsStayIndependent(t *testing.T) {
	var posts atomic.Int64
	healthChaosServer(t, tursoIndexesOKPayloadForHealthChaos(), &posts)

	const calls = 40
	var wg sync.WaitGroup
	results := make([]int, calls)
	for i := 0; i < calls; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := httptest.NewRecorder()
			Handler(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
			results[i] = rec.Code
			if strings.Contains(rec.Body.String(), "health-chaos-token") {
				t.Errorf("call %d leaked the upstream token", i)
			}
		}(i)
	}
	wg.Wait()
	for i, code := range results {
		if code != http.StatusOK {
			t.Errorf("call %d: status = %d, want 200", i, code)
		}
	}
	if got := posts.Load(); got != calls {
		t.Fatalf("pipeline posts = %d, want %d", got, calls)
	}
}

func tursoIndexesOKPayloadForHealthChaos() string {
	b, _ := json.Marshal(map[string]interface{}{"results": []interface{}{
		map[string]interface{}{"type": "execute", "response": map[string]interface{}{
			"result": map[string]interface{}{"rows": []interface{}{}, "affected_row_count": 0}}},
		map[string]interface{}{"type": "execute", "response": map[string]interface{}{
			"result": map[string]interface{}{"rows": [][]interface{}{
				{"idx_devices_token_hash"}, {"idx_agent_tokens_token_hash"},
			}, "affected_row_count": 0}}},
	}})
	return string(b)
}

func TestHealthMalformedDatabaseResponsesDegradeCleanly(t *testing.T) {
	const secret = "health-chaos-secret-detail"
	cases := []struct {
		name, payload string
		want          int
		wantErrorText string
	}{
		{"empty results", `{"results":[]}`, http.StatusServiceUnavailable, "database check failed"},
		{
			name:    "nested and short rows",
			payload: `{"results":[{"response":{"result":{"rows":[]}}},{"response":{"result":{"rows":[[],["unexpected"],[{"type":"null","value":null}],{"not":"a row"}],"affected_row_count":0}}}]}`,
			want:    http.StatusServiceUnavailable,
		},
		{
			name:    "invalid JSON with secret",
			payload: `{"results":[` + secret,
			want:    http.StatusServiceUnavailable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var posts atomic.Int64
			healthChaosServer(t, tc.payload, &posts)
			rec := httptest.NewRecorder()
			Handler(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), secret) || strings.Contains(rec.Body.String(), "health-chaos-token") {
				t.Fatalf("health response leaked upstream data: %s", rec.Body.String())
			}
			if tc.wantErrorText != "" && !strings.Contains(rec.Body.String(), tc.wantErrorText) {
				t.Fatalf("response has no %q marker: %s", tc.wantErrorText, rec.Body.String())
			}
			var out healthResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("health body is not JSON: %q (%v)", rec.Body.String(), err)
			}
			if out.OK || out.Status != "degraded" || out.Error == "" {
				t.Fatalf("unexpected degraded response: %+v", out)
			}
		})
	}
}

func TestHealthUnavailableDatabaseIsDegraded(t *testing.T) {
	// Point at a server that is immediately closed: every request fails at the
	// transport layer and must degrade rather than panic or hang.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	t.Setenv("TURSO_DATABASE_URL", url)
	t.Setenv("TURSO_AUTH_TOKEN", "health-unavailable-token")
	if tr, ok := lib.NewTurso().HTTP.Transport.(*http.Transport); ok {
		defer tr.CloseIdleConnections()
	}

	rec := httptest.NewRecorder()
	Handler(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (%s)", rec.Code, rec.Body.String())
	}
	var out healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "degraded" || out.OK || !strings.Contains(rec.Body.String(), "database check failed") {
		t.Fatalf("unexpected health response: %+v (%s)", out, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "health-unavailable-token") {
		t.Fatal("health response leaked the upstream token")
	}
}
