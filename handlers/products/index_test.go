package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tajfroid-mobile/lib"
)

func TestCreateProductIsIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req lib.TursoPipelineBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		sql := ""
		if len(req.Requests) > 0 {
			sql = req.Requests[0].Stmt.SQL
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(sql, "SELECT sku, name FROM catalog WHERE sku=?") {
			w.Write([]byte(`{"results":[{"type":"execute","response":{"result":{"cols":[],"rows":[["S1","Existing"]],"affected_row_count":0}}}]}`))
			return
		}
		if strings.Contains(sql, "INSERT INTO catalog") {
			t.Fatalf("existing sku must not be inserted again: %s", sql)
		}
		t.Errorf("unexpected SQL: %s", sql)
	}))
	defer srv.Close()
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "test")

	body := `{"sku":"S1","name":"New"}`
	req := httptest.NewRequest(http.MethodPost, "/api/products", strings.NewReader(body))
	rec := httptest.NewRecorder()
	Handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var data map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data["idempotent"] != true || data["name"] != "Existing" {
		t.Fatalf("unexpected replay response: %s", rec.Body.String())
	}
}
