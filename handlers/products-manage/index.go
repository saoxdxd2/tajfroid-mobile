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
	manageMaxSKURunes   = 128
	manageMaxNameRunes  = 512
	manageMaxDescRunes  = 2000
	manageMaxTermRunes  = 128
	manageMaxMoneyRunes = 64
	manageMaxStock      = int64(1000000000)
)

func Handler(w http.ResponseWriter, r *http.Request) {
	db := lib.NewTurso()
	sku, err := cleanManagedSKU(r.URL.Query().Get("sku"))
	if err != nil {
		http.Error(w, "sku "+err.Error(), http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		res, err := db.Execute("SELECT sku, name, description, purchase_price, sale_price, vat_rate, stock, active, category, brand FROM catalog WHERE sku = ?", lib.Text(sku))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(res.Rows) == 0 {
			http.Error(w, "product not found", http.StatusNotFound)
			return
		}
		ver, err := currentCatalogVersion(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		row := res.Rows[0]
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"sku": row[0], "name": row[1], "description": lib.NullStr(row[2]),
			"purchasePrice": row[3], "salePrice": row[4], "vatRate": row[5],
			"stock": lib.ToInt(row[6]), "active": lib.ToInt(row[7]) == 1,
			"category": lib.NullStr(row[8]), "brand": lib.NullStr(row[9]),
			"catalogVersion": ver,
		})

	case http.MethodPut:
		var body managedProductBody
		dec := json.NewDecoder(r.Body)
		dec.UseNumber()
		if err := dec.Decode(&body); err != nil {
			http.Error(w, "invalid product", http.StatusBadRequest)
			return
		}
		// Keep the existing 409 contract: a stale write loses before any
		// field-level validation or database mutation happens.
		if body.BaseCatalogVersion > 0 {
			cur, err := currentCatalogVersion(db)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if cur != body.BaseCatalogVersion {
				writeCatalogConflict(w, cur)
				return
			}
		}

		updates, err := managedUpdates(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		now := time.Now().Unix()
		stmts := make([]lib.TursoStmt, 0, len(updates)+1)
		for _, u := range updates {
			stmts = append(stmts, lib.TursoStmt{SQL: u.sql, Args: []lib.TursoArg{u.value, lib.Text(sku)}})
		}
		// catalog_meta.version is the sync cursor the phone agent polls; the
		// catalog table itself has no updated_at column.
		stmts = append(stmts, lib.TursoStmt{
			SQL:  lib.CatalogMetaBumpSQL,
			Args: []lib.TursoArg{lib.Int(now), lib.Int(now)},
		})
		if _, err := db.ExecuteBatch(r.Context(), stmts...); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	case http.MethodDelete:
		if raw := r.URL.Query().Get("baseCatalogVersion"); raw != "" {
			base, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || base < 0 {
				http.Error(w, "invalid baseCatalogVersion", http.StatusBadRequest)
				return
			}
			if base > 0 {
				cur, err := currentCatalogVersion(db)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				if cur != base {
					writeCatalogConflict(w, cur)
					return
				}
			}
		}
		// Archive (soft delete) — set active=0
		now := time.Now().Unix()
		stmts := []lib.TursoStmt{
			{SQL: "UPDATE catalog SET active=0 WHERE sku=?", Args: []lib.TursoArg{lib.Text(sku)}},
			{
				SQL:  lib.CatalogMetaBumpSQL,
				Args: []lib.TursoArg{lib.Int(now), lib.Int(now)},
			},
		}
		if _, err := db.ExecuteBatch(r.Context(), stmts...); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeCatalogConflict(w http.ResponseWriter, current int64) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	json.NewEncoder(w).Encode(map[string]interface{}{"error": "conflict: catalog changed on another device", "currentCatalogVersion": current})
}

type managedProductBody struct {
	Name               string      `json:"name"`
	Description        string      `json:"description"`
	PurchasePrice      interface{} `json:"purchasePrice"`
	SalePrice          interface{} `json:"salePrice"`
	VatRate            interface{} `json:"vatRate"`
	Stock              interface{} `json:"stock"`
	Active             interface{} `json:"active"`
	Category           string      `json:"category"`
	Brand              string      `json:"brand"`
	BaseCatalogVersion int64       `json:"baseCatalogVersion"`
}

type managedUpdate struct {
	sql   string
	value lib.TursoArg
}

// managedUpdates validates every field before any statement is sent, so a
// malformed number or negative stock never leaves a half-applied batch.
func managedUpdates(body managedProductBody) ([]managedUpdate, error) {
	updates := make([]managedUpdate, 0, 9)
	addText := func(field, sqlText, value string, maxRunes int) error {
		clean, err := cleanManagedText(value, maxRunes)
		if err != nil {
			return fmt.Errorf("%s %w", field, err)
		}
		if clean == "" {
			return nil
		}
		updates = append(updates, managedUpdate{sql: sqlText, value: lib.Text(clean)})
		return nil
	}
	if err := addText("name", "UPDATE catalog SET name=? WHERE sku=?", body.Name, manageMaxNameRunes); err != nil {
		return nil, err
	}
	if err := addText("description", "UPDATE catalog SET description=? WHERE sku=?", body.Description, manageMaxDescRunes); err != nil {
		return nil, err
	}
	for _, f := range []struct {
		field string
		sql   string
		value interface{}
	}{
		{"purchasePrice", "UPDATE catalog SET purchase_price=? WHERE sku=?", body.PurchasePrice},
		{"salePrice", "UPDATE catalog SET sale_price=? WHERE sku=?", body.SalePrice},
		{"vatRate", "UPDATE catalog SET vat_rate=? WHERE sku=?", body.VatRate},
	} {
		if f.value == nil {
			continue
		}
		s, err := parseManagedDecimal(f.value)
		if err != nil {
			return nil, fmt.Errorf("%s %w", f.field, err)
		}
		if s == "" {
			continue
		}
		updates = append(updates, managedUpdate{sql: f.sql, value: lib.Text(s)})
	}
	if body.Stock != nil {
		stock, err := parseManagedStock(body.Stock)
		if err != nil {
			return nil, fmt.Errorf("stock %w", err)
		}
		updates = append(updates, managedUpdate{sql: "UPDATE catalog SET stock=? WHERE sku=?", value: lib.Int(stock)})
	}
	if body.Active != nil {
		active, err := parseManagedActive(body.Active)
		if err != nil {
			return nil, fmt.Errorf("active %w", err)
		}
		updates = append(updates, managedUpdate{sql: "UPDATE catalog SET active=? WHERE sku=?", value: lib.Int(active)})
	}
	if err := addText("category", "UPDATE catalog SET category=? WHERE sku=?", body.Category, manageMaxTermRunes); err != nil {
		return nil, err
	}
	if err := addText("brand", "UPDATE catalog SET brand=? WHERE sku=?", body.Brand, manageMaxTermRunes); err != nil {
		return nil, err
	}
	return updates, nil
}

func cleanManagedSKU(s string) (string, error) {
	if strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return "", errors.New("contains control characters")
	}
	clean, err := cleanManagedText(s, manageMaxSKURunes)
	if err != nil {
		return "", err
	}
	if clean == "" {
		return "", errors.New("required")
	}
	return clean, nil
}

// cleanManagedText drops control characters while preserving printable
// unicode. Values that exceed the limit are rejected, not truncated.
func cleanManagedText(s string, maxRunes int) (string, error) {
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

func parseManagedDecimal(v interface{}) (string, error) {
	s := canonicalDecimalText(lib.ToString(v))
	if s == "" || s == "<nil>" {
		return "", nil
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
	if len(out) > manageMaxMoneyRunes {
		return "", errors.New("number is too large")
	}
	return out, nil
}

func parseManagedStock(v interface{}) (int64, error) {
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
	if f > float64(manageMaxStock) {
		return 0, errors.New("stock is too large")
	}
	return int64(f), nil
}

func parseManagedActive(v interface{}) (int64, error) {
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

// catalogVersion is kept as a small compatibility wrapper; handlers use the
// error-aware variant so a failed version read is never reported as a conflict.
func catalogVersion(db *lib.TursoClient) int64 {
	v, _ := currentCatalogVersion(db)
	return v
}

func currentCatalogVersion(db *lib.TursoClient) (int64, error) {
	res, err := db.Execute("SELECT version FROM catalog_meta WHERE id=1")
	if err != nil {
		return 0, err
	}
	if len(res.Rows) == 0 {
		return 0, nil
	}
	return lib.ToInt64(res.Rows[0][0]), nil
}
