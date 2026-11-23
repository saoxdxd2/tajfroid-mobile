package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	db := lib.NewTurso()
	switch r.Method {
	case http.MethodGet:
		invoiceID := r.URL.Query().Get("invoiceId")
		if invoiceID == "" {
			http.Error(w, "invoiceId required", http.StatusBadRequest)
			return
		}
		items, err := listItems(db, invoiceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"items": items})

	case http.MethodPost:
		var body struct {
			ID                string `json:"id"` // optional client idempotency key
			InvoiceID         string `json:"invoiceId"`
			ProductSKU        string `json:"productSku"`
			ProductName       string `json:"productName"`
			Quantity          int    `json:"quantity"`
			UnitPrice         string `json:"unitPrice"`
			VatRate           string `json:"vatRate"`
			UnitCost          string `json:"unitCost"`
			WarrantyMonths    int    `json:"warrantyMonths"`
			DescriptionOverride string `json:"descriptionOverride"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.ID != "" {
			if ex, err := db.Execute("SELECT id FROM invoice_items WHERE id=?", lib.Text(body.ID)); err == nil && len(ex.Rows) > 0 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{"id": body.ID, "idempotent": true})
				return
			}
		}
		if body.InvoiceID == "" || body.ProductSKU == "" {
			http.Error(w, "invoiceId and productSku required", http.StatusBadRequest)
			return
		}
		if body.Quantity <= 0 {
			http.Error(w, "quantity must be positive", http.StatusBadRequest)
			return
		}
		inv, err := getInvoiceState(db, body.InvoiceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if inv != "Draft" {
			http.Error(w, "forbidden: cannot modify an invoice after it has left Draft state", http.StatusBadRequest)
			return
		}
		prod, err := db.Execute("SELECT name, purchase_price, sale_price, vat_rate, active FROM catalog WHERE sku = ?", lib.Text(body.ProductSKU))
		if err != nil || len(prod.Rows) == 0 {
			http.Error(w, "product not found", http.StatusNotFound)
			return
		}
		if lib.ToInt(prod.Rows[0][4]) != 1 {
			http.Error(w, "product is archived", http.StatusBadRequest)
			return
		}
		name := body.ProductName
		if name == "" {
			name = lib.ToString(prod.Rows[0][0])
		}
		unitPrice := body.UnitPrice
		if unitPrice == "" {
			unitPrice = lib.ToString(prod.Rows[0][2])
		}
		vatRate := body.VatRate
		if vatRate == "" {
			vatRate = lib.ToString(prod.Rows[0][3])
		}
		unitCost := body.UnitCost
		if unitCost == "" {
			unitCost = lib.ToString(prod.Rows[0][1])
		}
		if lib.ParseMoney(unitPrice) < 0 || lib.ParseRate(vatRate) < 0 || lib.ParseMoney(unitCost) < 0 {
			http.Error(w, "prices and VAT cannot be negative", http.StatusBadRequest)
			return
		}
		warranty := body.WarrantyMonths
		if warranty < 0 {
			warranty = 0
		}
		if warranty == 0 {
			warranty = 12
		}
		id := body.ID
		if id == "" {
			id = lib.NewID()
		}
		now := time.Now().Unix()
		_, err = db.Execute(`INSERT INTO invoice_items
			(id, invoice_id, product_sku, product_name, quantity, unit_price, vat_rate,
			 unit_cost, warranty_months, description_override, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			lib.Text(id), lib.Text(body.InvoiceID), lib.Text(body.ProductSKU), lib.Text(name),
			lib.Int(int64(body.Quantity)), lib.Text(lib.FormatMoney(lib.ParseMoney(unitPrice))),
			lib.Text(formatRate(vatRate)), lib.Text(lib.FormatMoney(lib.ParseMoney(unitCost))),
			lib.Int(int64(warranty)), nullableText(body.DescriptionOverride),
			lib.Int(now), lib.Int(now))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		total, err := recalcTotal(db, body.InvoiceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "totalAmount": lib.FormatMoney(total)})

	case http.MethodDelete:
		itemID := r.URL.Query().Get("id")
		if itemID == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		res, err := db.Execute("SELECT invoice_id FROM invoice_items WHERE id = ?", lib.Text(itemID))
		if err != nil || len(res.Rows) == 0 {
			http.Error(w, "item not found", http.StatusNotFound)
			return
		}
		invoiceID := lib.ToString(res.Rows[0][0])
		if v := r.URL.Query().Get("baseUpdatedAt"); v != "" {
			if base, err := strconv.ParseInt(v, 10, 64); err == nil && base > 0 {
				if cur, err := db.Execute("SELECT updated_at FROM invoices WHERE id=?", lib.Text(invoiceID)); err == nil && len(cur.Rows) > 0 && lib.ToInt64(cur.Rows[0][0]) != base {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					json.NewEncoder(w).Encode(map[string]interface{}{"error": "conflict: record changed on another device", "currentUpdatedAt": lib.ToInt64(cur.Rows[0][0])})
					return
				}
			}
		}
		state, err := getInvoiceState(db, invoiceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if state != "Draft" {
			http.Error(w, "forbidden: cannot modify an invoice after it has left Draft state", http.StatusBadRequest)
			return
		}
		if _, err := db.Execute("DELETE FROM invoice_items WHERE id = ?", lib.Text(itemID)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		total, err := recalcTotal(db, invoiceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "totalAmount": lib.FormatMoney(total)})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func listItems(db *lib.TursoClient, invoiceID string) ([]map[string]interface{}, error) {
	res, err := db.Execute(`SELECT id, product_sku, product_name, quantity, unit_price, vat_rate,
		unit_cost, warranty_months, description_override, warranty_end_date, created_at
		FROM invoice_items WHERE invoice_id = ? ORDER BY created_at ASC`, lib.Text(invoiceID))
	if err != nil {
		return nil, err
	}
	items := []map[string]interface{}{}
	for _, row := range res.Rows {
		items = append(items, map[string]interface{}{
			"id": row[0], "productSku": row[1], "productName": nullableStr(row[2]),
			"quantity": lib.ToInt(row[3]), "unitPrice": row[4], "vatRate": row[5],
			"unitCost": row[6], "warrantyMonths": lib.ToInt(row[7]),
			"descriptionOverride": nullableStr(row[8]), "warrantyEndDate": row[9],
			"createdAt": row[10],
		})
	}
	return items, nil
}

func getInvoiceState(db *lib.TursoClient, id string) (string, error) {
	res, err := db.Execute("SELECT state FROM invoices WHERE id = ?", lib.Text(id))
	if err != nil {
		return "", err
	}
	if len(res.Rows) == 0 {
		return "", errText("invoice not found")
	}
	return lib.ToString(res.Rows[0][0]), nil
}

// recalcTotal recomputes the VAT-inclusive total from the stored line items,
// using integer cents so the result matches the desktop to the cent.
func recalcTotal(db *lib.TursoClient, invoiceID string) (int64, error) {
	res, err := db.Execute("SELECT quantity, unit_price, vat_rate FROM invoice_items WHERE invoice_id = ?", lib.Text(invoiceID))
	if err != nil {
		return 0, err
	}
	total := int64(0)
	for _, row := range res.Rows {
		total += lib.LineTotalCents(
			lib.ToInt(row[0]),
			lib.ParseMoney(lib.ToString(row[1])),
			lib.ParseRate(lib.ToString(row[2])),
		)
	}
	now := time.Now().Unix()
	_, err = db.Execute("UPDATE invoices SET total_amount=?, updated_at=? WHERE id=?",
		lib.Text(lib.FormatMoney(total)), lib.Int(now), lib.Text(invoiceID))
	return total, err
}

func formatRate(s string) string {
	return strconv.FormatFloat(lib.ParseRate(s), 'f', 2, 64)
}

type errText string

func (e errText) Error() string { return string(e) }

func nullableStr(v interface{}) string {
	if v == nil {
		return ""
	}
	return lib.ToString(v)
}

func nullableText(s string) lib.TursoArg {
	if s == "" {
		return lib.Null()
	}
	return lib.Text(s)
}
