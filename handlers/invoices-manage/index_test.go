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

// invoicePipelineServer is a minimal Turso /v2/pipeline stub. It records each
// POST and the SQL statements inside it so the tests can prove the per-line
// N+1 loop is gone.
type invoicePipelineServer struct {
	posts   int64
	mu      sync.Mutex
	sqls    [][]string
	stock   map[string]int
	names   map[string]string
	omitSKU string
}

func (s *invoicePipelineServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		case strings.Contains(sql, "FROM invoices WHERE id") && strings.Contains(sql, "invoice_number"):
			rows = []interface{}{[]interface{}{"inv-1", "INV-0001", "cust-1", "Draft", "10000", "store", nil, "", nil, nil}}
		case strings.Contains(sql, "FROM invoice_items WHERE invoice_id"):
			rows = []interface{}{
				[]interface{}{"SKU-1", 1, 12},
				[]interface{}{"SKU-2", 2, 0},
				[]interface{}{"SKU-3", 3, 24},
			}
		case strings.Contains(sql, "SELECT stock, name FROM catalog WHERE sku"):
			sku := ""
			if len(req.Stmt.Args) > 0 {
				sku = lib.ToString(req.Stmt.Args[0].Value)
			}
			stock, ok := s.stock[sku]
			if ok && sku != s.omitSKU {
				name := s.names[sku]
				if name == "" {
					name = "Name-" + sku
				}
				rows = []interface{}{[]interface{}{stock, name}}
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

func newInvoicePipelineDB(t *testing.T, s *invoicePipelineServer) *lib.TursoClient {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "invoices-batch-test")
	db := lib.NewTurso()
	if tr, ok := db.HTTP.Transport.(*http.Transport); ok {
		t.Cleanup(tr.CloseIdleConnections)
	}
	return db
}

func TestValidateInvoiceUsesConstantRoundTrips(t *testing.T) {
	s := &invoicePipelineServer{
		stock: map[string]int{"SKU-1": 5, "SKU-2": 5, "SKU-3": 5},
		names: map[string]string{"SKU-2": "Beta"},
	}
	db := newInvoicePipelineDB(t, s)

	if err := validateInvoice(context.Background(), db, "inv-1"); err != nil {
		t.Fatalf("validateInvoice: %v", err)
	}
	gotPosts := atomic.LoadInt64(&s.posts)
	t.Logf("invoice validate: posts=%d lines=3 (was 13)", gotPosts)
	if gotPosts != 3 {
		t.Fatalf("pipeline POSTs = %d, want 3 for 3 lines (read + stock checks + writes)", gotPosts)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sqls) != 3 || len(s.sqls[1]) != 3 {
		t.Fatalf("unexpected pipeline batches: %q", s.sqls)
	}
	for _, sql := range s.sqls[1] {
		if !strings.Contains(sql, "SELECT stock, name FROM catalog") {
			t.Fatalf("stock check batch contains %q", sql)
		}
	}
	var stockUpdates, warranties int
	for _, sql := range s.sqls[2] {
		switch {
		case strings.HasPrefix(sql, "UPDATE catalog SET stock = stock -"):
			stockUpdates++
		case strings.Contains(sql, "UPDATE invoice_items SET warranty_end_date"):
			warranties++
		case strings.Contains(sql, "UPDATE invoices SET state"),
			strings.Contains(sql, "INSERT INTO journal_entries"),
			strings.Contains(sql, "INSERT INTO catalog_meta"):
		default:
			t.Errorf("unexpected write in batch: %q", sql)
		}
	}
	if stockUpdates != 3 || warranties != 2 || len(s.sqls[2]) != 8 {
		t.Fatalf("write batch = %q (stock=%d, warranty=%d)", s.sqls[2], stockUpdates, warranties)
	}
}

func TestValidateInvoiceInsufficientStockPreservesErrorAndNoWrites(t *testing.T) {
	s := &invoicePipelineServer{
		stock: map[string]int{"SKU-1": 9, "SKU-2": 1, "SKU-3": 9},
		names: map[string]string{"SKU-2": "Beta"},
	}
	db := newInvoicePipelineDB(t, s)

	err := validateInvoice(context.Background(), db, "inv-1")
	if err == nil || err.Error() != "insufficient stock for Beta (SKU-2)" {
		t.Fatalf("err = %v, want %q", err, "insufficient stock for Beta (SKU-2)")
	}
	if got := atomic.LoadInt64(&s.posts); got != 2 {
		t.Fatalf("pipeline POSTs = %d, want 2 (failed check must not send writes)", got)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, batch := range s.sqls {
		for _, sql := range batch {
			if strings.HasPrefix(sql, "UPDATE ") || strings.HasPrefix(sql, "INSERT ") {
				t.Fatalf("mutation sent before all stock checks passed: %q", sql)
			}
		}
	}
}

func TestValidateInvoiceMissingProductPreservesError(t *testing.T) {
	s := &invoicePipelineServer{
		stock:   map[string]int{"SKU-1": 9, "SKU-2": 9, "SKU-3": 9},
		omitSKU: "SKU-3",
	}
	db := newInvoicePipelineDB(t, s)

	err := validateInvoice(context.Background(), db, "inv-1")
	if err == nil || err.Error() != "product not found: SKU-3" {
		t.Fatalf("err = %v, want %q", err, "product not found: SKU-3")
	}
	if got := atomic.LoadInt64(&s.posts); got != 2 {
		t.Fatalf("pipeline POSTs = %d, want 2", got)
	}
}
