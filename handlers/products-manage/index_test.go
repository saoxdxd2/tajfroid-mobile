package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"unicode"

	productshandler "tajfroid-mobile/handlers/products"
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

type recordedCalls struct {
	mu    sync.Mutex
	calls [][]lib.TursoStmt
}

func (r *recordedCalls) add(stmts []lib.TursoStmt) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyOf := append([]lib.TursoStmt(nil), stmts...)
	r.calls = append(r.calls, copyOf)
}

func (r *recordedCalls) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func (r *recordedCalls) snapshot() [][]lib.TursoStmt {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]lib.TursoStmt(nil), r.calls...)
}

func tursoRowsPayload(rows []interface{}) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"results": []interface{}{map[string]interface{}{
			"type": "execute",
			"response": map[string]interface{}{
				"result": map[string]interface{}{
					"columns": []interface{}{}, "rows": rows,
					"affected_row_count": 0,
				},
			},
		}},
	})
	return b
}

func tursoErrorPayload(n int, message string) []byte {
	results := make([]interface{}, 0, n)
	for i := 0; i < n; i++ {
		results = append(results, map[string]interface{}{
			"type":     "execute",
			"response": map[string]interface{}{},
			"error":    map[string]interface{}{"message": message},
		})
	}
	b, _ := json.Marshal(map[string]interface{}{"results": results})
	return b
}

func mockManageTurso(t *testing.T, version int64, rec *recordedCalls, batchErr string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body lib.TursoPipelineBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode pipeline: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		stmts := make([]lib.TursoStmt, 0, len(body.Requests))
		for _, req := range body.Requests {
			stmts = append(stmts, req.Stmt)
		}
		rec.add(stmts)
		w.Header().Set("Content-Type", "application/json")
		if len(stmts) == 1 && strings.Contains(stmts[0].SQL, "SELECT version FROM catalog_meta") {
			w.Write(tursoVersionPayload(version))
			return
		}
		if len(stmts) == 1 && strings.Contains(stmts[0].SQL, "FROM catalog WHERE sku = ?") {
			w.Write(tursoRowsPayload([]interface{}{
				[]interface{}{"S1", "Name", nil, "10", "12", "20", 5, 1, "Cat", "Brand"},
			}))
			return
		}
		if batchErr != "" {
			w.Write(tursoErrorPayload(len(stmts), batchErr))
			return
		}
		w.Write(tursoOKPayload(len(stmts)))
	}))
	setupMockTurso(t, srv)
	return srv
}

func TestProductManageSKUIsValidatedAndParameterized(t *testing.T) {
	var rec recordedCalls
	srv := mockManageTurso(t, 7, &rec, "")
	defer srv.Close()

	const injection = "X'); DROP TABLE catalog; --"

	t.Run("injection stays a bound parameter on GET and DELETE", func(t *testing.T) {
		before := rec.len()
		req := httptest.NewRequest(http.MethodGet, "/api/products-manage?sku="+url.QueryEscape(injection), nil)
		res := httptest.NewRecorder()
		Handler(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("GET got %d, want 200: %s", res.Code, res.Body.String())
		}
		var out map[string]interface{}
		if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"sku", "name", "description", "purchasePrice", "salePrice", "vatRate", "stock", "active", "category", "brand", "catalogVersion"} {
			if _, ok := out[key]; !ok {
				t.Fatalf("GET response missing %q: %s", key, res.Body.String())
			}
		}
		calls := rec.snapshot()
		if len(calls) != before+2 {
			t.Fatalf("calls=%d, want version lookup + product lookup", len(calls)-before)
		}
		for _, stmts := range calls[before:] {
			for _, stmt := range stmts {
				if strings.Contains(stmt.SQL, "DROP TABLE") {
					t.Fatalf("hostile SKU interpolated into SQL: %q", stmt.SQL)
				}
				for _, arg := range stmt.Args {
					if lib.ToString(arg.Value) == injection {
						goto found
					}
				}
			}
		}
		t.Fatal("hostile SKU was not passed as a bound parameter")
	found:
		before = rec.len()
		req = httptest.NewRequest(http.MethodDelete, "/api/products-manage?sku="+url.QueryEscape(injection)+"&baseCatalogVersion=7", nil)
		res = httptest.NewRecorder()
		Handler(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("DELETE got %d, want 200: %s", res.Code, res.Body.String())
		}
		calls = rec.snapshot()
		if len(calls) != before+2 {
			t.Fatalf("calls=%d, want version lookup + archive batch", len(calls)-before)
		}
		for _, stmt := range calls[len(calls)-1] {
			if strings.Contains(stmt.SQL, "DROP TABLE") {
				t.Fatalf("hostile SKU interpolated into SQL: %q", stmt.SQL)
			}
		}
	})

	for _, tc := range []struct {
		name string
		sku  string
	}{
		{"empty", ""},
		{"whitespace", "   "},
		{"nul byte", "A\x00B"},
		{"control char", "A\x1fB"},
		{"too long", strings.Repeat("S", 129)},
	} {
		t.Run(tc.name+" sku rejected", func(t *testing.T) {
			before := rec.len()
			req := httptest.NewRequest(http.MethodGet, "/api/products-manage?sku="+url.QueryEscape(tc.sku), nil)
			res := httptest.NewRecorder()
			Handler(res, req)
			if res.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400: %s", res.Code, res.Body.String())
			}
			if got := rec.len(); got != before {
				t.Fatalf("invalid SKU reached Turso: calls %d -> %d", before, got)
			}
		})
	}
}

