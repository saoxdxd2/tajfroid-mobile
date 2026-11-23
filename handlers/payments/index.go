package handler

import (
	"encoding/json"
	"net/http"
	"strings"
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
			ID        string `json:"id"` // optional client-supplied idempotency key (UUID)
			InvoiceID string `json:"invoiceId"`
			Method    string `json:"method"`
			Amount    string `json:"amount"`
			CheckID   string `json:"checkId"`
			Reference string `json:"reference"`
			Notes     string `json:"notes"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.ID != "" {
			// Idempotent replay: if this payment already exists, return it.
			ex, err := db.Execute("SELECT id, amount FROM payments WHERE id = ?", lib.Text(body.ID))
			if err == nil && len(ex.Rows) > 0 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{"id": body.ID, "idempotent": true, "amount": ex.Rows[0][1]})
				return
			}
		}
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
		res, err := db.Execute("SELECT state, total_amount, invoice_number, customer_id FROM invoices WHERE id = ?", lib.Text(body.InvoiceID))
		if err != nil || len(res.Rows) == 0 {
			http.Error(w, "invoice not found", http.StatusNotFound)
			return
		}
		state := lib.ToString(res.Rows[0][0])
		if state != "Validated" && state != "Issued" {
			http.Error(w, "payments are only allowed on validated/issued invoices", http.StatusBadRequest)
			return
		}
		invoiceNumber := lib.ToString(res.Rows[0][2])
		customerID := lib.ToString(res.Rows[0][3])
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

		checkID := ""
		if body.Method == "Check" {
			checkID, err = findIncomingCheck(db, body.CheckID, body.Reference, amount, customerID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		} else if body.CheckID != "" {
			http.Error(w, "checkId is only valid for Check payments", http.StatusBadRequest)
			return
		}

		id := body.ID
		if id == "" {
			id = lib.NewID()
		}
		now := time.Now().Unix()
		_, err = db.Execute(`INSERT INTO payments
			(id, invoice_id, method, amount, check_id, reference, notes, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			lib.Text(id), lib.Text(body.InvoiceID), lib.Text(body.Method),
			lib.Text(lib.FormatMoney(amount)), nullableText(checkID),
			nullableText(body.Reference), nullableText(body.Notes), lib.Int(now), lib.Int(now))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// A check only becomes money when it clears, exactly like the desktop
		// journal; cash and transfers are recorded immediately.
		if body.Method != "Check" {
			lib.Journal(db, lib.TxPaymentReceived, "INV-"+invoiceNumber,
				"Payment ("+body.Method+") for invoice "+invoiceNumber, amount, time.Now())
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
			"balance": lib.FormatMoney(newBalance), "checkId": checkID,
		})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// findIncomingCheck resolves and validates the paper check backing a payment.
func findIncomingCheck(db *lib.TursoClient, checkID, number string, amount int64, customerID string) (string, error) {
	if checkID == "" {
		number = strings.TrimSpace(number)
		if number == "" {
			return "", errText("a check payment needs a check number")
		}
		res, err := db.Execute("SELECT id FROM checks WHERE check_number=? AND direction='Incoming' ORDER BY created_at DESC LIMIT 1", lib.Text(number))
		if err != nil {
			return "", err
		}
		if len(res.Rows) == 0 {
			return "", errText("no incoming check #" + number + " found; register it in Finance first")
		}
		checkID = lib.ToString(res.Rows[0][0])
	}
	res, err := db.Execute("SELECT direction, status, amount, customer_id FROM checks WHERE id = ?", lib.Text(checkID))
	if err != nil {
		return "", err
	}
	if len(res.Rows) == 0 {
		return "", errText("check not found")
	}
	direction := lib.ToString(res.Rows[0][0])
	status := lib.ToString(res.Rows[0][1])
	checkAmount := lib.ParseMoney(lib.ToString(res.Rows[0][2]))
	checkCustomer := lib.ToString(res.Rows[0][3])
	if direction != "Incoming" {
		return "", errText("only incoming customer checks can pay a sales invoice")
	}
	if checkCustomer != "" && checkCustomer != customerID {
		return "", errText("the check does not belong to the customer on this invoice")
	}
	if status != "Pending" && status != "Deposited" && status != "Cleared" {
		return "", errText("check is " + status + "; it cannot be used for a new payment")
	}
	if checkAmount != amount {
		return "", errText("check amount " + lib.FormatMoney(checkAmount) + " does not match payment " + lib.FormatMoney(amount))
	}
	linked, err := db.Execute("SELECT id FROM payments WHERE check_id = ?", lib.Text(checkID))
	if err != nil {
		return "", err
	}
	if len(linked.Rows) > 0 {
		return "", errText("this check is already linked to a payment")
	}
	return checkID, nil
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
