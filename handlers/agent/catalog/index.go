package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"tajfroid-mobile/lib"
)

// A full catalog rebuild used to issue one HTTPS request per product. The
// relay now sends all statements through one Turso pipeline request: network
// round-trips drop from O(N) to O(1) while statement count stays O(N).
const maxCatalogBody = 16 << 20

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Version  int                      `json:"version"`
		Products []map[string]interface{} `json:"products"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCatalogBody)).Decode(&body); err != nil {
		http.Error(w, "invalid catalog", http.StatusBadRequest)
		return
	}

	stmts := make([]lib.TursoStmt, 0, len(body.Products)+2)
	stmts = append(stmts, lib.TursoStmt{SQL: "DELETE FROM catalog"})
	for _, p := range body.Products {
		sku := lib.ToString(p["sku"])
		name := lib.ToString(p["name"])
		price := lib.ToString(p["sale_price"])
		vat := lib.ToString(p["vat_rate"])
		stock := lib.ToInt(p["stock"])
		active := 0
		if b, ok := p["active"].(bool); ok && b {
			active = 1
		}
		cat := nullableAny(p["category"])
		brand := nullableAny(p["brand"])
		stmts = append(stmts, lib.TursoStmt{
			SQL: "INSERT INTO catalog (sku, name, sale_price, vat_rate, stock, active, category, brand) VALUES (?,?,?,?,?,?,?,?)",
			Args: []lib.TursoArg{
				lib.Text(sku), lib.Text(name), lib.Text(price), lib.Text(vat),
				lib.Int(int64(stock)), lib.Int(int64(active)), cat, brand,
			},
		})
	}
	now := time.Now().Unix()
	stmts = append(stmts, lib.TursoStmt{
		SQL:  "INSERT INTO catalog_meta (id, version, updated_at) VALUES (1,?,?) ON CONFLICT(id) DO UPDATE SET version=excluded.version, updated_at=excluded.updated_at",
		Args: []lib.TursoArg{lib.Int(int64(body.Version)), lib.Int(now)},
	})

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	if _, err := lib.NewTurso().ExecuteBatch(ctx, stmts...); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "count": len(body.Products)})
}

func nullableAny(v interface{}) lib.TursoArg {
	if v == nil {
		return lib.Null()
	}
	s := lib.ToString(v)
	if s == "" || s == "<nil>" {
		return lib.Null()
	}
	return lib.Text(s)
}
