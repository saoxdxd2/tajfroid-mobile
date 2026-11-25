package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"tajfroid-mobile/lib"
)

// A full catalog rebuild used to issue one HTTPS request per product. The
// relay now sends all statements through one Turso pipeline request: network
// round-trips drop from O(N) to O(1) while statement count stays O(N).
const maxCatalogBody = 16 << 20

// ERP exports are messy but bounded. Reject values outside these limits with
// a 4xx instead of writing garbage (or an unbounded blob) into the catalog.
const (
	catalogMaxSKURunes   = 128
	catalogMaxNameRunes  = 512
	catalogMaxTermRunes  = 128
	catalogMaxMoneyRunes = 64
	catalogMaxStock      = int64(1000000000)
)

const catalogInsertSQL = "INSERT INTO catalog (sku, name, pinned_price, sale_price, vat_rate, stock, active, category, brand, no_sku, display_sku, sku_warning, is_return, return_qty, sealed_qty, return_label, price_status) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) " +
	"ON CONFLICT(sku) DO UPDATE SET name=excluded.name, " +
	// A legacy client that omits (or nulls) pinned_price must not wipe a pin
	// entered on the phone. An explicit '' clears it; a decimal sets it.
	"pinned_price = CASE WHEN excluded.pinned_price IS NULL THEN catalog.pinned_price ELSE excluded.pinned_price END, " +
	"sale_price=excluded.sale_price, vat_rate=excluded.vat_rate, " +
	"stock=excluded.stock, active=excluded.active, category=excluded.category, brand=excluded.brand, " +
	"no_sku=excluded.no_sku, display_sku=excluded.display_sku, sku_warning=excluded.sku_warning, " +
	"is_return=excluded.is_return, return_qty=excluded.return_qty, sealed_qty=excluded.sealed_qty, " +
	"return_label=excluded.return_label, price_status=excluded.price_status"

