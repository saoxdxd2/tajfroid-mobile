package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"tajfroid-mobile/lib"
)

const (
	productMaxSKURunes   = 128
	productMaxNameRunes  = 512
	productMaxDescRunes  = 2000
	productMaxTermRunes  = 128
	productMaxMoneyRunes = 64
	productMaxStock      = int64(1000000000)
)

func Handler(w http.ResponseWriter, r *http.Request) {
	db := lib.NewTurso()
	switch r.Method {
	case http.MethodGet:
		res, err := db.Execute("SELECT sku, name, description, purchase_price, sale_price, pinned_price, vat_rate, stock, active, category, brand FROM catalog ORDER BY name ASC")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		meta, err := db.Execute("SELECT version FROM catalog_meta WHERE id=1")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ver := int64(0)
		if len(meta.Rows) > 0 {
			ver = lib.ToInt64(meta.Rows[0][0])
		}
		// Sales surfaces use the default filtered list; pricing and inventory
		// surfaces opt in to unpriced rows with include_unpriced=1.
		includeUnpriced := r.URL.Query().Get("include_unpriced") == "1"
		items := []map[string]interface{}{}
		for _, row := range res.Rows {
			effectivePrice, priced := lib.CatalogEffectivePrice(row[5], row[4])
			if !priced && !includeUnpriced {
				continue
			}
			items = append(items, map[string]interface{}{
				"sku":            row[0],
				"name":           row[1],
				"description":    lib.NullStr(row[2]),
				"purchasePrice":  row[3],
				"salePrice":      row[4],
				"pinnedPrice":    lib.NullIface(row[5]),
				"effectivePrice": effectivePrice,
				"priced":         priced,
				"vatRate":        row[6],
				"stock":          lib.ToInt(row[7]),
				"active":         lib.ToInt(row[8]) == 1,
				"category":       lib.NullStr(row[9]),
				"brand":          lib.NullStr(row[10]),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"products": items, "catalogVersion": ver})

	case http.MethodPost:
		var body struct {
			SKU           string      `json:"sku"`
			Name          string      `json:"name"`
			Description   string      `json:"description"`
			PurchasePrice interface{} `json:"purchasePrice"`
			SalePrice     interface{} `json:"salePrice"`
			VatRate       interface{} `json:"vatRate"`
			Stock         interface{} `json:"stock"`
			Active        interface{} `json:"active"`
			Category      string      `json:"category"`
			Brand         string      `json:"brand"`
		}
		dec := json.NewDecoder(r.Body)
		dec.UseNumber()
		if err := dec.Decode(&body); err != nil {
			http.Error(w, "invalid product", http.StatusBadRequest)
			return
		}
		sku, err := cleanProductSKU(body.SKU)
		if err != nil {
			http.Error(w, "sku "+err.Error(), http.StatusBadRequest)
			return
		}
		if sku == "" {
			http.Error(w, "sku and name required", http.StatusBadRequest)
			return
		}
		name, err := cleanProductText(body.Name, productMaxNameRunes)
		if err != nil {
			http.Error(w, "name "+err.Error(), http.StatusBadRequest)
			return
		}
		if name == "" {
			http.Error(w, "sku and name required", http.StatusBadRequest)
			return
		}
		description, err := cleanProductText(body.Description, productMaxDescRunes)
		if err != nil {
			http.Error(w, "description "+err.Error(), http.StatusBadRequest)
			return
		}
		category, err := cleanProductText(body.Category, productMaxTermRunes)
		if err != nil {
			http.Error(w, "category "+err.Error(), http.StatusBadRequest)
			return
		}
		brand, err := cleanProductText(body.Brand, productMaxTermRunes)
		if err != nil {
			http.Error(w, "brand "+err.Error(), http.StatusBadRequest)
			return
		}
		purchasePrice, err := parseProductDecimal(body.PurchasePrice, "0")
		if err != nil {
			http.Error(w, "purchasePrice "+err.Error(), http.StatusBadRequest)
			return
		}
		salePrice, err := parseProductDecimal(body.SalePrice, "0")
		if err != nil {
			http.Error(w, "salePrice "+err.Error(), http.StatusBadRequest)
			return
		}
		vatRate, err := parseProductDecimal(body.VatRate, "20")
		if err != nil {
			http.Error(w, "vatRate "+err.Error(), http.StatusBadRequest)
			return
		}
		stock, err := parseProductStock(body.Stock)
		if err != nil {
			http.Error(w, "stock "+err.Error(), http.StatusBadRequest)
			return
		}
		active, err := parseProductActive(body.Active)
		if err != nil {
			http.Error(w, "active "+err.Error(), http.StatusBadRequest)
			return
		}

		// Idempotent replay: an already-created SKU returns 200 without a write.
		ex, err := db.Execute("SELECT sku, name FROM catalog WHERE sku=?", lib.Text(sku))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(ex.Rows) > 0 {
			lib.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"sku": ex.Rows[0][0], "name": ex.Rows[0][1], "idempotent": true,
			})
			return
		}

		now := time.Now().Unix()
		_, err = db.ExecuteBatch(r.Context(),
			lib.TursoStmt{
				SQL: `INSERT INTO catalog (sku, name, description, purchase_price, sale_price, vat_rate, stock, active, category, brand) VALUES (?,?,?,?,?,?,?,?,?,?)`,
				Args: []lib.TursoArg{
					lib.Text(sku), lib.Text(name), nullableText(description),
					lib.Text(purchasePrice), lib.Text(salePrice), lib.Text(vatRate),
					lib.Int(stock), lib.Int(active),
					nullableText(category), nullableText(brand),
				},
			},
			lib.TursoStmt{
				SQL:  lib.CatalogMetaBumpSQL,
				Args: []lib.TursoArg{lib.Int(now), lib.Int(now)},
			},
		)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				lib.WriteJSON(w, http.StatusConflict, map[string]interface{}{
					"error": err.Error(), "sku": sku, "idempotent": true,
				})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"sku": sku, "name": name})

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

