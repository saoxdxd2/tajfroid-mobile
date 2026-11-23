package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	db := lib.NewTurso()
	switch r.Method {
	case http.MethodGet:
		res, err := db.Execute("SELECT sku, name, description, purchase_price, sale_price, vat_rate, stock, active, category, brand FROM catalog ORDER BY name ASC")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ver := int64(0)
		if m, err := db.Execute("SELECT version FROM catalog_meta WHERE id=1"); err == nil && len(m.Rows) > 0 {
			ver = lib.ToInt64(m.Rows[0][0])
		}
		items := []map[string]interface{}{}
		for _, row := range res.Rows {
			items = append(items, map[string]interface{}{
				"sku":           row[0],
				"name":          row[1],
				"description":   lib.NullStr(row[2]),
				"purchasePrice": row[3],
				"salePrice":     row[4],
				"vatRate":       row[5],
				"stock":         lib.ToInt(row[6]),
				"active":        lib.ToInt(row[7]) == 1,
				"category":      lib.NullStr(row[8]),
				"brand":         lib.NullStr(row[9]),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"products": items, "catalogVersion": ver})

	case http.MethodPost:
		var body struct {
			SKU           string  `json:"sku"`
			Name          string  `json:"name"`
			Description   string  `json:"description"`
			PurchasePrice string  `json:"purchasePrice"`
			SalePrice     string  `json:"salePrice"`
			VatRate       string  `json:"vatRate"`
			Stock         int     `json:"stock"`
			Active        bool    `json:"active"`
			Category      string  `json:"category"`
			Brand         string  `json:"brand"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.SKU == "" || body.Name == "" {
			http.Error(w, "sku and name required", http.StatusBadRequest)
			return
		}
		if ex, err := db.Execute("SELECT sku, name FROM catalog WHERE sku=?", lib.Text(body.SKU)); err == nil && len(ex.Rows) > 0 {
			lib.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"sku": ex.Rows[0][0], "name": ex.Rows[0][1], "idempotent": true,
			})
			return
		}
		if body.SalePrice == "" {
			body.SalePrice = "0"
		}
		if body.VatRate == "" {
			body.VatRate = "20"
		}
		if body.PurchasePrice == "" {
			body.PurchasePrice = "0"
		}
		active := 0
		if body.Active {
			active = 1
		}
		now := time.Now().Unix()
		_, err := db.Execute(`INSERT INTO catalog (sku, name, description, purchase_price, sale_price, vat_rate, stock, active, category, brand) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			lib.Text(body.SKU), lib.Text(body.Name), nullableText(body.Description),
			lib.Text(body.PurchasePrice), lib.Text(body.SalePrice), lib.Text(body.VatRate),
			lib.Int(int64(body.Stock)), lib.Int(int64(active)),
			nullableText(body.Category), nullableText(body.Brand))
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				lib.WriteJSON(w, http.StatusConflict, map[string]interface{}{
					"error": err.Error(), "sku": body.SKU, "idempotent": true,
				})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Update catalog_meta version
		db.Execute("INSERT INTO catalog_meta (id, version, updated_at) VALUES (1, 1, ?) ON CONFLICT(id) DO UPDATE SET version = version + 1, updated_at = excluded.updated_at", lib.Int(now))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"sku": body.SKU, "name": body.Name})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}


func nullableText(s string) lib.TursoArg {
	if s == "" {
		return lib.Null()
	}
	return lib.Text(s)
}