package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"tajfroid-mobile/lib"
)

// purchasePipelineServer counts pipeline POSTs and records their SQL so the
// tests can prove existence checks and stock updates are batched, not per line.
type purchasePipelineServer struct {
	posts   int64
	mu      sync.Mutex
	sqls    [][]string
	catalog map[string]bool
}

func (s *purchasePipelineServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&s.posts, 1)
	var body lib.TursoPipelineBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sqls := make([]string, 0, len(body.Requests))
	for _, req := range body.Requests {
		sqls = append(sqls, req.Stmt.SQL)
	}
	s.mu.Lock()
	s.sqls = append(s.sqls, sqls)
	s.mu.Unlock()

	results := make([]interface{}, 0, len(body.Requests))
	for _, req := range body.Requests {
		sql := req.Stmt.SQL
		rows := []interface{}{}
		switch {
		case strings.Contains(sql, "FROM purchases WHERE id") && strings.Contains(sql, "reference"):
			rows = []interface{}{[]interface{}{"Draft", "PUR-0001", "sup-1", "2500"}}
		case strings.Contains(sql, "FROM purchase_items WHERE purchase_id"):
			rows = []interface{}{
				[]interface{}{"SKU-1", 1},
				[]interface{}{"SKU-2", 2},
				[]interface{}{"SKU-3", 3},
			}
		case strings.Contains(sql, "SELECT sku FROM catalog WHERE sku"):
			sku := ""
			if len(req.Stmt.Args) > 0 {
				sku = lib.ToString(req.Stmt.Args[0].Value)
			}
			if s.catalog[sku] {
				rows = []interface{}{[]interface{}{sku}}
			}
		}
		results = append(results, map[string]interface{}{
			"type": "execute",
			"response": map[string]interface{}{
				"result": map[string]interface{}{
					"columns":            []interface{}{},
					"rows":               rows,
					"affected_row_count": 1,
				},
			},
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"results": results})
}

func newPurchasePipelineDB(t *testing.T, s *purchasePipelineServer) *lib.TursoClient {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "purchases-batch-test")
	db := lib.NewTurso()
	if tr, ok := db.HTTP.Transport.(*http.Transport); ok {
		t.Cleanup(tr.CloseIdleConnections)
	}
	return db
}

func TestValidatePurchaseUsesConstantRoundTrips(t *testing.T) {
	s := &purchasePipelineServer{catalog: map[string]bool{"SKU-1": true, "SKU-2": true, "SKU-3": true}}
	db := newPurchasePipelineDB(t, s)

	if err := validatePurchase(context.Background(), db, "pur-1"); err != nil {
		t.Fatalf("validatePurchase: %v", err)
	}
	gotPosts := atomic.LoadInt64(&s.posts)
	t.Logf("purchase validate: posts=%d lines=3 (was 12)", gotPosts)
	if gotPosts != 3 {
		t.Fatalf("pipeline POSTs = %d, want 3 for 3 lines (read + existence checks + writes)", gotPosts)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sqls) != 3 || len(s.sqls[1]) != 3 {
		t.Fatalf("unexpected pipeline batches: %q", s.sqls)
	}
	for _, sql := range s.sqls[1] {
		if !strings.Contains(sql, "SELECT sku FROM catalog WHERE sku") {
			t.Fatalf("existence check batch contains %q", sql)
		}
	}
	var stockUpdates int
	for _, sql := range s.sqls[2] {
		switch {
		case strings.Contains(sql, "UPDATE catalog SET stock = stock +"):
			stockUpdates++
		case strings.Contains(sql, "UPDATE purchases SET state"),
			strings.Contains(sql, "INSERT INTO journal_entries"),
			strings.Contains(sql, "INSERT INTO catalog_meta"):
		default:
			t.Errorf("unexpected write in batch: %q", sql)
		}
	}
	if stockUpdates != 3 || len(s.sqls[2]) != 6 {
		t.Fatalf("write batch = %q (stock updates=%d)", s.sqls[2], stockUpdates)
	}
}

func TestValidatePurchaseMissingProductPreservesErrorAndNoWrites(t *testing.T) {
	s := &purchasePipelineServer{catalog: map[string]bool{"SKU-1": true, "SKU-3": true}}
	db := newPurchasePipelineDB(t, s)

	err := validatePurchase(context.Background(), db, "pur-1")
	if err == nil || err.Error() != "product not found: SKU-2" {
		t.Fatalf("err = %v, want %q", err, "product not found: SKU-2")
	}
	if got := atomic.LoadInt64(&s.posts); got != 2 {
		t.Fatalf("pipeline POSTs = %d, want 2 (failed check must not send writes)", got)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, batch := range s.sqls {
		for _, sql := range batch {
			if strings.HasPrefix(sql, "UPDATE ") || strings.HasPrefix(sql, "INSERT ") {
				t.Fatalf("mutation sent before all existence checks passed: %q", sql)
			}
		}
	}
}
