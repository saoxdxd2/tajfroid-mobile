package handler

import (
	"encoding/json"
	"net/http"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db := lib.NewTurso()
	res, err := db.Execute("SELECT version FROM catalog_meta WHERE id = 1")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	version := int64(0)
	if len(res.Rows) > 0 {
		version = lib.ToInt64(res.Rows[0][0])
	}
	reqVer := int64(0)
	if v := r.URL.Query().Get("version"); v != "" {
		reqVer = lib.ToInt64(v)
	}
	if reqVer >= version && version > 0 {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	rows, err := db.Execute("SELECT sku, name, sale_price, vat_rate, stock, active, category, brand FROM catalog")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	products := []map[string]interface{}{}
	for _, row := range rows.Rows {
		products = append(products, map[string]interface{}{
			"sku":        lib.ToString(row[0]),
			"name":       lib.ToString(row[1]),
			"sale_price": lib.ToString(row[2]),
			"vat_rate":   lib.ToString(row[3]),
			"stock":      lib.ToInt(row[4]),
			"active":     lib.ToInt(row[5]) == 1,
			"category":   lib.NullIface(row[6]),
			"brand":      lib.NullIface(row[7]),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"version": version, "products": products})
}
