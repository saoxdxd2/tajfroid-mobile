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
		detail, err := loadInvoice(db, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(detail)

	case http.MethodPut:
		var body struct {
			Action     string `json:"action"`
			CustomerID string `json:"customerId"`
			Channel    string `json:"channel"`
			DueDate    int64  `json:"dueDate"`
			Notes      string `json:"notes"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		switch body.Action {
		case "":
			if err := updateInvoiceFields(db, id, body.CustomerID, body.Channel, body.DueDate, body.Notes); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "validate":
			if err := validateInvoice(db, id); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "issue":
			if err := transitionInvoice(db, id, []string{"Validated"}, "Issued"); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "archive":
			if err := transitionInvoice(db, id, []string{"Draft", "Validated", "Issued", "Paid"}, "Archived"); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
		detail, err := loadInvoice(db, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "invoice": detail["invoice"]})

	case http.MethodDelete:
		if err := transitionInvoice(db, id, []string{"Draft", "Validated", "Issued", "Paid"}, "Archived"); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func updateInvoiceFields(db *lib.TursoClient, id, customerID, channel string, dueDate int64, notes string) error {
	inv, err := getInvoice(db, id)
	if err != nil {
		return err
	}
	if lib.ToString(inv["state"]) != "Draft" {
		return errText("forbidden: only a Draft invoice can be edited")
	}
	now := time.Now().Unix()
	if customerID != "" {
		res, err := db.Execute("SELECT id FROM customers WHERE id = ? AND is_archived = 0", lib.Text(customerID))
		if err != nil || len(res.Rows) == 0 {
			return errText("customer not found")
		}
		db.Execute("UPDATE invoices SET customer_id=? WHERE id=?", lib.Text(customerID), lib.Text(id))
	}
	if channel != "" {
		if channel != "store" && channel != "website" && channel != "mobile_app" {
			return errText("invalid channel")
		}
		db.Execute("UPDATE invoices SET channel=? WHERE id=?", lib.Text(channel), lib.Text(id))
	}
	if dueDate > 0 {
		db.Execute("UPDATE invoices SET due_date=? WHERE id=?", lib.Int(dueDate), lib.Text(id))
	}
	if strings.TrimSpace(notes) != "" {
		db.Execute("UPDATE invoices SET notes=? WHERE id=?", lib.Text(notes), lib.Text(id))
	}
	_, err = db.Execute("UPDATE invoices SET updated_at=? WHERE id=?", lib.Int(now), lib.Text(id))
	return err
}

// validateInvoice is the multi-domain transaction from the desktop ERP:
// stock goes down (never below zero) and the invoice becomes Validated.
func validateInvoice(db *lib.TursoClient, id string) error {
	inv, err := getInvoice(db, id)
	if err != nil {
		return err
	}
	if lib.ToString(inv["state"]) != "Draft" {
		return errText("invoice is not in Draft state")
	}
	res, err := db.Execute("SELECT product_sku, quantity, warranty_months FROM invoice_items WHERE invoice_id = ?", lib.Text(id))
	if err != nil {
		return err
	}
	if len(res.Rows) == 0 {
		return errText("cannot validate an invoice with no items")
	}
	type line struct {
		sku  string
		qty  int
		warr int
	}
	lines := make([]line, 0, len(res.Rows))
	for _, row := range res.Rows {
		lines = append(lines, line{
			sku: lib.ToString(row[0]), qty: lib.ToInt(row[1]), warr: lib.ToInt(row[2]),
		})
	}
	// Check every line before mutating anything, so a failure leaves stock intact.
	for _, l := range lines {
		stockRes, err := db.Execute("SELECT stock, name FROM catalog WHERE sku = ?", lib.Text(l.sku))
		if err != nil || len(stockRes.Rows) == 0 {
			return errText("product not found: " + l.sku)
		}
		if lib.ToInt(stockRes.Rows[0][0]) < l.qty {
			return errText("insufficient stock for " + lib.ToString(stockRes.Rows[0][1]) + " (" + l.sku + ")")
		}
	}
	now := time.Now().Unix()
	for _, l := range lines {
		if _, err := db.Execute("UPDATE catalog SET stock = stock - ? WHERE sku = ? AND stock >= ?",
			lib.Int(int64(l.qty)), lib.Text(l.sku), lib.Int(int64(l.qty))); err != nil {
			return err
		}
		if l.warr > 0 {
			end := now + int64(l.warr)*30*24*3600
			db.Execute("UPDATE invoice_items SET warranty_end_date=? WHERE invoice_id=? AND product_sku=?",
				lib.Int(end), lib.Text(id), lib.Text(l.sku))
		}
	}
	if _, err := db.Execute("UPDATE invoices SET state=?, updated_at=? WHERE id=?", lib.Text("Validated"), lib.Int(now), lib.Text(id)); err != nil {
		return err
	}
	lib.Journal(db, lib.TxSale, "INV-"+lib.ToString(inv["invoiceNumber"]),
		"Invoice validated for "+lib.ToString(inv["customerId"]),
		lib.ParseMoney(lib.ToString(inv["totalAmount"])), time.Now())
	bumpCatalog(db, now)
	return nil
}

func transitionInvoice(db *lib.TursoClient, id string, from []string, to string) error {
	inv, err := getInvoice(db, id)
	if err != nil {
		return err
	}
	state := lib.ToString(inv["state"])
	allowed := false
	for _, s := range from {
		if s == state {
			allowed = true
			break
		}
	}
	if !allowed {
		return errText("invoice is " + state + "; cannot move to " + to)
	}
	now := time.Now().Unix()
	_, err = db.Execute("UPDATE invoices SET state=?, updated_at=? WHERE id=?", lib.Text(to), lib.Int(now), lib.Text(id))
	return err
}

func getInvoice(db *lib.TursoClient, id string) (map[string]interface{}, error) {
	res, err := db.Execute("SELECT id, invoice_number, customer_id, state, total_amount, channel, due_date, notes, created_at, updated_at FROM invoices WHERE id = ?", lib.Text(id))
	if err != nil {
		return nil, err
	}
	if len(res.Rows) == 0 {
		return nil, errText("invoice not found")
	}
	row := res.Rows[0]
	return map[string]interface{}{
		"id": row[0], "invoiceNumber": row[1], "customerId": row[2],
		"state": row[3], "totalAmount": row[4], "channel": row[5],
		"dueDate": row[6], "notes": row[7], "createdAt": row[8], "updatedAt": row[9],
	}, nil
}

func loadInvoice(db *lib.TursoClient, id string) (map[string]interface{}, error) {
	inv, err := getInvoice(db, id)
	if err != nil {
		return nil, err
	}
	custName := ""
	custRes, _ := db.Execute("SELECT company_name, phone FROM customers WHERE id = ?", lib.Text(lib.ToString(inv["customerId"])))
	if len(custRes.Rows) > 0 {
		custName = lib.ToString(custRes.Rows[0][0])
		inv["customerName"] = custName
		inv["customerPhone"] = nullableStr(custRes.Rows[0][1])
	}
	inv["totalAmount"] = lib.FormatMoney(lib.ParseMoney(lib.ToString(inv["totalAmount"])))

	itemRes, err := db.Execute(`SELECT id, product_sku, product_name, quantity, unit_price, vat_rate,
		unit_cost, warranty_months, description_override, warranty_end_date, created_at
		FROM invoice_items WHERE invoice_id = ? ORDER BY created_at ASC`, lib.Text(id))
	if err != nil {
		return nil, err
	}
	items := []map[string]interface{}{}
	for _, row := range itemRes.Rows {
		items = append(items, map[string]interface{}{
			"id": row[0], "productSku": row[1], "productName": nullableStr(row[2]),
			"quantity": lib.ToInt(row[3]), "unitPrice": row[4], "vatRate": row[5],
			"unitCost": row[6], "warrantyMonths": lib.ToInt(row[7]),
			"descriptionOverride": nullableStr(row[8]), "warrantyEndDate": row[9],
			"createdAt": row[10],
		})
	}

	payRes, err := db.Execute("SELECT id, method, amount, reference, notes, created_at FROM payments WHERE invoice_id = ? ORDER BY created_at ASC", lib.Text(id))
	if err != nil {
		return nil, err
	}
	payments := []map[string]interface{}{}
	paid := int64(0)
	for _, row := range payRes.Rows {
		amount := lib.ParseMoney(lib.ToString(row[2]))
		paid += amount
		payments = append(payments, map[string]interface{}{
			"id": row[0], "method": row[1], "amount": lib.FormatMoney(amount),
			"reference": nullableStr(row[3]), "notes": nullableStr(row[4]), "createdAt": row[5],
		})
	}
	total := lib.ParseMoney(lib.ToString(inv["totalAmount"]))
	inv["paidAmount"] = lib.FormatMoney(paid)
	inv["balance"] = lib.FormatMoney(total - paid)
	inv["items"] = items
	inv["payments"] = payments
	return inv, nil
}

func bumpCatalog(db *lib.TursoClient, now int64) {
	db.Execute("INSERT INTO catalog_meta (id, version, updated_at) VALUES (1, 1, ?) ON CONFLICT(id) DO UPDATE SET version = version + 1, updated_at = excluded.updated_at", lib.Int(now))
}

type errText string

func (e errText) Error() string { return string(e) }

func nullableStr(v interface{}) string {
	if v == nil {
		return ""
	}
	return lib.ToString(v)
}
