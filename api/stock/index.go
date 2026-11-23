package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	db := lib.NewTurso()
	switch r.Method {
	case http.MethodGet:
		res, err := db.Execute("SELECT sku, name, stock, active FROM catalog ORDER BY name ASC")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := []map[string]interface{}{}
		for _, row := range res.Rows {
			items = append(items, map[string]interface{}{
				"sku": row[0], "name": row[1], "stock": lib.ToInt(row[2]), "active": lib.ToInt(row[3]) == 1,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"stock": items})

	case http.MethodPost:
		var body struct {
			SKU    string `json:"sku"`
			Change int    `json:"change"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.SKU == "" {
			http.Error(w, "sku required", http.StatusBadRequest)
			return
		}
		// Get current stock
		res, err := db.Execute("SELECT stock FROM catalog WHERE sku = ?", lib.Text(body.SKU))
		if err != nil || len(res.Rows) == 0 {
			http.Error(w, "product not found", http.StatusNotFound)
			return
		}
		current := lib.ToInt(res.Rows[0][0])
		newStock := current + body.Change
		if newStock < 0 {
			http.Error(w, "insufficient stock", http.StatusBadRequest)
			return
		}
		now := time.Now().Unix()
		db.Execute("UPDATE catalog SET stock=?, updated_at=? WHERE sku=?", lib.Int(int64(newStock)), lib.Int(now), lib.Text(body.SKU))
		db.Execute("INSERT INTO catalog_meta (id, version, updated_at) VALUES (1, 1, ?) ON CONFLICT(id) DO UPDATE SET version = version + 1, updated_at = excluded.updated_at", lib.Int(now))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"sku": body.SKU, "oldStock": current, "newStock": newStock})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}