func TestProductPutRejectsImpossibleDataBeforeWriting(t *testing.T) {
	var rec recordedCalls
	srv := mockManageTurso(t, 7, &rec, "")
	defer srv.Close()

	cases := []struct {
		name string
		body string
	}{
		{"negative stock", `{"baseCatalogVersion":7,"stock":-1}`},
		{"absurd stock", `{"baseCatalogVersion":7,"stock":1000000000000}`},
		{"stock as exponent", `{"baseCatalogVersion":7,"stock":1e100}`},
		{"fractional stock", `{"baseCatalogVersion":7,"stock":1.5}`},
		{"NaN price", `{"baseCatalogVersion":7,"salePrice":"NaN"}`},
		{"Infinity purchase price", `{"baseCatalogVersion":7,"purchasePrice":"Infinity"}`},
		{"negative vat", `{"baseCatalogVersion":7,"vatRate":"-1"}`},
		{"invalid active", `{"baseCatalogVersion":7,"active":"maybe"}`},
		{"active out of range", `{"baseCatalogVersion":7,"active":2}`},
		{"long name", fmt.Sprintf(`{"baseCatalogVersion":7,"name":%q}`, strings.Repeat("N", 513))},
		{"malformed base version", `{"baseCatalogVersion":"seven","stock":1}`},
		{"malformed json", `{"baseCatalogVersion":7,`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := rec.len()
			req := httptest.NewRequest(http.MethodPut, "/api/products-manage?sku=S1", strings.NewReader(tc.body))
			res := httptest.NewRecorder()
			Handler(res, req)
			if res.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400: %s", res.Code, res.Body.String())
			}
			wantCalls := before + 1 // version preflight for well-formed JSON
			if strings.Contains(tc.name, "malformed") {
				wantCalls = before
			}
			if got := rec.len(); got != wantCalls {
				t.Fatalf("calls=%d, want %d (no batch on invalid input)", got, wantCalls)
			}
		})
	}
}

func TestProductManageDeleteVersionAndConflictSemantics(t *testing.T) {
	var rec recordedCalls
	srv := mockManageTurso(t, 7, &rec, "")
	defer srv.Close()

	t.Run("mismatched version is still 409", func(t *testing.T) {
		before := rec.len()
		req := httptest.NewRequest(http.MethodDelete, "/api/products-manage?sku=S1&baseCatalogVersion=8", nil)
		res := httptest.NewRecorder()
		Handler(res, req)
		if res.Code != http.StatusConflict {
			t.Fatalf("got %d, want 409: %s", res.Code, res.Body.String())
		}
		var out map[string]interface{}
		if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out["error"] != "conflict: catalog changed on another device" || out["currentCatalogVersion"] != float64(7) {
			t.Fatalf("unexpected conflict body: %s", res.Body.String())
		}
		if got := rec.len(); got != before+1 {
			t.Fatalf("calls=%d, want only the version preflight", got-before)
		}
	})

	t.Run("malformed base version is 400", func(t *testing.T) {
		before := rec.len()
		req := httptest.NewRequest(http.MethodDelete, "/api/products-manage?sku=S1&baseCatalogVersion=NaN", nil)
		res := httptest.NewRecorder()
		Handler(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400: %s", res.Code, res.Body.String())
		}
		if got := rec.len(); got != before {
			t.Fatalf("malformed version reached Turso: calls %d -> %d", before, got)
		}
	})
}

