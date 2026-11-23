package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"tajfroid-mobile/lib"
)

// allowedTransitions mirrors the desktop CheckStatus lifecycle.
var allowedTransitions = map[string]map[string]bool{
	"Pending":   {"Deposited": true, "Cleared": true, "Cancelled": true},
	"Deposited": {"Cleared": true, "Bounced": true},
	"Bounced":   {"Deposited": true, "Cancelled": true},
}

func Handler(w http.ResponseWriter, r *http.Request) {
	db := lib.NewTurso()
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		res, err := db.Execute(`SELECT id, check_number, direction, status, amount, due_date,
			bank, party_name, customer_id, supplier_id, notes, created_at, updated_at
			FROM checks WHERE id = ?`, lib.Text(id))
		if err != nil || len(res.Rows) == 0 {
			http.Error(w, "check not found", http.StatusNotFound)
			return
		}
		row := res.Rows[0]
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": row[0], "checkNumber": row[1], "direction": row[2], "status": row[3],
			"amount": row[4], "dueDate": row[5], "bank": lib.NullStr(row[6]),
			"partyName": row[7], "customerId": lib.NullStr(row[8]), "supplierId": lib.NullStr(row[9]),
			"notes": lib.NullStr(row[10]), "createdAt": row[11], "updatedAt": row[12],
		})

	case http.MethodPut:
		var body struct {
			Status        string `json:"status"`
			BaseUpdatedAt int64  `json:"baseUpdatedAt"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Status == "" {
			http.Error(w, "status required", http.StatusBadRequest)
			return
		}
		if body.BaseUpdatedAt > 0 {
			cur, err := db.Execute("SELECT updated_at FROM checks WHERE id=?", lib.Text(id))
			if err == nil && len(cur.Rows) > 0 && lib.ToInt64(cur.Rows[0][0]) != body.BaseUpdatedAt {
				lib.WriteConflict(w, lib.ToInt64(cur.Rows[0][0]))
				return
			}
		}
		res, err := db.Execute("SELECT check_number, direction, status, amount, party_name FROM checks WHERE id = ?", lib.Text(id))
		if err != nil || len(res.Rows) == 0 {
			http.Error(w, "check not found", http.StatusNotFound)
			return
		}
		number := lib.ToString(res.Rows[0][0])
		direction := lib.ToString(res.Rows[0][1])
		status := lib.ToString(res.Rows[0][2])
		amount := lib.ParseMoney(lib.ToString(res.Rows[0][3]))
		party := lib.ToString(res.Rows[0][4])
		if status == body.Status {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"ok": true, "unchanged": true})
			return
		}
		if !allowedTransitions[status][body.Status] {
			http.Error(w, "cannot move check from "+status+" to "+body.Status, http.StatusBadRequest)
			return
		}
		now := time.Now().Unix()
		if body.Status == "Bounced" || body.Status == "Cancelled" {
			if err := reverseCheckPayments(db, id); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if _, err := db.Execute("UPDATE checks SET status=?, updated_at=? WHERE id=?",
			lib.Text(body.Status), lib.Int(now), lib.Text(id)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if body.Status == "Cleared" {
			signed := amount
			txType := lib.TxPaymentReceived
			if direction == "Outgoing" {
				signed = -amount
				txType = lib.TxPaymentSent
			}
			lib.Journal(db, txType, "CHECK-"+number,
				"Check #"+number+" cleared ("+party+")", signed, time.Now())
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// reverseCheckPayments undoes the provisional settlement of a check that will
// never clear: the linked payment disappears and an invoice that was marked
// Paid reopens as Validated.
func reverseCheckPayments(db *lib.TursoClient, checkID string) error {
	res, err := db.Execute("SELECT id, invoice_id, amount FROM payments WHERE check_id = ?", lib.Text(checkID))
	if err != nil {
		return err
	}
	for _, row := range res.Rows {
		paymentID := lib.ToString(row[0])
		invoiceID := lib.ToString(row[1])
		if _, err := db.Execute("DELETE FROM payments WHERE id = ?", lib.Text(paymentID)); err != nil {
			return err
		}
		inv, err := db.Execute("SELECT state, total_amount FROM invoices WHERE id = ?", lib.Text(invoiceID))
		if err != nil || len(inv.Rows) == 0 {
			continue
		}
		if lib.ToString(inv.Rows[0][0]) == "Paid" {
			paid, err := paidForInvoice(db, invoiceID)
			if err == nil && paid < lib.ParseMoney(lib.ToString(inv.Rows[0][1])) {
				db.Execute("UPDATE invoices SET state=?, updated_at=? WHERE id=?",
					lib.Text("Validated"), lib.Int(time.Now().Unix()), lib.Text(invoiceID))
			}
		}
	}
	return nil
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

