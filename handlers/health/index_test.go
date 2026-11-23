package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
