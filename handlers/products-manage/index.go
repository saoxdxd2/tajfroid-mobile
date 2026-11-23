package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	db := lib.NewTurso()
	sku := r.URL.Query().Get("sku")
	if sku == "" {
		http.Error(w, "sku required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		res, err := db.Execute("SELECT sku, name, description, purchase_price, sale_price, vat_rate, stock, active, category, brand FROM catalog WHERE sku = ?", lib.Text(sku))
		if err != nil || len(res.Rows) == 0 {
			http.Error(w, "product not found", http.StatusNotFound)
			return
		}
		row := res.Rows[0]
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"sku": row[0], "name": row[1], "description": nullableStr(row[2]),
			"purchasePrice": row[3], "salePrice": row[4], "vatRate": row[5],
			"stock": lib.ToInt(row[6]), "active": lib.ToInt(row[7]) == 1,
			"category": nullableStr(row[8]), "brand": nullableStr(row[9]),
		})

	case http.MethodPut:
		var body struct {
			Name          string `json:"name"`
			Description   string `json:"description"`
			PurchasePrice string `json:"purchasePrice"`
			SalePrice     string `json:"salePrice"`
			VatRate       string `json:"vatRate"`
			Stock         *int   `json:"stock"`
			Active        *bool  `json:"active"`
			Category      string `json:"category"`
			Brand         string `json:"brand"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		now := time.Now().Unix()
		if body.Name != "" {
			db.Execute("UPDATE catalog SET name=? WHERE sku=?", lib.Text(body.Name), lib.Text(sku))
		}
		if body.Description != "" {
			db.Execute("UPDATE catalog SET description=? WHERE sku=?", lib.Text(body.Description), lib.Text(sku))
		}
		if body.PurchasePrice != "" {
			db.Execute("UPDATE catalog SET purchase_price=? WHERE sku=?", lib.Text(body.PurchasePrice), lib.Text(sku))
		}
		if body.SalePrice != "" {
			db.Execute("UPDATE catalog SET sale_price=? WHERE sku=?", lib.Text(body.SalePrice), lib.Text(sku))
		}
		if body.VatRate != "" {
			db.Execute("UPDATE catalog SET vat_rate=? WHERE sku=?", lib.Text(body.VatRate), lib.Text(sku))
		}
		if body.Stock != nil {
			db.Execute("UPDATE catalog SET stock=? WHERE sku=?", lib.Int(int64(*body.Stock)), lib.Text(sku))
		}
		if body.Active != nil {
			a := 0
			if *body.Active {
				a = 1
			}
			db.Execute("UPDATE catalog SET active=? WHERE sku=?", lib.Int(int64(a)), lib.Text(sku))
		}
		if body.Category != "" {
			db.Execute("UPDATE catalog SET category=? WHERE sku=?", lib.Text(body.Category), lib.Text(sku))
		}
		if body.Brand != "" {
			db.Execute("UPDATE catalog SET brand=? WHERE sku=?", lib.Text(body.Brand), lib.Text(sku))
		}
		// catalog_meta.version is the sync cursor the phone agent polls; the
		// catalog table itself has no updated_at column.
		db.Execute("INSERT INTO catalog_meta (id, version, updated_at) VALUES (1, 1, ?) ON CONFLICT(id) DO UPDATE SET version = version + 1, updated_at = excluded.updated_at", lib.Int(now))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	case http.MethodDelete:
		// Archive (soft delete) — set active=0
		now := time.Now().Unix()
		db.Execute("UPDATE catalog SET active=0 WHERE sku=?", lib.Text(sku))
		db.Execute("INSERT INTO catalog_meta (id, version, updated_at) VALUES (1, 1, ?) ON CONFLICT(id) DO UPDATE SET version = version + 1, updated_at = excluded.updated_at", lib.Int(now))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func nullableStr(v interface{}) string {
	if v == nil {
		return ""
	}
	return lib.ToString(v)
}