// cleanProductText removes control characters (including NUL) before a value
// reaches SQL or JSON. Printable unicode is kept; overlength values are a
// client error rather than silently truncated.
func cleanProductText(s string, maxRunes int) (string, error) {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxRunes {
		return "", fmt.Errorf("exceeds %d characters", maxRunes)
	}
	return s, nil
}

func cleanProductSKU(s string) (string, error) {
	if strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return "", errors.New("contains control characters")
	}
	return cleanProductText(s, productMaxSKURunes)
}

func canonicalDecimalText(s string) string {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, ",") {
		return s
	}
	if strings.Contains(s, ".") {
		return strings.ReplaceAll(s, ",", "")
	}
	parts := strings.Split(s, ",")
	last := parts[len(parts)-1]
	if len(last) > 0 && len(last) <= 2 {
		return strings.Join(parts[:len(parts)-1], "") + "." + last
	}
	return strings.ReplaceAll(s, ",", "")
}

func parseProductDecimal(v interface{}, fallback string) (string, error) {
	s := canonicalDecimalText(lib.ToString(v))
	if s == "" || s == "<nil>" {
		return fallback, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return "", errors.New("must be a finite number")
	}
	if f < 0 {
		return "", errors.New("must not be negative")
	}
	if f == 0 {
		return "0", nil
	}
	// Preserve meaningful decimal precision after comma canonicalization
	// (e.g. "10,50" -> "10.50"), but normalize whole numbers and exponent
	// notation to their plain decimal form ("20.0" -> "20", "1e3" -> "1000").
	out := s
	if f == math.Trunc(f) || strings.ContainsAny(s, "eE") {
		out = strconv.FormatFloat(f, 'f', -1, 64)
	}
	if len(out) > productMaxMoneyRunes {
		return "", errors.New("number is too large")
	}
	return out, nil
}

func parseProductStock(v interface{}) (int64, error) {
	if v == nil {
		return 0, nil
	}
	s := canonicalDecimalText(lib.ToString(v))
	if s == "" || s == "<nil>" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, errors.New("must be a finite integer")
	}
	if f < 0 {
		return 0, errors.New("must not be negative")
	}
	if math.Trunc(f) != f {
		return 0, errors.New("must be an integer")
	}
	if f > float64(productMaxStock) {
		return 0, errors.New("stock is too large")
	}
	return int64(f), nil
}

func parseProductActive(v interface{}) (int64, error) {
	if v == nil {
		return 0, nil
	}
	if b, ok := v.(bool); ok {
		if b {
			return 1, nil
		}
		return 0, nil
	}
	switch strings.ToLower(strings.TrimSpace(lib.ToString(v))) {
	case "", "0", "false", "no", "n", "off":
		return 0, nil
	case "1", "true", "yes", "y", "on":
		return 1, nil
	default:
		return 0, errors.New("must be a boolean or 0/1")
	}
}
