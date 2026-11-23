package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tajfroid-mobile/lib"
)

// Invoice lifecycle mirrors the desktop ERP:
// Draft -> Validated -> Issued -> Paid, plus Archived.
var invoiceStates = map[string]bool{
	"Draft": true, "Validated": true, "Issued": true, "Paid": true, "Archived": true,
}

func Handler(w http.ResponseWriter, r *http.Request) {
	db := lib.NewTurso()
	switch r.Method {
	case http.MethodGet:
		includeArchived := r.URL.Query().Get("archived") == "1"
		q := `SELECT i.id, i.invoice_number, i.customer_id, c.company_name, i.state,
			i.total_amount, i.channel, i.due_date, i.notes, i.created_at, i.updated_at
			FROM invoices i LEFT JOIN customers c ON i.customer_id = c.id`
		if !includeArchived {
			q += " WHERE i.state != 'Archived'"
		}
		q += " ORDER BY i.created_at DESC, i.id DESC"
		res, err := db.Execute(q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		paid := paidByInvoice(db)
		items := []map[string]interface{}{}
		for _, row := range res.Rows {
			id := lib.ToString(row[0])
			total := lib.ParseMoney(lib.ToString(row[5]))
			items = append(items, map[string]interface{}{
				"id": id, "invoiceNumber": lib.ToString(row[1]),
				"customerId": lib.ToString(row[2]), "customerName": nullableStr(row[3]),
				"state": lib.ToString(row[4]), "totalAmount": lib.FormatMoney(total),
				"paidAmount": lib.FormatMoney(paid[id]),
				"balance":    lib.FormatMoney(total - paid[id]),
				"channel":    lib.ToString(row[6]), "dueDate": row[7], "notes": nullableStr(row[8]),
				"createdAt": row[9], "updatedAt": row[10],
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"invoices": items})

	case http.MethodPost:
		var body struct {
			ID         string `json:"id"` // optional client idempotency key
			CustomerID string `json:"customerId"`
			Channel    string `json:"channel"`
			Notes      string `json:"notes"`
			DueDate    int64  `json:"dueDate"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.ID != "" {
			if ex, err := db.Execute("SELECT id, invoice_number FROM invoices WHERE id=?", lib.Text(body.ID)); err == nil && len(ex.Rows) > 0 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{"id": body.ID, "invoiceNumber": ex.Rows[0][1], "idempotent": true})
				return
			}
		}
		channel := body.Channel
		if channel == "" {
			channel = "mobile_app"
		}
		if channel != "store" && channel != "website" && channel != "mobile_app" {
			http.Error(w, "invalid channel", http.StatusBadRequest)
			return
		}
		customerID := body.CustomerID
		if customerID == "" {
			var err error
			customerID, err = ensureWalkInCustomer(db)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		// Confirm the customer exists (and is not archived).
		check, err := db.Execute("SELECT company_name FROM customers WHERE id = ? AND is_archived = 0", lib.Text(customerID))
		if err != nil || len(check.Rows) == 0 {
			http.Error(w, "customer not found", http.StatusNotFound)
			return
		}
		id := body.ID
		if id == "" {
			id = lib.NewID()
		}
		now := time.Now().Unix()
		invNum := ""
		var insertErr error
		for attempt := 0; attempt < 5; attempt++ {
			invNum, insertErr = nextInvoiceNumber(db)
			if insertErr != nil {
				http.Error(w, insertErr.Error(), http.StatusInternalServerError)
				return
			}
			_, insertErr = db.Execute(`INSERT INTO invoices
				(id, invoice_number, customer_id, state, total_amount, channel, due_date, notes, created_at, updated_at)
				VALUES (?,?,?,?,?,?,?,?,?,?)`,
				lib.Text(id), lib.Text(invNum), lib.Text(customerID), lib.Text("Draft"),
				lib.Text("0.00"), lib.Text(channel), nullableInt(body.DueDate),
				nullableText(body.Notes), lib.Int(now), lib.Int(now))
			if insertErr == nil {
				break
			}
		}
		if insertErr != nil {
			http.Error(w, insertErr.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": id, "invoiceNumber": invNum})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// nextInvoiceNumber returns the next sequential number for the current year:
// N°01-26, N°02-26, ... Exactly like the desktop SalesService.
func nextInvoiceNumber(db *lib.TursoClient) (string, error) {
	yy := fmt.Sprintf("%02d", time.Now().Year()%100)
	suffix := "-" + yy
	res, err := db.Execute("SELECT invoice_number FROM invoices WHERE invoice_number LIKE ?", lib.Text("N°%"+suffix))
	if err != nil {
		return "", err
	}
	maxSeq := 0
	for _, row := range res.Rows {
		number := lib.ToString(row[0])
		middle := strings.TrimSuffix(strings.TrimPrefix(number, "N°"), suffix)
		if n, err := strconv.Atoi(middle); err == nil && n > maxSeq {
			maxSeq = n
		}
	}
	return fmt.Sprintf("N°%02d-%s", maxSeq+1, yy), nil
}

// ensureWalkInCustomer mirrors the desktop CRM "Client de passage (Walk-in)".
func ensureWalkInCustomer(db *lib.TursoClient) (string, error) {
	const name = "Client de passage (Walk-in)"
	res, err := db.Execute("SELECT id FROM customers WHERE company_name = ? AND is_archived = 0 LIMIT 1", lib.Text(name))
	if err != nil {
		return "", err
	}
	if len(res.Rows) > 0 {
		return lib.ToString(res.Rows[0][0]), nil
	}
	id := lib.NewID()
	now := time.Now().Unix()
	_, err = db.Execute(`INSERT INTO customers
		(id, party_type, company_name, is_archived, created_at, updated_at)
		VALUES (?,?,?,?,?,?)`,
		lib.Text(id), lib.Text("enterprise"), lib.Text(name), lib.Int(0), lib.Int(now), lib.Int(now))
	if err != nil {
		return "", err
	}
	return id, nil
}

// paidByInvoice sums recorded payments per invoice in integer cents.
func paidByInvoice(db *lib.TursoClient) map[string]int64 {
	out := map[string]int64{}
	res, err := db.Execute("SELECT invoice_id, amount FROM payments")
	if err != nil {
		return out
	}
	for _, row := range res.Rows {
		out[lib.ToString(row[0])] += lib.ParseMoney(lib.ToString(row[1]))
	}
	return out
}

func nullableStr(v interface{}) string {
	if v == nil {
		return ""
	}
	return lib.ToString(v)
}

func nullableText(s string) lib.TursoArg {
	if strings.TrimSpace(s) == "" {
		return lib.Null()
	}
	return lib.Text(s)
}

func nullableInt(n int64) lib.TursoArg {
	if n <= 0 {
		return lib.Null()
	}
	return lib.Int(n)
}
