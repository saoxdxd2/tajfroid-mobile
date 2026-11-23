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

func catalogPayload(n int) []byte {
	items := make([]interface{}, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, map[string]interface{}{
			"type": "execute",
			"response": map[string]interface{}{
				"result": map[string]interface{}{
					"columns": []interface{}{}, "rows": []interface{}{},
					"affected_row_count": 0,
				},
			},
		})
	}
	b, _ := json.Marshal(map[string]interface{}{"results": items})
	return b
}

func TestCatalogRebuildUsesOneRoundTrip(t *testing.T) {
	var posts, statements int64
	var mu sync.Mutex
	var sqls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&posts, 1)
		var body lib.TursoPipelineBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		atomic.StoreInt64(&statements, int64(len(body.Requests)))
		mu.Lock()
		sqls = make([]string, 0, len(body.Requests))
		for _, req := range body.Requests {
			sqls = append(sqls, req.Stmt.SQL)
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write(catalogPayload(len(body.Requests)))
	}))
	defer srv.Close()

	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "catalog-test")
	db := lib.NewTurso()
	if tr, ok := db.HTTP.Transport.(*http.Transport); ok {
		defer tr.CloseIdleConnections()
	}

	body := `{"version":7,"products":[
		{"sku":"A","name":"Alpha","sale_price":"10","vat_rate":"20","stock":2,"active":true},
		{"sku":"B","name":"Beta","sale_price":"12","vat_rate":"20","stock":4,"active":false},
		{"sku":"C","name":"Gamma","sale_price":"14","vat_rate":"20","stock":6,"active":true}
	]}`
	req := httptest.NewRequest(http.MethodPut, "/api/agent/catalog", strings.NewReader(body))
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	gotPosts := atomic.LoadInt64(&posts)
	gotStatements := atomic.LoadInt64(&statements)
	t.Logf("catalog rebuild: posts=%d statements=%d products=3", gotPosts, gotStatements)
	if gotPosts != 1 {
		t.Fatalf("posts=%d, want 1 (catalog rebuild is N+1 round-trips)", gotPosts)
	}
	if gotStatements != 5 {
		t.Fatalf("statements=%d, want 5 (delete+3 inserts+meta)", gotStatements)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sqls) != 5 || !strings.Contains(sqls[0], "DELETE FROM catalog") ||
		!strings.Contains(sqls[4], "catalog_meta") {
		t.Fatalf("statement order wrong: %q", sqls)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["count"] != float64(3) || out["ok"] != true {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}
