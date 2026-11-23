package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"unicode"

	"tajfroid-mobile/lib"
)

func catalogMetaPayload(version, revision int64) []byte {
	b, _ := json.Marshal(map[string]interface{}{"results": []interface{}{
		map[string]interface{}{
			"type": "execute",
			"response": map[string]interface{}{
				"result": map[string]interface{}{
					"columns": []interface{}{},
					"rows": []interface{}{[]interface{}{
						map[string]interface{}{"type": "integer", "value": version},
						map[string]interface{}{"type": "integer", "value": revision},
					}},
					"affected_row_count": 0,
				},
			},
		},
	}})
	return b
}

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
	if gotPosts != 2 {
		t.Fatalf("posts=%d, want 2 (revision read + one write pipeline)", gotPosts)
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
	if version, ok := out["version"].(float64); !ok || version <= 0 {
		t.Fatalf("missing monotonic version in response: %s", rec.Body.String())
	}
	if revision, ok := out["revision"].(float64); !ok || revision <= 0 {
		t.Fatalf("missing revision in response: %s", rec.Body.String())
	}
}

func catalogMock(t *testing.T, onBatch func([]lib.TursoStmt)) (*httptest.Server, *int64) {
	t.Helper()
	var posts int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&posts, 1)
		var body lib.TursoPipelineBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode pipeline: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		isMetaRead := len(body.Requests) == 1 &&
			strings.Contains(body.Requests[0].Stmt.SQL, "SELECT version, updated_at")
		if isMetaRead {
			w.Header().Set("Content-Type", "application/json")
			w.Write(catalogMetaPayload(0, 0))
			return
		}
		if onBatch != nil {
			stmts := make([]lib.TursoStmt, 0, len(body.Requests))
			for _, req := range body.Requests {
				stmts = append(stmts, req.Stmt)
			}
			onBatch(stmts)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(catalogPayload(len(body.Requests)))
	}))
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "catalog-hardening-test")
	return srv, &posts
}

func putCatalog(t *testing.T, payload interface{}) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/agent/catalog", strings.NewReader(string(b)))
	rec := httptest.NewRecorder()
	Handler(rec, req)
	return rec
}

func assertNoControlRunes(t *testing.T, s string) {
	t.Helper()
	for _, r := range s {
		if unicode.IsControl(r) {
			t.Fatalf("control character %U survived sanitization", r)
		}
	}
}