type catalogRow struct {
	sku         string
	name        string
	pinned      lib.TursoArg
	salePrice   string
	vatRate     string
	stock       int64
	active      int64
	category    lib.TursoArg
	brand       lib.TursoArg
	noSKU       int64
	displaySKU  lib.TursoArg
	skuWarning  lib.TursoArg
	isReturn    int64
	returnQty   int64
	sealedQty   int64
	returnLabel lib.TursoArg
	priceStatus string
}

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Version      int64                    `json:"version"`
		BaseRevision int64                    `json:"baseRevision"`
		Products     []map[string]interface{} `json:"products"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCatalogBody))
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "invalid catalog", http.StatusBadRequest)
		return
	}
	if body.Version < 0 {
		http.Error(w, "invalid catalog version", http.StatusBadRequest)
		return
	}
	rows, err := prepareCatalogProducts(body.Products)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	// A full rebuild is a read-modify-write. The caller sends the revision it
	// last saw, so an offline desktop cannot overwrite a newer catalog and
	// leave the phone on a stale 304 response.
	db := lib.NewTurso()
	meta, err := db.ExecuteContext(ctx, "SELECT version, updated_at FROM catalog_meta WHERE id = 1")
	if err != nil {
		http.Error(w, "catalog state unavailable", http.StatusInternalServerError)
		return
	}
	var currentVersion, currentRevision int64
	if len(meta.Rows) > 0 {
		currentVersion = lib.ToInt64(meta.Rows[0][0])
		currentRevision = lib.ToInt64(meta.Rows[0][1])
	}
	if body.BaseRevision > 0 && body.BaseRevision != currentRevision {
		lib.WriteJSON(w, http.StatusConflict, map[string]interface{}{
			"error":           "conflict: catalog changed on another device",
			"currentVersion":  currentVersion,
			"currentRevision": currentRevision,
		})
		return
	}

	// The shared bump SQL assigns a fresh, monotonic sync cursor. The content
	// hash in body.Version is validated for compatibility but deliberately not
	// used as the phone cursor: hashes do not sort chronologically.
	newRevision := currentRevision + 1
	if now := time.Now().Unix(); now > newRevision {
		newRevision = now + 1
	}
	newVersion := currentVersion + 1
	if newRevision > newVersion {
		newVersion = newRevision
	}

	// A rebuild is not allowed to wipe pins entered on the phone while a
	// desktop was offline. Read the current SKUs only after the baseRevision
	// check, then delete just the rows missing from this snapshot.
	existing, err := db.Execute("SELECT sku FROM catalog")
	if err != nil {
		http.Error(w, "catalog state unavailable", http.StatusInternalServerError)
		return
	}
	incoming := make(map[string]struct{}, len(rows))
	for _, p := range rows {
		incoming[p.sku] = struct{}{}
	}
	stale := make([]string, 0)
	seenStale := make(map[string]struct{})
	for _, row := range existing.Rows {
		sku := lib.ToString(row[0])
		if sku == "" {
			continue
		}
		if _, ok := incoming[sku]; ok {
			continue
		}
		if _, ok := seenStale[sku]; ok {
			continue
		}
		seenStale[sku] = struct{}{}
		stale = append(stale, sku)
	}
	sort.Strings(stale)

	stmts := make([]lib.TursoStmt, 0, len(stale)+len(rows)+1)
	for _, sku := range stale {
		stmts = append(stmts, lib.TursoStmt{
			SQL:  "DELETE FROM catalog WHERE sku=?",
			Args: []lib.TursoArg{lib.Text(sku)},
		})
	}
	for _, p := range rows {
		stmts = append(stmts, lib.TursoStmt{
			SQL: catalogInsertSQL,
			Args: []lib.TursoArg{
				lib.Text(p.sku), lib.Text(p.name), p.pinned, lib.Text(p.salePrice), lib.Text(p.vatRate),
				lib.Int(p.stock), lib.Int(p.active), p.category, p.brand,
				lib.Int(p.noSKU), p.displaySKU, p.skuWarning, lib.Int(p.isReturn), lib.Int(p.returnQty),
				lib.Int(p.sealedQty), p.returnLabel, lib.Text(p.priceStatus),
			},
		})
	}
	stmts = append(stmts, lib.TursoStmt{
		SQL:  lib.CatalogMetaBumpSQL,
		Args: []lib.TursoArg{lib.Int(newRevision), lib.Int(newRevision)},
	})

	if _, err := db.ExecuteBatch(ctx, stmts...); err != nil {
		// Surface the real pipeline error; never report a partial success.
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok": true, "count": len(rows), "version": newVersion, "revision": newRevision,
	})
}

// prepareCatalogProducts validates and normalizes the whole payload before a
// single statement is sent. Duplicate SKUs are resolved deterministically:
// the last occurrence wins, while the row keeps the position of the first
// occurrence so the generated batch does not depend on map iteration order.
func prepareCatalogProducts(raw []map[string]interface{}) ([]catalogRow, error) {
	rows := make([]catalogRow, 0, len(raw))
	indexBySKU := make(map[string]int, len(raw))
	for i, p := range raw {
		if p == nil {
			return nil, fmt.Errorf("product %d: empty object", i)
		}
		rawSKU := lib.ToString(p["sku"])
		if strings.IndexFunc(rawSKU, unicode.IsControl) >= 0 {
			return nil, fmt.Errorf("product %d sku: contains control characters", i)
		}
		sku, err := cleanCatalogText(rawSKU, catalogMaxSKURunes)
		if err != nil {
			return nil, fmt.Errorf("product %d sku: %w", i, err)
		}
		if sku == "" {
			return nil, fmt.Errorf("product %d: sku required", i)
		}
		name, err := cleanCatalogText(lib.ToString(p["name"]), catalogMaxNameRunes)
		if err != nil {
			return nil, fmt.Errorf("product %d name: %w", i, err)
		}
		if name == "" {
			return nil, fmt.Errorf("product %d: name required", i)
		}
		pinned, err := parseCatalogPin(p, i)
		if err != nil {
			return nil, err
		}
		salePrice, err := parseCatalogDecimal(p["sale_price"], "0")
		if err != nil {
			return nil, fmt.Errorf("product %d sale_price: %w", i, err)
		}
		vatRate, err := parseCatalogDecimal(p["vat_rate"], "20")
		if err != nil {
			return nil, fmt.Errorf("product %d vat_rate: %w", i, err)
		}
		stock, err := parseCatalogStock(p["stock"])
		if err != nil {
			return nil, fmt.Errorf("product %d stock: %w", i, err)
		}
		active, err := parseCatalogActive(p["active"])
		if err != nil {
			return nil, fmt.Errorf("product %d active: %w", i, err)
		}
		category, err := catalogNullable(p["category"], "category", i)
		if err != nil {
			return nil, err
		}
		brand, err := catalogNullable(p["brand"], "brand", i)
		if err != nil {
			return nil, err
		}

		noSKU, _ := parseCatalogActive(p["no_sku"])
		isReturn, _ := parseCatalogActive(p["is_return"])
		returnQty, _ := parseCatalogStock(p["return_qty"])
		sealedQty := stock - returnQty
		if sealedQty < 0 {
			sealedQty = 0
		}
		if rawSealed, ok := p["sealed_qty"]; ok && rawSealed != nil {
			if sq, sqErr := parseCatalogStock(rawSealed); sqErr == nil {
				sealedQty = sq
			}
		}
		displaySKU := lib.Text(sku)
		if ds, err := catalogNullable(p["display_sku"], "display_sku", i); err == nil && ds.Type == "text" {
			displaySKU = ds
		} else if noSKU == 1 {
			displaySKU = lib.Text("NO SKU")
		}
		skuWarning, _ := catalogNullable(p["sku_warning"], "sku_warning", i)
		returnLabel, _ := catalogNullable(p["return_label"], "return_label", i)
		priceStatus := "NO_PRICE"
		if ps := strings.TrimSpace(lib.ToString(p["price_status"])); ps != "" && ps != "<nil>" {
			priceStatus = ps
		} else if salePrice != "0" || (pinned.Type == "text" && lib.ToString(pinned.Value) != "") {
			priceStatus = "PRICED"
		}

		row := catalogRow{
			sku:         sku,
			name:        name,
			pinned:      pinned,
			salePrice:   salePrice,
			vatRate:     vatRate,
			stock:       stock,
			active:      active,
			category:    category,
			brand:       brand,
			noSKU:       noSKU,
			displaySKU:  displaySKU,
			skuWarning:  skuWarning,
			isReturn:    isReturn,
			returnQty:   returnQty,
			sealedQty:   sealedQty,
			returnLabel: returnLabel,
			priceStatus: priceStatus,
		}
		if existing, ok := indexBySKU[sku]; ok {
			rows[existing] = row
			continue
		}
		indexBySKU[sku] = len(rows)
		rows = append(rows, row)
	}
	return rows, nil
}

// cleanCatalogText drops C0/C1 control characters (including NUL) and trims
// surrounding whitespace. Printable unicode, including non-Latin scripts and
// emoji, is preserved verbatim.
func cleanCatalogText(s string, maxRunes int) (string, error) {
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

// parseCatalogDecimal accepts numbers or numeric strings, including decimal
// and scientific notation, and canonicalizes them to a finite, non-negative
// decimal string. NaN, Infinity and other non-numeric values are rejected.
func canonicalDecimalText(s string) string {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, ",") {
		return s
	}
	if strings.Contains(s, ".") {
		// "1,234.50" -> thousands separator, dot is the decimal mark.
		return strings.ReplaceAll(s, ",", "")
	}
	parts := strings.Split(s, ",")
	// "10,50" is a French decimal; "1,000" is a thousands group.
	last := parts[len(parts)-1]
	if len(last) > 0 && len(last) <= 2 {
		return strings.Join(parts[:len(parts)-1], "") + "." + last
	}
	return strings.ReplaceAll(s, ",", "")
}

// parseCatalogPin keeps the three pin states distinct. A missing key and a
// JSON null both become SQL NULL, which the upsert treats as "preserve"; an
// empty string becomes SQL empty text (explicit clear); a decimal sets the pin.
func parseCatalogPin(product map[string]interface{}, i int) (lib.TursoArg, error) {
	raw, ok := product["pinned_price"]
	if !ok || raw == nil {
		raw = product["pinnedPrice"]
	}
	if raw == nil {
		return lib.Null(), nil
	}
	s := lib.ToString(raw)
	if s == "" {
		return lib.Text(""), nil
	}
	if canonicalDecimalText(s) == "" {
		return lib.Null(), fmt.Errorf("product %d pinned_price: must be a decimal string or an empty string", i)
	}
	value, err := parseCatalogDecimal(raw, "")
	if err != nil {
		return lib.Null(), fmt.Errorf("product %d pinned_price: %w", i, err)
	}
	if value == "" {
		return lib.Null(), fmt.Errorf("product %d pinned_price: must be a decimal string or an empty string", i)
	}
	// Keep the submitted scale ("149.00" stays "149.00"). Only exponent
	// notation needs expanding to plain decimal after validation.
	text := canonicalDecimalText(s)
	if strings.ContainsAny(text, "eE") {
		text = value
	}
	return lib.Text(text), nil
}

func parseCatalogDecimal(v interface{}, fallback string) (string, error) {
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
	if len(out) > catalogMaxMoneyRunes {
		return "", errors.New("number is too large")
	}
	return out, nil
}

func parseCatalogStock(v interface{}) (int64, error) {
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
	if f > float64(catalogMaxStock) {
		return 0, errors.New("stock is too large")
	}
	return int64(f), nil
}

func parseCatalogActive(v interface{}) (int64, error) {
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

func catalogNullable(v interface{}, field string, i int) (lib.TursoArg, error) {
	if v == nil {
		return lib.Null(), nil
	}
	s, err := cleanCatalogText(lib.ToString(v), catalogMaxTermRunes)
	if err != nil {
		return lib.Null(), fmt.Errorf("product %d %s: %w", i, field, err)
	}
	if s == "" || s == "<nil>" {
		return lib.Null(), nil
	}
	return lib.Text(s), nil
}
