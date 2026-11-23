package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tajfroid-mobile/lib"
)

func tursoPayload(rows [][]interface{}, affected int64) string {
	b, _ := json.Marshal(map[string]interface{}{
		"results": []interface{}{map[string]interface{}{
			"type": "execute",
			"response": map[string]interface{}{
				"result": map[string]interface{}{
					"cols":               []interface{}{},
					"rows":               rows,
					"affected_row_count": affected,
				},
			},
		}},
	})
	return string(b)
}

func mockTurso(t *testing.T, revokedAt int64, affected int64, getRows [][]interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req lib.TursoPipelineBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode turso request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sql := ""
		if len(req.Requests) > 0 {
			sql = req.Requests[0].Stmt.SQL
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(sql, "SELECT revoked_at FROM devices"):
			var v interface{}
			if revokedAt > 0 {
				v = revokedAt
			}
			w.Write([]byte(tursoPayload([][]interface{}{{v}}, 0)))
		case strings.Contains(sql, "UPDATE devices SET revoked_at"):
			w.Write([]byte(tursoPayload(nil, affected)))
		case strings.Contains(sql, "SELECT id, name, paired_at"):
			w.Write([]byte(tursoPayload(getRows, 0)))
		default:
			t.Errorf("unexpected SQL: %s", sql)
			http.Error(w, "unexpected SQL", http.StatusInternalServerError)
		}
	}))
}

func TestDeviceRevokeConflictToken(t *testing.T) {
	cases := []struct {
		name        string
		current     int64
		base        int64
		affected    int64
		want        int
		wantRevoked bool
	}{
		{name: "active with fresh base", current: 0, base: 0, affected: 1, want: 200, wantRevoked: true},
		{name: "active with stale base", current: 0, base: 99, want: 409},
		{name: "already revoked with matching base is idempotent", current: 123, base: 123, want: 200, wantRevoked: false},
		{name: "already revoked with zero base conflicts", current: 123, base: 0, want: 409},
		{name: "lost update races into conflict", current: 0, base: 0, affected: 0, want: 409},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := mockTurso(t, tc.current, tc.affected, nil)
			defer srv.Close()
			t.Setenv("TURSO_DATABASE_URL", srv.URL)
			t.Setenv("TURSO_AUTH_TOKEN", "test")

			body, _ := json.Marshal(map[string]interface{}{"baseRevokedAt": tc.base})
			req := httptest.NewRequest(http.MethodDelete, "/api/devices/manage?id=d1", strings.NewReader(string(body)))
			rec := httptest.NewRecorder()
			Handler(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
			var data map[string]interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if tc.want == http.StatusOK && data["revoked"] != tc.wantRevoked {
				t.Fatalf("revoked=%v, want %v", data["revoked"], tc.wantRevoked)
			}
			if tc.want == http.StatusConflict {
				if data["record"] == nil || data["currentRevokedAt"] == nil {
					t.Fatalf("conflict missing merge seed: %s", rec.Body.String())
				}
			}
		})
	}
}

func TestDeviceGetIncludesBaseRevokedAt(t *testing.T) {
	rows := [][]interface{}{{"d1", "phone", 1700000000, 123, 1699990000}}
	srv := mockTurso(t, 0, 0, rows)
	defer srv.Close()
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "test")

	req := httptest.NewRequest(http.MethodGet, "/api/devices/manage?id=d1", nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var data map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data["baseRevokedAt"] != float64(123) {
		t.Fatalf("baseRevokedAt=%v, want 123", data["baseRevokedAt"])
	}
}