func TestCatalogRebuildSanitizesAndDeduplicates1000Products(t *testing.T) {
	const total = 1000
	const duplicates = 20

	products := make([]map[string]interface{}, 0, total)
	for i := 0; i < total; i++ {
		sku := fmt.Sprintf("SKU-%04d", i)
		if i >= total-duplicates {
			sku = fmt.Sprintf("SKU-%04d", i-duplicates) // deterministic duplicate
		}
		name := "Produit " + string(rune(0x3B2)) + " 🧊 " + fmt.Sprint(i)
		switch i % 5 {
		case 0:
			name = "\x00\x1f" + name + "\x7f" // control chars must be stripped
		case 1:
			name = name + " " + strings.Repeat("λ", 400) // long but within limit
		}
		p := map[string]interface{}{
			"sku":        sku,
			"name":       name,
			"sale_price": "10.50",
			"vat_rate":   "20.0",
			"stock":      "3.0",
			"active":     true,
			"category":   "Cat\x00égorie",
			"brand":      "Marque",
		}
		switch i % 4 {
		case 0:
			p["sale_price"] = "1e2" // exponent notation
			p["stock"] = "1e1"
			p["active"] = "Y"
		case 1:
			p["vat_rate"] = 20 // numeric value
			p["active"] = 0
		case 2:
			delete(p, "category") // missing optional fields
			delete(p, "brand")
			delete(p, "active")
		case 3:
			p["sale_price"] = 12.25 // JSON number
			p["stock"] = 7
			p["active"] = "true"
		}
		products = append(products, p)
	}

	var mu sync.Mutex
	var captured [][]lib.TursoStmt
	srv, posts := catalogMock(t, func(stmts []lib.TursoStmt) {
		mu.Lock()
		defer mu.Unlock()
		captured = append(captured, stmts)
	})
	defer srv.Close()

	rec := putCatalog(t, map[string]interface{}{"version": 42, "products": products})
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var out map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["ok"] != true || out["count"] != float64(total-duplicates) {
		t.Fatalf("response shape/count wrong: %s", rec.Body.String())
	}
	if got := atomic.LoadInt64(posts); got != 2 {
		t.Fatalf("posts=%d, want 2 (revision read + write pipeline)", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 1 {
		t.Fatalf("captured %d batches, want 1", len(captured))
	}
	stmts := captured[0]
	if len(stmts) != total-duplicates+2 {
		t.Fatalf("statements=%d, want %d", len(stmts), total-duplicates+2)
	}
	if !strings.HasPrefix(stmts[0].SQL, "DELETE FROM catalog") {
		t.Fatalf("first statement should clear the catalog: %q", stmts[0].SQL)
	}
	if !strings.Contains(stmts[len(stmts)-1].SQL, "catalog_meta") {
		t.Fatalf("last statement should bump the version: %q", stmts[len(stmts)-1].SQL)
	}
	seen := make(map[string]bool, total-duplicates)
	for i, stmt := range stmts[1 : len(stmts)-1] {
		if stmt.SQL != catalogInsertSQL {
			t.Fatalf("statement %d is not the parameterized insert: %q", i+1, stmt.SQL)
		}
		if len(stmt.Args) != 8 {
			t.Fatalf("statement %d has %d args, want 8", i+1, len(stmt.Args))
		}
		sku := lib.ToString(stmt.Args[0].Value)
		if seen[sku] {
			t.Fatalf("duplicate SKU in batch: %q", sku)
		}
		seen[sku] = true
		for _, arg := range stmt.Args {
			if arg.Type == "text" {
				assertNoControlRunes(t, lib.ToString(arg.Value))
			}
		}
		price := lib.ToString(stmt.Args[2].Value)
		if price == "NaN" || price == "Infinity" || strings.HasPrefix(price, "-") {
			t.Fatalf("unsafe price %q reached the batch", price)
		}
		stock := lib.ToInt64(stmt.Args[4].Value)
		if stock < 0 {
			t.Fatalf("negative stock %d reached the batch", stock)
		}
		if strings.HasSuffix(sku, "0000") {
			if price != "100" { // 1e2 canonicalized
				t.Fatalf("exponent price not canonicalized: %q", price)
			}
			if stock != 10 { // 1e1 canonicalized
				t.Fatalf("exponent stock not canonicalized: %d", stock)
			}
			if lib.ToInt64(stmt.Args[5].Value) != 1 { // "Y"
				t.Fatal("active=Y was not accepted")
			}
		}
	}
	if len(seen) != total-duplicates {
		t.Fatalf("unique SKUs=%d, want %d", len(seen), total-duplicates)
	}
}

func TestCatalogRebuildRejectsImpossibleDataWithoutWriting(t *testing.T) {
	valid := func() map[string]interface{} {
		return map[string]interface{}{
			"sku": "S1", "name": "Name", "sale_price": "10", "vat_rate": "20",
			"stock": 1, "active": true,
		}
	}
	tooLong := func(n int) string { return strings.Repeat("x", n) }
	cases := []struct {
		name    string
		version interface{}
		mutate  func(map[string]interface{})
		rawBody string
	}{
		{name: "empty sku", mutate: func(p map[string]interface{}) { p["sku"] = "" }},
		{name: "whitespace sku", mutate: func(p map[string]interface{}) { p["sku"] = "   " }},
		{name: "empty name", mutate: func(p map[string]interface{}) { p["name"] = "" }},
		{name: "control-only sku", mutate: func(p map[string]interface{}) { p["sku"] = "\u0000\u001f" }},
		{name: "long sku", mutate: func(p map[string]interface{}) { p["sku"] = tooLong(129) }},
		{name: "long name", mutate: func(p map[string]interface{}) { p["name"] = tooLong(513) }},
		{name: "long category", mutate: func(p map[string]interface{}) { p["category"] = tooLong(129) }},
		{name: "NaN price", mutate: func(p map[string]interface{}) { p["sale_price"] = "NaN" }},
		{name: "Infinity vat", mutate: func(p map[string]interface{}) { p["vat_rate"] = "Infinity" }},
		{name: "negative infinity price", mutate: func(p map[string]interface{}) { p["sale_price"] = "-Infinity" }},
		{name: "negative price", mutate: func(p map[string]interface{}) { p["sale_price"] = "-1" }},
		{name: "fractional stock", mutate: func(p map[string]interface{}) { p["stock"] = 1.5 }},
		{name: "negative stock", mutate: func(p map[string]interface{}) { p["stock"] = -4 }},
		{name: "absurd stock", mutate: func(p map[string]interface{}) { p["stock"] = "1e100" }},
		{name: "invalid active", mutate: func(p map[string]interface{}) { p["active"] = "maybe" }},
		{name: "active out of range", mutate: func(p map[string]interface{}) { p["active"] = 2 }},
		{name: "negative version", version: -1},
		{name: "malformed json", rawBody: `{"version":1,"products":[`},
	}

	srv, posts := catalogMock(t, nil)
	defer srv.Close()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := atomic.LoadInt64(posts)
			var rec *httptest.ResponseRecorder
			if tc.rawBody != "" {
				req := httptest.NewRequest(http.MethodPut, "/api/agent/catalog", strings.NewReader(tc.rawBody))
				rec = httptest.NewRecorder()
				Handler(rec, req)
			} else {
				p := valid()
				if tc.mutate != nil {
					tc.mutate(p)
				}
				version := tc.version
				if version == nil {
					version = 1
				}
				rec = putCatalog(t, map[string]interface{}{"version": version, "products": []interface{}{p}})
			}
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400: %s", rec.Code, rec.Body.String())
			}
			if rec.Body.Len() == 0 {
				t.Fatal("expected a clean error message")
			}
			if got := atomic.LoadInt64(posts); got != before {
				t.Fatalf("invalid payload wrote to Turso: posts %d -> %d", before, got)
			}
		})
	}
}

