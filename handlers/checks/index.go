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
	switch r.Method {
	case http.MethodGet:
		q := `SELECT c.id, c.check_number, c.direction, c.status, c.amount, c.due_date,
			c.bank, c.party_name, c.customer_id, c.supplier_id, c.notes, c.created_at, c.updated_at
			FROM checks c`
		args := []lib.TursoArg{}
		status := r.URL.Query().Get("status")
		if status != "" {
			q += " WHERE c.status = ?"
			args = append(args, lib.Text(status))
		}
		q += " ORDER BY c.due_date ASC"
		res, err := db.Execute(q, args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := []map[string]interface{}{}
		for _, row := range res.Rows {
			items = append(items, map[string]interface{}{
				"id": row[0], "checkNumber": row[1], "direction": row[2],
				"status": row[3], "amount": row[4], "dueDate": row[5],
				"bank": lib.NullStr(row[6]), "partyName": row[7],
				"customerId": lib.NullStr(row[8]), "supplierId": lib.NullStr(row[9]),
				"notes": lib.NullStr(row[10]), "createdAt": row[11], "updatedAt": row[12],
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"checks": items})

	case http.MethodPost:
		var body struct {
			ID          string `json:"id"` // optional client idempotency key
			CheckNumber string `json:"checkNumber"`
			Direction   string `json:"direction"`
			Amount      string `json:"amount"`
			DueDate     int64  `json:"dueDate"`
			Bank        string `json:"bank"`
			PartyName   string `json:"partyName"`
			CustomerID  string `json:"customerId"`
			SupplierID  string `json:"supplierId"`
			Notes       string `json:"notes"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.ID != "" {
			if ex, err := db.Execute("SELECT id FROM checks WHERE id=?", lib.Text(body.ID)); err == nil && len(ex.Rows) > 0 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{"id": body.ID, "idempotent": true})
				return
			}
		}
		if strings.TrimSpace(body.CheckNumber) == "" {
			http.Error(w, "check number is required", http.StatusBadRequest)
			return
		}
		if body.Direction == "" {
			body.Direction = "Incoming"
		}
		if body.Direction != "Incoming" && body.Direction != "Outgoing" {
			http.Error(w, "invalid check direction", http.StatusBadRequest)
			return
		}
		amount := lib.ParseMoney(body.Amount)
		if amount <= 0 {
			http.Error(w, "check amount must be strictly positive", http.StatusBadRequest)
			return
		}
		if body.DueDate <= 0 {
			http.Error(w, "due date is required", http.StatusBadRequest)
			return
		}
		partyName := strings.TrimSpace(body.PartyName)
		if body.Direction == "Incoming" {
			if body.CustomerID == "" {
				http.Error(w, "incoming checks must be linked to a customer", http.StatusBadRequest)
				return
			}
			res, err := db.Execute("SELECT company_name FROM customers WHERE id = ?", lib.Text(body.CustomerID))
			if err != nil || len(res.Rows) == 0 {
				http.Error(w, "customer not found", http.StatusNotFound)
				return
			}
			if partyName == "" {
				partyName = lib.ToString(res.Rows[0][0])
			}
		} else {
			if body.SupplierID == "" {
				http.Error(w, "outgoing checks must be linked to a supplier", http.StatusBadRequest)
				return
			}
			res, err := db.Execute("SELECT company_name FROM suppliers WHERE id = ?", lib.Text(body.SupplierID))
			if err != nil || len(res.Rows) == 0 {
				http.Error(w, "supplier not found", http.StatusNotFound)
				return
			}
			if partyName == "" {
				partyName = lib.ToString(res.Rows[0][0])
			}
		}
		id := body.ID
		if id == "" {
			id = lib.NewID()
		}
		now := time.Now().Unix()
		_, err := db.Execute(`INSERT INTO checks
			(id, check_number, direction, status, amount, due_date, bank, party_name, customer_id, supplier_id, notes, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			lib.Text(id), lib.Text(strings.TrimSpace(body.CheckNumber)), lib.Text(body.Direction),
			lib.Text("Pending"), lib.Text(lib.FormatMoney(amount)), lib.Int(body.DueDate),
			nullableText(body.Bank), lib.Text(partyName), nullableText(body.CustomerID),
			nullableText(body.SupplierID), nullableText(body.Notes), lib.Int(now), lib.Int(now))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": id})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}


func nullableText(s string) lib.TursoArg {
	if strings.TrimSpace(s) == "" {
		return lib.Null()
	}
	return lib.Text(s)
}
