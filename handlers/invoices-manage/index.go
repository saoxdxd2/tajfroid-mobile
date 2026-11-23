package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
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
			Action        string `json:"action"`
			CustomerID    string `json:"customerId"`
			Channel       string `json:"channel"`
			DueDate       int64  `json:"dueDate"`
			Notes         string `json:"notes"`
			BaseUpdatedAt int64  `json:"baseUpdatedAt"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.BaseUpdatedAt > 0 {
			cur, err := db.Execute("SELECT updated_at FROM invoices WHERE id=?", lib.Text(id))
			if err == nil && len(cur.Rows) > 0 && lib.ToInt64(cur.Rows[0][0]) != body.BaseUpdatedAt {
				lib.WriteConflict(w, lib.ToInt64(cur.Rows[0][0]))
				return
			}
		}
		switch body.Action {
		case "":
			if err := updateInvoiceFields(db, id, body.CustomerID, body.Channel, body.DueDate, body.Notes); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "validate":
			if err := validateInvoice(r.Context(), db, id); err != nil {
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
		if v := r.URL.Query().Get("baseUpdatedAt"); v != "" {
			if base, err := strconv.ParseInt(v, 10, 64); err == nil && base > 0 {
				if cur, err := db.Execute("SELECT updated_at FROM invoices WHERE id=?", lib.Text(id)); err == nil && len(cur.Rows) > 0 && lib.ToInt64(cur.Rows[0][0]) != base {
					lib.WriteConflict(w, lib.ToInt64(cur.Rows[0][0]))
					return
				}
			}
		}
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
// The old per-line loop cost up to 3 pipeline POSTs per line; the reads and
// writes now each fit in a single ExecuteBatch call.
func validateInvoice(ctx context.Context, db *lib.TursoClient, id string) error {
	reads, err := db.ExecuteBatch(ctx,
		lib.TursoStmt{
			SQL:  "SELECT id, invoice_number, customer_id, state, total_amount, channel, due_date, notes, created_at, updated_at FROM invoices WHERE id = ?",
			Args: []lib.TursoArg{lib.Text(id)},
		},
		lib.TursoStmt{
			SQL:  "SELECT product_sku, quantity, warranty_months FROM invoice_items WHERE invoice_id = ?",
			Args: []lib.TursoArg{lib.Text(id)},
		},
	)
	if err != nil {
		return err
	}
	if len(reads[0].Rows) == 0 {
		return errText("invoice not found")
	}
	inv := invoiceFromRow(reads[0].Rows[0])
	if lib.ToString(inv["state"]) != "Draft" {
		return errText("invoice is not in Draft state")
	}
	if len(reads[1].Rows) == 0 {
		return errText("cannot validate an invoice with no items")
	}
	type line struct {
		sku  string
		qty  int
		warr int
	}
	lines := make([]line, 0, len(reads[1].Rows))
	for _, row := range reads[1].Rows {
		lines = append(lines, line{
			sku: lib.ToString(row[0]), qty: lib.ToInt(row[1]), warr: lib.ToInt(row[2]),
		})
	}

	// One stock check per unique SKU, with quantities summed so repeated SKU
	// lines cannot each spend the same units.
	skus := make([]string, 0, len(lines))
	needed := make(map[string]int, len(lines))
	for _, l := range lines {
		if _, ok := needed[l.sku]; !ok {
			skus = append(skus, l.sku)
		}
		needed[l.sku] += l.qty
	}
	checks := make([]lib.TursoStmt, 0, len(skus))
	for _, sku := range skus {
		checks = append(checks, lib.TursoStmt{
			SQL:  "SELECT stock, name FROM catalog WHERE sku = ?",
			Args: []lib.TursoArg{lib.Text(sku)},
		})
	}
	stockRows, err := db.ExecuteBatch(ctx, checks...)
	if err != nil {
		return err
	}
	// Verify every SKU before mutating anything, so a failure leaves stock intact.
	for i, sku := range skus {
		if len(stockRows[i].Rows) == 0 {
			return errText("product not found: " + sku)
		}
		row := stockRows[i].Rows[0]
		if lib.ToInt(row[0]) < needed[sku] {
			return errText("insufficient stock for " + lib.ToString(row[1]) + " (" + sku + ")")
		}
	}

	now := time.Now().Unix()
	writes := make([]lib.TursoStmt, 0, len(skus)+len(lines)+3)
	for _, sku := range skus {
		qty := needed[sku]
		writes = append(writes, lib.TursoStmt{
			SQL:  "UPDATE catalog SET stock = stock - ? WHERE sku = ? AND stock >= ?",
			Args: []lib.TursoArg{lib.Int(int64(qty)), lib.Text(sku), lib.Int(int64(qty))},
		})
	}
	for _, l := range lines {
		if l.warr > 0 {
			end := now + int64(l.warr)*30*24*3600
			writes = append(writes, lib.TursoStmt{
				SQL:  "UPDATE invoice_items SET warranty_end_date=? WHERE invoice_id=? AND product_sku=?",
				Args: []lib.TursoArg{lib.Int(end), lib.Text(id), lib.Text(l.sku)},
			})
		}
	}
	writes = append(writes,
		lib.TursoStmt{
			SQL:  "UPDATE invoices SET state=?, updated_at=? WHERE id=?",
			Args: []lib.TursoArg{lib.Text("Validated"), lib.Int(now), lib.Text(id)},
		},
		saleJournalStmt(inv, now),
		catalogMetaStmt(now),
	)
	_, err = db.ExecuteBatch(ctx, writes...)
	return err
}

func invoiceFromRow(row []interface{}) map[string]interface{} {
	return map[string]interface{}{
		"id": row[0], "invoiceNumber": row[1], "customerId": row[2],
		"state": row[3], "totalAmount": row[4], "channel": row[5],
		"dueDate": row[6], "notes": row[7], "createdAt": row[8], "updatedAt": row[9],
	}
}

func saleJournalStmt(inv map[string]interface{}, now int64) lib.TursoStmt {
	return lib.TursoStmt{
		SQL: `INSERT INTO journal_entries
		(id, transaction_type, reference_id, description, amount, is_reversed, created_at)
		VALUES (?,?,?,?,?,?,?)`,
		Args: []lib.TursoArg{
			lib.Text(lib.NewID()), lib.Text(lib.TxSale),
			lib.Text("INV-" + lib.ToString(inv["invoiceNumber"])),
			lib.Text("Invoice validated for " + lib.ToString(inv["customerId"])),
			lib.Text(lib.FormatMoney(lib.ParseMoney(lib.ToString(inv["totalAmount"])))),
			lib.Int(0), lib.Int(now),
		},
	}
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
	return invoiceFromRow(res.Rows[0]), nil
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
		inv["customerPhone"] = lib.NullStr(custRes.Rows[0][1])
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
			"id": row[0], "productSku": row[1], "productName": lib.NullStr(row[2]),
			"quantity": lib.ToInt(row[3]), "unitPrice": row[4], "vatRate": row[5],
			"unitCost": row[6], "warrantyMonths": lib.ToInt(row[7]),
			"descriptionOverride": lib.NullStr(row[8]), "warrantyEndDate": row[9],
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
			"reference": lib.NullStr(row[3]), "notes": lib.NullStr(row[4]), "createdAt": row[5],
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
	stmt := catalogMetaStmt(now)
	db.Execute(stmt.SQL, stmt.Args...)
}

func catalogMetaStmt(now int64) lib.TursoStmt {
	return lib.TursoStmt{
		SQL:  lib.CatalogMetaBumpSQL,
		Args: []lib.TursoArg{lib.Int(now), lib.Int(now)},
	}
}

type errText string

func (e errText) Error() string { return string(e) }
