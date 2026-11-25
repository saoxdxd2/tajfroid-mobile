package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	reqVer := int64(0)
	if raw := r.URL.Query().Get("version"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			http.Error(w, "invalid version", http.StatusBadRequest)
			return
		}
		reqVer = v
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
	// A mode switch (pricing view vs sales catalog) must never be answered
	// from a cached 304: the two modes have different bodies at the same
	// catalog version.
	includeUnpriced := r.URL.Query().Get("include_unpriced") == "1"
	if !includeUnpriced && reqVer >= version && version > 0 {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	rows, err := db.Execute("SELECT sku, name, sale_price, pinned_price, vat_rate, stock, active, category, brand, no_sku, display_sku, sku_warning, is_return, return_qty, sealed_qty, return_label, price_status FROM catalog")
	if err != nil {
		// Backward-compatible fallback if older DB has not run migration yet
		rows, err = db.Execute("SELECT sku, name, sale_price, pinned_price, vat_rate, stock, active, category, brand FROM catalog")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	products := []map[string]interface{}{}
	for _, row := range rows.Rows {
		effectivePrice, priced := lib.CatalogEffectivePrice(row[3], row[2])
		if !priced && !includeUnpriced {
			continue
		}
		sku := lib.ToString(row[0])
		noSKU := false
		displaySKU := sku
		var skuWarning interface{}
		isReturn := false
		returnQty := 0
		stock := lib.ToInt(row[5])
		sealedQty := stock
		var returnLabel interface{}
		priceStatus := "PRICED"
		if !priced {
			priceStatus = "NO_PRICE"
		}
		if len(row) >= 17 {
			noSKU = lib.ToInt(row[9]) == 1
			if ds := lib.NullStr(row[10]); ds != "" {
				displaySKU = ds
			} else if noSKU {
				displaySKU = "NO SKU"
			}
			skuWarning = lib.NullIface(row[11])
			isReturn = lib.ToInt(row[12]) == 1
			returnQty = lib.ToInt(row[13])
			sealedQty = lib.ToInt(row[14])
			returnLabel = lib.NullIface(row[15])
			if ps := lib.NullStr(row[16]); ps != "" {
				priceStatus = ps
			}
		}
		products = append(products, map[string]interface{}{
			"sku":             sku,
			"display_sku":     displaySKU,
			"no_sku":          noSKU,
			"sku_warning":     skuWarning,
			"name":            lib.ToString(row[1]),
			"sale_price":      lib.ToString(row[2]),
			"pinned_price":    lib.NullIface(row[3]),
			"effective_price": effectivePrice,
			"priced":          priced,
			"price_status":    priceStatus,
			"vat_rate":        lib.ToString(row[4]),
			"stock":           stock,
			"sealed_qty":      sealedQty,
			"is_return":       isReturn,
			"return_qty":      returnQty,
			"return_label":    returnLabel,
			"active":          lib.ToInt(row[6]) == 1,
			"category":        lib.NullIface(row[7]),
			"brand":           lib.NullIface(row[8]),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"version": version, "products": products})
}
