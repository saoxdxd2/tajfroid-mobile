package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Version  int                      `json:"version"`
		Products []map[string]interface{} `json:"products"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	db := lib.NewTurso()
	db.Execute("DELETE FROM catalog")
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
		db.Execute("INSERT INTO catalog (sku, name, sale_price, vat_rate, stock, active, category, brand) VALUES (?,?,?,?,?,?,?,?)",
			lib.Text(sku), lib.Text(name), lib.Text(price), lib.Text(vat), lib.Int(int64(stock)), lib.Int(int64(active)), cat, brand)
	}
	now := time.Now().Unix()
	db.Execute("INSERT INTO catalog_meta (id, version, updated_at) VALUES (1,?,?) ON CONFLICT(id) DO UPDATE SET version=excluded.version, updated_at=excluded.updated_at",
		lib.Int(int64(body.Version)), lib.Int(now))
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