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

func tursoOKPayload(n int) []byte {
	items := make([]interface{}, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, map[string]interface{}{
			"type": "execute",
			"response": map[string]interface{}{
				"result": map[string]interface{}{
					"columns": []interface{}{}, "rows": []interface{}{},
					"affected_row_count": 1,
				},
			},
		})
	}
	b, _ := json.Marshal(map[string]interface{}{"results": items})
	return b
}

func tursoVersionPayload(v int64) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"results": []interface{}{map[string]interface{}{
			"type": "execute",
			"response": map[string]interface{}{
				"result": map[string]interface{}{
					"columns": []interface{}{}, "rows": []interface{}{[]interface{}{v}},
					"affected_row_count": 0,
				},
			},
		}},
	})
	return b
}

// mockCatalogTurso records the SQL of every pipeline POST. The version
// preflight is answered with currentVersion; every other POST is a batch.
func mockCatalogTurso(t *testing.T, currentVersion int64, posts *int64, mu *sync.Mutex, captured *[][]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(posts, 1)
		var body lib.TursoPipelineBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode turso request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sqls := make([]string, 0, len(body.Requests))
		for _, req := range body.Requests {
			sqls = append(sqls, req.Stmt.SQL)
		}
		mu.Lock()
		*captured = append(*captured, sqls)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if len(body.Requests) == 1 && strings.Contains(body.Requests[0].Stmt.SQL, "SELECT version FROM catalog_meta") {
			w.Write(tursoVersionPayload(currentVersion))
			return
		}
		w.Write(tursoOKPayload(len(body.Requests)))
	}))
}

func setupMockTurso(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "products-manage-test")
	if tr, ok := lib.NewTurso().HTTP.Transport.(*http.Transport); ok {
		t.Cleanup(tr.CloseIdleConnections)
	}
}

func TestPutBatchesAllUpdatesIntoOneRoundTrip(t *testing.T) {
	var posts int64
	var mu sync.Mutex
	var captured [][]string
	srv := mockCatalogTurso(t, 7, &posts, &mu, &captured)
	defer srv.Close()
	setupMockTurso(t, srv)

	body := `{"name":"New name","description":"Desc","purchasePrice":"10.00","salePrice":"15.00","vatRate":"20","stock":5,"active":true,"category":"Cat","brand":"Brand","baseCatalogVersion":7}`
	req := httptest.NewRequest(http.MethodPut, "/api/products-manage?sku=S1", strings.NewReader(body))
	rec := httptest.NewRecorder()
	Handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var out map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["ok"] != true {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}

	gotPosts := atomic.LoadInt64(&posts)
	t.Logf("product PUT: posts=%d statements=9 (was 11)", gotPosts)
	if gotPosts != 2 {
		t.Fatalf("posts=%d, want 2 (preflight version + 1 batch, not 11)", gotPosts)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 2 {
		t.Fatalf("captured %d POSTs, want 2", len(captured))
	}
	if len(captured[0]) != 1 || !strings.Contains(captured[0][0], "SELECT version FROM catalog_meta") {
		t.Fatalf("first POST should be the version preflight: %q", captured[0])
	}
	if len(captured[1]) != 10 {
		t.Fatalf("batch has %d statements, want 10 (9 updates + meta bump): %q", len(captured[1]), captured[1])
	}
	columns := []string{"name", "description", "purchase_price", "sale_price", "vat_rate", "stock", "active", "category", "brand"}
	for i, col := range columns {
		if !strings.Contains(captured[1][i], "UPDATE catalog SET "+col+"=? WHERE sku=?") {
			t.Fatalf("batch statement %d wrong: %q", i, captured[1][i])
		}
	}
	if !strings.Contains(captured[1][9], "catalog_meta") {
		t.Fatalf("last batch statement should bump catalog_meta: %q", captured[1][9])
	}
}

func TestPutConflictStillReturns409(t *testing.T) {
	var posts int64
	var mu sync.Mutex
	var captured [][]string
	srv := mockCatalogTurso(t, 99, &posts, &mu, &captured)
	defer srv.Close()
	setupMockTurso(t, srv)

	body := `{"name":"New name","baseCatalogVersion":7}`
	req := httptest.NewRequest(http.MethodPut, "/api/products-manage?sku=S1", strings.NewReader(body))
	rec := httptest.NewRecorder()
	Handler(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("got %d, want 409: %s", rec.Code, rec.Body.String())
	}
	var out map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["error"] != "conflict: catalog changed on another device" || out["currentCatalogVersion"] != float64(99) {
		t.Fatalf("unexpected conflict response: %s", rec.Body.String())
	}
	if got := atomic.LoadInt64(&posts); got != 1 {
		t.Fatalf("posts=%d, want 1 (no batch on conflict)", got)
	}
}

func TestDeleteBatchesArchiveAndVersionBump(t *testing.T) {
	var posts int64
	var mu sync.Mutex
	var captured [][]string
	srv := mockCatalogTurso(t, 7, &posts, &mu, &captured)
	defer srv.Close()
	setupMockTurso(t, srv)

	req := httptest.NewRequest(http.MethodDelete, "/api/products-manage?sku=S1&baseCatalogVersion=7", nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := atomic.LoadInt64(&posts); got != 2 {
		t.Fatalf("posts=%d, want 2 (preflight version + 1 batch)", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 2 || len(captured[1]) != 2 {
		t.Fatalf("want preflight + 2-statement batch, got %q", captured)
	}
	if !strings.Contains(captured[1][0], "UPDATE catalog SET active=0 WHERE sku=?") ||
		!strings.Contains(captured[1][1], "catalog_meta") {
		t.Fatalf("unexpected batch SQL: %q", captured[1])
	}
}
