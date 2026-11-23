package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"tajfroid-relay/lib"
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
		version = toInt64(res.Rows[0][0])
	}
	reqVer := int64(0)
	if v := r.URL.Query().Get("version"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			reqVer = n
		}
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
			"sku":        toString(row[0]),
			"name":       toString(row[1]),
			"sale_price": toString(row[2]),
			"vat_rate":   toString(row[3]),
			"stock":      toInt(row[4]),
			"active":     toInt(row[5]) == 1,
			"category":   nullableString(row[6]),
			"brand":      nullableString(row[7]),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"version": version, "products": products})
}

func toInt64(v interface{}) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	}
	return 0
}

func toInt(v interface{}) int {
	return int(toInt64(v))
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func nullableString(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	return fmt.Sprintf("%v", v)
}