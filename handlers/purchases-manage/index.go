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
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		purchase, err := loadPurchase(db, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(purchase)

	case http.MethodPut:
		var body struct {
			Action           string `json:"action"`
			SupplierID       string `json:"supplierId"`
			Reference        string `json:"reference"`
			PurchaseDate     int64  `json:"purchaseDate"`
			SupplierInvoiceNo string `json:"supplierInvoiceNo"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		switch body.Action {
		case "":
			if err := updatePurchase(db, id, body.SupplierID, body.Reference, body.PurchaseDate, body.SupplierInvoiceNo); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "validate":
			if err := validatePurchase(db, id); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
		purchase, err := loadPurchase(db, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "purchase": purchase})

	case http.MethodDelete:
		state, err := purchaseState(db, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if state != "Draft" {
			http.Error(w, "only a Draft purchase can be deleted", http.StatusBadRequest)
			return
		}
		db.Execute("DELETE FROM purchase_items WHERE purchase_id = ?", lib.Text(id))
		if _, err := db.Execute("DELETE FROM purchases WHERE id = ? AND state = 'Draft'", lib.Text(id)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func updatePurchase(db *lib.TursoClient, id, supplierID, reference string, purchaseDate int64, supplierInvoiceNo string) error {
	state, err := purchaseState(db, id)
	if err != nil {
		return err
	}
	if state != "Draft" {
		return errText("only a Draft purchase can be edited")
	}
	now := time.Now().Unix()
	if supplierID != "" {
		res, err := db.Execute("SELECT id FROM suppliers WHERE id = ? AND is_archived = 0", lib.Text(supplierID))
		if err != nil || len(res.Rows) == 0 {
			return errText("supplier not found")
		}
		db.Execute("UPDATE purchases SET supplier_id=? WHERE id=?", lib.Text(supplierID), lib.Text(id))
	}
	if strings.TrimSpace(reference) != "" {
		if _, err := db.Execute("UPDATE purchases SET reference=? WHERE id=?", lib.Text(strings.TrimSpace(reference)), lib.Text(id)); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return errText("this purchase reference is already used")
			}
			return err
		}
	}
	if purchaseDate > 0 {
		db.Execute("UPDATE purchases SET purchase_date=? WHERE id=?", lib.Int(purchaseDate), lib.Text(id))
	}
	if strings.TrimSpace(supplierInvoiceNo) != "" {
		db.Execute("UPDATE purchases SET supplier_invoice_no=? WHERE id=?", lib.Text(supplierInvoiceNo), lib.Text(id))
	}
	_, err = db.Execute("UPDATE purchases SET updated_at=? WHERE id=?", lib.Int(now), lib.Text(id))
	return err
}

// validatePurchase adds every line back into stock, exactly like the desktop
// PurchasingService.validate_purchase, then marks the purchase Validated.
func validatePurchase(db *lib.TursoClient, id string) error {
	state, err := purchaseState(db, id)
	if err != nil {
		return err
	}
	if state != "Draft" {
		return errText("purchase is not in Draft state")
	}
	res, err := db.Execute("SELECT product_sku, quantity FROM purchase_items WHERE purchase_id = ?", lib.Text(id))
	if err != nil {
		return err
	}
	if len(res.Rows) == 0 {
		return errText("cannot validate a purchase with no items")
	}
	type line struct {
		sku string
		qty int
	}
	lines := make([]line, 0, len(res.Rows))
	for _, row := range res.Rows {
		lines = append(lines, line{sku: lib.ToString(row[0]), qty: lib.ToInt(row[1])})
	}
	// Verify every product exists before touching stock.
	for _, l := range lines {
		check, err := db.Execute("SELECT sku FROM catalog WHERE sku = ?", lib.Text(l.sku))
		if err != nil || len(check.Rows) == 0 {
			return errText("product not found: " + l.sku)
		}
	}
	now := time.Now().Unix()
	for _, l := range lines {
		if _, err := db.Execute("UPDATE catalog SET stock = stock + ? WHERE sku = ?", lib.Int(int64(l.qty)), lib.Text(l.sku)); err != nil {
			return err
		}
	}
	if _, err := db.Execute("UPDATE purchases SET state=?, updated_at=? WHERE id=?", lib.Text("Validated"), lib.Int(now), lib.Text(id)); err != nil {
		return err
	}
	ref, _ := db.Execute("SELECT reference, supplier_id, total_amount FROM purchases WHERE id = ?", lib.Text(id))
	if len(ref.Rows) > 0 {
		lib.Journal(db, lib.TxPurchase, "PUR-"+lib.ToString(ref.Rows[0][0]),
			"Purchase validated from supplier "+lib.ToString(ref.Rows[0][1]),
			-lib.ParseMoney(lib.ToString(ref.Rows[0][2])), time.Now())
	}
	db.Execute("INSERT INTO catalog_meta (id, version, updated_at) VALUES (1, 1, ?) ON CONFLICT(id) DO UPDATE SET version = version + 1, updated_at = excluded.updated_at", lib.Int(now))
	return nil
}

func purchaseState(db *lib.TursoClient, id string) (string, error) {
	res, err := db.Execute("SELECT state FROM purchases WHERE id = ?", lib.Text(id))
	if err != nil {
		return "", err
	}
	if len(res.Rows) == 0 {
		return "", errText("purchase not found")
	}
	return lib.ToString(res.Rows[0][0]), nil
}

func loadPurchase(db *lib.TursoClient, id string) (map[string]interface{}, error) {
	res, err := db.Execute(`SELECT p.id, p.reference, p.supplier_id, s.company_name, p.state,
		p.total_amount, p.purchase_date, p.supplier_invoice_no, p.source_file_name,
		p.created_at, p.updated_at
		FROM purchases p LEFT JOIN suppliers s ON p.supplier_id = s.id WHERE p.id = ?`, lib.Text(id))
	if err != nil {
		return nil, err
	}
	if len(res.Rows) == 0 {
		return nil, errText("purchase not found")
	}
	row := res.Rows[0]
	purchase := map[string]interface{}{
		"id": row[0], "reference": row[1], "supplierId": row[2],
		"supplierName": nullableStr(row[3]), "state": row[4],
		"totalAmount": lib.FormatMoney(lib.ParseMoney(lib.ToString(row[5]))),
		"purchaseDate": row[6], "supplierInvoiceNo": nullableStr(row[7]),
		"sourceFileName": nullableStr(row[8]), "createdAt": row[9], "updatedAt": row[10],
	}
	itemRes, err := db.Execute(`SELECT id, product_sku, product_name, quantity, unit_cost, created_at
		FROM purchase_items WHERE purchase_id = ? ORDER BY created_at ASC`, lib.Text(id))
	if err != nil {
		return nil, err
	}
	items := []map[string]interface{}{}
	for _, it := range itemRes.Rows {
		items = append(items, map[string]interface{}{
			"id": it[0], "productSku": it[1], "productName": nullableStr(it[2]),
			"quantity": lib.ToInt(it[3]), "unitCost": it[4], "createdAt": it[5],
		})
	}
	purchase["items"] = items
	return purchase, nil
}

type errText string

func (e errText) Error() string { return string(e) }

func nullableStr(v interface{}) string {
	if v == nil {
		return ""
	}
	return lib.ToString(v)
}