func TestProductManageDoesNotSwallowBatchErrors(t *testing.T) {
	var rec recordedCalls
	srv := mockManageTurso(t, 7, &rec, "disk full")
	defer srv.Close()

	for _, tc := range []struct {
		name, method, body, target string
	}{
		{"put", http.MethodPut, `{"baseCatalogVersion":7,"name":"New"}`, "/api/products-manage?sku=S1"},
		{"delete", http.MethodDelete, "", "/api/products-manage?sku=S1&baseCatalogVersion=7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := rec.len()
			req := httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body))
			res := httptest.NewRecorder()
			Handler(res, req)
			if res.Code != http.StatusInternalServerError {
				t.Fatalf("got %d, want 500: %s", res.Code, res.Body.String())
			}
			if !strings.Contains(res.Body.String(), "disk full") || strings.Contains(res.Body.String(), `"ok":true`) {
				t.Fatalf("real error was hidden: %s", res.Body.String())
			}
			if got := rec.len(); got != before+2 {
				t.Fatalf("calls=%d, want version lookup + batch", got-before)
			}
		})
	}
}

type productsMockOptions struct {
	existingSKU bool
	batchError  string
}

func mockProductsTurso(t *testing.T, opts productsMockOptions, rec *recordedCalls) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body lib.TursoPipelineBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode pipeline: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		stmts := make([]lib.TursoStmt, 0, len(body.Requests))
		for _, req := range body.Requests {
			stmts = append(stmts, req.Stmt)
		}
		rec.add(stmts)
		w.Header().Set("Content-Type", "application/json")
		if len(stmts) == 1 && strings.Contains(stmts[0].SQL, "SELECT sku, name FROM catalog WHERE sku=?") {
			if opts.existingSKU {
				w.Write(tursoRowsPayload([]interface{}{[]interface{}{"S1", "Existing"}}))
				return
			}
			w.Write(tursoRowsPayload([]interface{}{}))
			return
		}
		if opts.batchError != "" {
			w.Write(tursoErrorPayload(len(stmts), opts.batchError))
			return
		}
		if len(stmts) == 1 && strings.Contains(stmts[0].SQL, "SELECT version FROM catalog_meta") {
			w.Write(tursoVersionPayload(7))
			return
		}
		w.Write(tursoOKPayload(len(stmts)))
	}))
	setupMockTurso(t, srv)
	return srv
}

func postProduct(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/products", strings.NewReader(body))
	res := httptest.NewRecorder()
	productshandler.Handler(res, req)
	return res
}

func TestProductCreateRejectsHostileFields(t *testing.T) {
	var rec recordedCalls
	srv := mockProductsTurso(t, productsMockOptions{}, &rec)
	defer srv.Close()

	cases := []struct {
		name, body string
	}{
		{"empty sku", `{"sku":"","name":"Name"}`},
		{"control sku", `{"sku":"A\u0000B","name":"Name"}`},
		{"empty name", `{"sku":"S1","name":""}`},
		{"long name", fmt.Sprintf(`{"sku":"S1","name":%q}`, strings.Repeat("N", 513))},
		{"negative stock", `{"sku":"S1","name":"Name","stock":-1}`},
		{"NaN price", `{"sku":"S1","name":"Name","salePrice":"NaN"}`},
		{"Infinity price", `{"sku":"S1","name":"Name","salePrice":"Infinity"}`},
		{"negative price", `{"sku":"S1","name":"Name","purchasePrice":"-2"}`},
		{"invalid active", `{"sku":"S1","name":"Name","active":"maybe"}`},
		{"malformed json", `{"sku":"S1",`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := rec.len()
			res := postProduct(t, tc.body)
			if res.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400: %s", res.Code, res.Body.String())
			}
			if got := rec.len(); got != before {
				t.Fatalf("invalid product reached Turso: calls %d -> %d", before, got)
			}
		})
	}
}

