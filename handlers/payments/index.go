package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"tajfroid-mobile/lib"
)

var paymentMethods = map[string]bool{
	"Cash": true, "Check": true, "Bank Transfer": true,
}

func Handler(w http.ResponseWriter, r *http.Request) {
	db := lib.NewTurso()
	switch r.Method {
	case http.MethodGet:
		invoiceID := r.URL.Query().Get("invoiceId")
		if invoiceID == "" {
			http.Error(w, "invoiceId required", http.StatusBadRequest)
			return
		}
		res, err := db.Execute("SELECT id, method, amount, reference, notes, created_at FROM payments WHERE invoice_id = ? ORDER BY created_at ASC", lib.Text(invoiceID))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := []map[string]interface{}{}
		for _, row := range res.Rows {
			items = append(items, map[string]interface{}{
				"id": row[0], "method": row[1], "amount": row[2],
				"reference": nullableStr(row[3]), "notes": nullableStr(row[4]),
				"createdAt": row[5],
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"payments": items})

	case http.MethodPost:
		var body struct {
			InvoiceID string `json:"invoiceId"`
			Method    string `json:"method"`
			Amount    string `json:"amount"`
			Reference string `json:"reference"`
			Notes     string `json:"notes"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.InvoiceID == "" {
			http.Error(w, "invoiceId required", http.StatusBadRequest)
			return
		}
		if body.Method == "" {
			body.Method = "Cash"
		}
		if !paymentMethods[body.Method] {
			http.Error(w, "invalid payment method (Cash, Check, Bank Transfer)", http.StatusBadRequest)
			return
		}
		amount := lib.ParseMoney(body.Amount)
		if amount <= 0 {
			http.Error(w, "payment amount must be strictly positive", http.StatusBadRequest)
			return
		}
		if body.Method == "Check" && body.Reference == "" {
			http.Error(w, "a check payment needs its check number as reference", http.StatusBadRequest)
			return
		}
		res, err := db.Execute("SELECT state, total_amount, invoice_number FROM invoices WHERE id = ?", lib.Text(body.InvoiceID))
		if err != nil || len(res.Rows) == 0 {
			http.Error(w, "invoice not found", http.StatusNotFound)
			return
		}
		state := lib.ToString(res.Rows[0][0])
		if state != "Validated" && state != "Issued" {
			http.Error(w, "payments are only allowed on validated/issued invoices", http.StatusBadRequest)
			return
		}
		total := lib.ParseMoney(lib.ToString(res.Rows[0][1]))
		paid, err := paidForInvoice(db, body.InvoiceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		balance := total - paid
		if amount > balance {
			http.Error(w, "payment exceeds remaining balance of "+lib.FormatMoney(balance), http.StatusBadRequest)
			return
		}
		id := lib.NewID()
		now := time.Now().Unix()
		_, err = db.Execute(`INSERT INTO payments
			(id, invoice_id, method, amount, reference, notes, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?)`,
			lib.Text(id), lib.Text(body.InvoiceID), lib.Text(body.Method),
			lib.Text(lib.FormatMoney(amount)), nullableText(body.Reference),
			nullableText(body.Notes), lib.Int(now), lib.Int(now))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newBalance := balance - amount
		if newBalance <= 0 {
			db.Execute("UPDATE invoices SET state=?, updated_at=? WHERE id=?",
				lib.Text("Paid"), lib.Int(now), lib.Text(body.InvoiceID))
			newBalance = 0
		} else {
			db.Execute("UPDATE invoices SET updated_at=? WHERE id=?", lib.Int(now), lib.Text(body.InvoiceID))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": id, "paidAmount": lib.FormatMoney(paid + amount),
			"balance": lib.FormatMoney(newBalance),
		})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func paidForInvoice(db *lib.TursoClient, invoiceID string) (int64, error) {
	res, err := db.Execute("SELECT amount FROM payments WHERE invoice_id = ?", lib.Text(invoiceID))
	if err != nil {
		return 0, err
	}
	total := int64(0)
	for _, row := range res.Rows {
		total += lib.ParseMoney(lib.ToString(row[0]))
	}
	return total, nil
}

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