func TestCatalogRebuildParameterizesHostileValues(t *testing.T) {
	const injection = "SKU'); DROP TABLE catalog; --"
	const script = "<script>alert(1)</script>"

	var mu sync.Mutex
	var captured [][]lib.TursoStmt
	srv, _ := catalogMock(t, func(stmts []lib.TursoStmt) {
		mu.Lock()
		defer mu.Unlock()
		captured = append(captured, stmts)
	})
	defer srv.Close()

	rec := putCatalog(t, map[string]interface{}{
		"version": 3,
		"products": []interface{}{
			map[string]interface{}{
				"sku": injection, "name": script + "\x00", "sale_price": "10", "vat_rate": "20",
				"stock": 1, "active": true, "category": "' OR '1'='1", "brand": "B",
			},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 1 || len(captured[0]) != 3 {
		t.Fatalf("want delete + insert + meta, got %d batches", len(captured))
	}
	for _, stmt := range captured[0] {
		for _, hostile := range []string{"DROP TABLE", "script", "OR '1'='1"} {
			if strings.Contains(stmt.SQL, hostile) {
				t.Fatalf("hostile value interpolated into SQL: %q", stmt.SQL)
			}
		}
	}
	insert := captured[0][1]
	if got := lib.ToString(insert.Args[0].Value); got != injection {
		t.Fatalf("sku arg was altered: %q", got)
	}
	if got := lib.ToString(insert.Args[1].Value); got != script {
		t.Fatalf("name arg was not sanitized: %q", got)
	}
	if got := lib.ToString(insert.Args[6].Value); got != "' OR '1'='1" {
		t.Fatalf("category arg was altered: %q", got)
	}
}

type rebuildState struct {
	mu       sync.Mutex
	version  int64
	revision int64
	rows     map[string]struct{}
	dupes    int
	allSQL   []string
	expected map[string]struct{}
}

func TestConcurrentCatalogRebuildsAreMonotonicAndDeduplicated(t *testing.T) {
	const workers = 12
	state := &rebuildState{rows: make(map[string]struct{}), expected: make(map[string]struct{})}
	for i := 0; i < 10; i++ {
		state.expected[fmt.Sprintf("SKU-%02d", i)] = struct{}{}
	}
	for i := 0; i < workers; i++ {
		for j := 0; j < 5; j++ {
			state.expected[fmt.Sprintf("SKU-R%02d-%02d", i, j)] = struct{}{}
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body lib.TursoPipelineBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode pipeline: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if len(body.Requests) == 1 &&
			strings.Contains(body.Requests[0].Stmt.SQL, "SELECT version, updated_at") {
			state.mu.Lock()
			version, revision := state.version, state.revision
			state.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.Write(catalogMetaPayload(version, revision))
			return
		}
		// Lock per statement so concurrent rebuilds can interleave like they
		// can against a real pipeline.
		batchSeen := make(map[string]bool, len(body.Requests))
		for _, req := range body.Requests {
			state.mu.Lock()
			sql := req.Stmt.SQL
			state.allSQL = append(state.allSQL, sql)
			switch {
			case strings.HasPrefix(sql, "DELETE FROM catalog"):
				state.rows = make(map[string]struct{})
			case strings.HasPrefix(sql, "INSERT INTO catalog "):
				sku := lib.ToString(req.Stmt.Args[0].Value)
				if batchSeen[sku] {
					state.dupes++ // the handler must dedupe inside one payload
				}
				batchSeen[sku] = true
				state.rows[sku] = struct{}{} // ON CONFLICT(sku) DO UPDATE
			case strings.HasPrefix(sql, "INSERT INTO catalog_meta"):
				v := lib.ToInt64(req.Stmt.Args[0].Value)
				if v > state.version {
					state.version = v
				}
				if len(req.Stmt.Args) > 1 {
					rev := lib.ToInt64(req.Stmt.Args[1].Value)
					if rev > state.revision {
						state.revision = rev
					}
				}
			}
			state.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(catalogPayload(len(body.Requests)))
	}))
	defer srv.Close()
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "catalog-concurrency-test")

	type result struct {
		code int
		err  interface{}
	}
	results := make(chan result, workers)
	versions := []int{5, 9, 3, 11, 7, 2, 12, 8, 4, 10, 6, 1}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker, version int) {
			defer wg.Done()
			defer func() {
				if p := recover(); p != nil {
					results <- result{err: p}
				}
			}()
			products := make([]map[string]interface{}, 0, 15)
			for k := 0; k < 10; k++ {
				products = append(products, map[string]interface{}{
					"sku": fmt.Sprintf("SKU-%02d", k), "name": fmt.Sprintf("Shared %d", k),
					"sale_price": "1", "vat_rate": "20", "stock": 1, "active": true,
				})
			}
			for k := 0; k < 5; k++ {
				products = append(products, map[string]interface{}{
					"sku":  fmt.Sprintf("SKU-R%02d-%02d", worker, k),
					"name": "Worker product", "sale_price": "2", "vat_rate": "20",
					"stock": 2, "active": false,
				})
			}
			// Include a duplicate SKU with a later value in every rebuild.
			products = append(products, map[string]interface{}{
				"sku": "SKU-00", "name": "Last wins", "sale_price": "3", "vat_rate": "20",
				"stock": 3, "active": true,
			})
			payload, err := json.Marshal(map[string]interface{}{"version": version, "products": products})
			if err != nil {
				results <- result{err: err}
				return
			}
			req := httptest.NewRequest(http.MethodPut, "/api/agent/catalog", strings.NewReader(string(payload)))
			rec := httptest.NewRecorder()
			Handler(rec, req)
			results <- result{code: rec.Code}
		}(i, versions[i])
	}
	wg.Wait()
	close(results)
	for res := range results {
		if res.err != nil {
			t.Fatalf("panic during concurrent rebuild: %v", res.err)
		}
		if res.code != http.StatusOK {
			t.Fatalf("concurrent rebuild returned %d", res.code)
		}
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.version <= 0 || state.revision <= 0 {
		t.Fatalf("final version/revision not monotonic: version=%d revision=%d",
			state.version, state.revision)
	}
	if state.dupes != 0 {
		t.Fatalf("observed %d duplicate SKU inserts", state.dupes)
	}
	if len(state.rows) == 0 {
		t.Fatal("no catalog rows were written")
	}
	for sku := range state.rows {
		if _, ok := state.expected[sku]; !ok {
			t.Fatalf("unexpected SKU in final catalog: %q", sku)
		}
	}
	for _, sql := range state.allSQL {
		if strings.Contains(sql, "DROP") || strings.Contains(sql, "NaN") {
			t.Fatalf("unsafe SQL emitted: %q", sql)
		}
	}
}

func TestCatalogRebuildRejectsStaleRevisionAndAcceptsDecimalComma(t *testing.T) {
	var writes int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body lib.TursoPipelineBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if len(body.Requests) == 1 &&
			strings.Contains(body.Requests[0].Stmt.SQL, "SELECT version, updated_at") {
			w.Write(catalogMetaPayload(500, 100))
			return
		}
		atomic.AddInt64(&writes, 1)
		for _, stmt := range body.Requests {
			if strings.HasPrefix(stmt.Stmt.SQL, "INSERT INTO catalog ") &&
				lib.ToString(stmt.Stmt.Args[2].Value) != "10.50" {
				t.Errorf("decimal comma was not canonicalized: %q", lib.ToString(stmt.Stmt.Args[2].Value))
			}
		}
		w.Write(catalogPayload(len(body.Requests)))
	}))
	defer srv.Close()
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "catalog-revision-test")
	db := lib.NewTurso()
	if tr, ok := db.HTTP.Transport.(*http.Transport); ok {
		defer tr.CloseIdleConnections()
	}

	products := []interface{}{map[string]interface{}{
		"sku": "FR-1", "name": "Produit", "sale_price": "10,50",
		"vat_rate": "20,00", "stock": "1", "active": true,
	}}
	stale := putCatalog(t, map[string]interface{}{
		"version": 1, "baseRevision": 99, "products": products,
	})
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale baseRevision: got %d, want 409: %s", stale.Code, stale.Body.String())
	}
	if got := atomic.LoadInt64(&writes); got != 0 {
		t.Fatalf("stale payload wrote %d batches", got)
	}

	fresh := putCatalog(t, map[string]interface{}{
		"version": 1, "baseRevision": 100, "products": products,
	})
	if fresh.Code != http.StatusOK {
		t.Fatalf("current baseRevision: got %d, want 200: %s", fresh.Code, fresh.Body.String())
	}
	var out map[string]interface{}
	if err := json.Unmarshal(fresh.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if revision, ok := out["revision"].(float64); !ok || revision <= 100 {
		t.Fatalf("new revision not advanced: %s", fresh.Body.String())
	}
}