func TestProductCreateSanitizesAndParameterizes(t *testing.T) {
	var rec recordedCalls
	srv := mockProductsTurso(t, productsMockOptions{}, &rec)
	defer srv.Close()

	const injection = "P'); DROP TABLE catalog; --"
	payload, err := json.Marshal(map[string]interface{}{
		"sku":           injection,
		"name":          "\x01Produit 🧊",
		"description":   "desc\x07",
		"purchasePrice": "1e1",
		"salePrice":     "1e2",
		"vatRate":       "20.0",
		"stock":         "2e1",
		"active":        "yes",
		"category":      nil,
		"brand":         "Marque\x1fQ",
	})
	if err != nil {
		t.Fatal(err)
	}

	res := postProduct(t, string(payload))
	if res.Code != http.StatusCreated {
		t.Fatalf("got %d, want 201: %s", res.Code, res.Body.String())
	}
	var out map[string]interface{}
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out["sku"] != injection || out["name"] != "Produit 🧊" {
		t.Fatalf("response shape changed: %s", res.Body.String())
	}
	calls := rec.snapshot()
	if len(calls) != 2 || len(calls[0]) != 1 || len(calls[1]) != 2 {
		t.Fatalf("want idempotency lookup + insert/meta batch, got %d calls", len(calls))
	}
	for _, stmts := range calls {
		for _, stmt := range stmts {
			if strings.Contains(stmt.SQL, "DROP TABLE") {
				t.Fatalf("hostile value interpolated into SQL: %q", stmt.SQL)
			}
			for _, arg := range stmt.Args {
				s := lib.ToString(arg.Value)
				for _, r := range s {
					if unicode.IsControl(r) {
						t.Fatalf("control character reached Turso: %U", r)
					}
				}
			}
		}
	}
	insert := calls[1][0]
	args := insert.Args
	if lib.ToString(args[0].Value) != injection || lib.ToString(args[1].Value) != "Produit 🧊" {
		t.Fatalf("sku/name args wrong: %#v", args[:2])
	}
	if lib.ToString(args[3].Value) != "10" || lib.ToString(args[4].Value) != "100" || lib.ToString(args[5].Value) != "20" {
		t.Fatalf("numeric strings not canonicalized: %#v", args[3:6])
	}
	if lib.ToInt64(args[6].Value) != 20 || lib.ToInt64(args[7].Value) != 1 {
		t.Fatalf("stock/active wrong: %#v", args[6:8])
	}
	if args[8].Type != "null" || lib.ToString(args[9].Value) != "MarqueQ" {
		t.Fatalf("nullable/sanitized text wrong: %#v", args[8:10])
	}
}

func TestProductCreateUNIQUEStillReturns409(t *testing.T) {
	var rec recordedCalls
	srv := mockProductsTurso(t, productsMockOptions{batchError: "UNIQUE constraint failed: catalog.sku"}, &rec)
	defer srv.Close()

	res := postProduct(t, `{"sku":"S1","name":"Name"}`)
	if res.Code != http.StatusConflict {
		t.Fatalf("got %d, want 409: %s", res.Code, res.Body.String())
	}
	var out map[string]interface{}
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 || out["sku"] != "S1" || out["idempotent"] != true || !strings.Contains(lib.ToString(out["error"]), "UNIQUE") {
		t.Fatalf("conflict response shape changed: %s", res.Body.String())
	}
}

func TestProductListResponseShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body lib.TursoPipelineBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode pipeline: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if len(body.Requests) == 1 && strings.Contains(body.Requests[0].Stmt.SQL, "SELECT version FROM catalog_meta") {
			w.Write(tursoVersionPayload(9))
			return
		}
		w.Write(tursoRowsPayload([]interface{}{
			[]interface{}{"S1", "Alpha", "desc", "10", "12", "20", 4, 1, "Cat", "Brand"},
		}))
	}))
	defer srv.Close()
	setupMockTurso(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	res := httptest.NewRecorder()
	productshandler.Handler(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", res.Code, res.Body.String())
	}
	var out map[string]interface{}
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out["catalogVersion"] != float64(9) {
		t.Fatalf("top-level shape changed: %s", res.Body.String())
	}
	items, ok := out["products"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("products shape changed: %s", res.Body.String())
	}
	item, ok := items[0].(map[string]interface{})
	if !ok || len(item) != 10 || item["sku"] != "S1" || item["stock"] != float64(4) || item["active"] != true {
		t.Fatalf("product shape changed: %#v", items[0])
	}
}
