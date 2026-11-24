package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"tajfroid-mobile/lib"
)

func catalogGetPayload() []byte {
	return []byte(`{"results":[
		{"type":"execute","response":{"result":{"columns":[],"rows":[
			["A-1","Alpha 🧊","10.50","","20",2,1,"Cat",null],
			["B-2","Beta","12",null,"20",0,0,null,"Brand"],
			["C-3","Gamma","0",null,"20",1,1,null,null],
			["D-4","Delta","0","149.00","20",3,1,null,null],
			["E-5","Epsilon","10","0","20",1,1,null,null]
		],"affected_row_count":5}}}
	]}`)
}

func catalogVersionPayload(v int64) []byte {
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

func mockCatalogRead(t *testing.T, version int64, posts *int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(posts, 1)
		var body struct {
			Requests []struct {
				Stmt struct {
					SQL string `json:"sql"`
				} `json:"stmt"`
			} `json:"requests"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode pipeline: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if len(body.Requests) == 1 && strings.Contains(body.Requests[0].Stmt.SQL, "SELECT version FROM catalog_meta") {
			w.Write(catalogVersionPayload(version))
			return
		}
		if len(body.Requests) != 1 || !strings.Contains(body.Requests[0].Stmt.SQL, "pinned_price") {
			t.Errorf("catalog rows query must select pinned_price: %#v", body.Requests)
		}
		w.Write(catalogGetPayload())
	}))
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "catalog-read-test")
	return srv
}

func assertJSONKeys(t *testing.T, raw []byte, want ...string) map[string]interface{} {
	t.Helper()
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	got := make([]string, 0, len(obj))
	for k := range obj {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("keys=%v, want %v (body %s)", got, want, raw)
	}
	return obj
}

func TestCatalogGetResponseShapeAndNotModified(t *testing.T) {
	var posts int64
	srv := mockCatalogRead(t, 7, &posts)
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/catalog", nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	out := assertJSONKeys(t, rec.Body.Bytes(), "version", "products")
	if out["version"] != float64(7) {
		t.Fatalf("version=%v, want 7", out["version"])
	}
	products, ok := out["products"].([]interface{})
	if !ok || len(products) != 3 {
		t.Fatalf("products has wrong shape: %#v", out["products"])
	}
	first := assertJSONKeys(t, mustMarshal(t, products[0]),
		"sku", "name", "sale_price", "pinned_price", "effective_price", "priced",
		"vat_rate", "stock", "active", "category", "brand")
	if first["sku"] != "A-1" || first["name"] != "Alpha 🧊" || first["stock"] != float64(2) || first["active"] != true {
		t.Fatalf("unexpected product: %#v", first)
	}
	if first["pinned_price"] != "" || first["effective_price"] != "10.50" || first["priced"] != true {
		t.Fatalf("cleared pin must fall back to sale price: %#v", first)
	}
	if first["category"] != "Cat" || first["brand"] != nil {
		t.Fatalf("nullable fields wrong: %#v", first)
	}
	second := products[1].(map[string]interface{})
	if second["pinned_price"] != nil || second["effective_price"] != "12" || second["priced"] != true {
		t.Fatalf("unpinned priced product must be visible: %#v", second)
	}
	third := products[2].(map[string]interface{})
	if third["pinned_price"] != "149.00" || third["effective_price"] != "149.00" || third["priced"] != true {
		t.Fatalf("pinned product must use the pin: %#v", third)
	}

	for _, version := range []string{"7", "9"} {
		before := atomic.LoadInt64(&posts)
		req := httptest.NewRequest(http.MethodGet, "/api/catalog?version="+version, nil)
		rec := httptest.NewRecorder()
		Handler(rec, req)
		if rec.Code != http.StatusNotModified {
			t.Fatalf("version=%s got %d, want 304: %s", version, rec.Code, rec.Body.String())
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("304 must have empty body: %s", rec.Body.String())
		}
		if got := atomic.LoadInt64(&posts); got != before+1 {
			t.Fatalf("version=%s posts=%d, want one version lookup", version, got)
		}
	}
}

func TestCatalogIncludeUnpricedBypassesFilterAnd304(t *testing.T) {
	var posts int64
	srv := mockCatalogRead(t, 7, &posts)
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/catalog?version=7&include_unpriced=1", nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("include_unpriced with current version got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := atomic.LoadInt64(&posts); got != 2 {
		t.Fatalf("posts=%d, want 2 (version lookup + rows, no 304)", got)
	}
	out := assertJSONKeys(t, rec.Body.Bytes(), "version", "products")
	products, ok := out["products"].([]interface{})
	if !ok || len(products) != 5 {
		t.Fatalf("include_unpriced products=%d, want 5: %s", len(products), rec.Body.String())
	}
	bySKU := make(map[string]map[string]interface{}, len(products))
	for _, p := range products {
		product := p.(map[string]interface{})
		bySKU[lib.ToString(product["sku"])] = product
	}
	if unpriced := bySKU["C-3"]; unpriced["priced"] != false || unpriced["effective_price"] != "0" {
		t.Fatalf("sale_price=0 row must be exposed as unpriced: %#v", unpriced)
	}
	if zeroPin := bySKU["E-5"]; zeroPin["pinned_price"] != "0" || zeroPin["priced"] != false {
		t.Fatalf("pin=0 row must be exposed as unpriced: %#v", zeroPin)
	}
}

func TestCatalogGetRejectsMalformedVersion(t *testing.T) {
	var posts int64
	srv := mockCatalogRead(t, 7, &posts)
	defer srv.Close()

	for _, raw := range []string{"abc", "NaN", "1e2", "-3", " 7", "Infinity"} {
		before := atomic.LoadInt64(&posts)
		req := httptest.NewRequest(http.MethodGet, "/api/catalog?version="+url.QueryEscape(raw), nil)
		rec := httptest.NewRecorder()
		Handler(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("version=%q got %d, want 400: %s", raw, rec.Code, rec.Body.String())
		}
		if got := atomic.LoadInt64(&posts); got != before {
			t.Fatalf("version=%q reached Turso", raw)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/catalog?version=0", nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("version=0 got %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
