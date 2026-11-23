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

func Handler(w http.ResponseWriter, r *http.Request) {
	db := lib.NewTurso()
	switch r.Method {
	case http.MethodGet:
		res, err := db.Execute(`SELECT p.id, p.reference, p.supplier_id, s.company_name, p.state,
			p.total_amount, p.purchase_date, p.supplier_invoice_no, p.created_at, p.updated_at
			FROM purchases p LEFT JOIN suppliers s ON p.supplier_id = s.id
			ORDER BY p.created_at DESC, p.id DESC`)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := []map[string]interface{}{}
		for _, row := range res.Rows {
			items = append(items, map[string]interface{}{
				"id": row[0], "reference": row[1], "supplierId": row[2],
				"supplierName": lib.NullStr(row[3]), "state": row[4],
				"totalAmount": row[5], "purchaseDate": row[6],
				"supplierInvoiceNo": lib.NullStr(row[7]),
				"createdAt": row[8], "updatedAt": row[9],
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"purchases": items})

	case http.MethodPost:
		var body struct {
			ID               string `json:"id"` // optional client idempotency key
			SupplierID       string `json:"supplierId"`
			Reference        string `json:"reference"`
			PurchaseDate     int64  `json:"purchaseDate"`
			SupplierInvoiceNo string `json:"supplierInvoiceNo"`
			SourceFileName   string `json:"sourceFileName"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.ID != "" {
			if ex, err := db.Execute("SELECT id, reference FROM purchases WHERE id=?", lib.Text(body.ID)); err == nil && len(ex.Rows) > 0 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{"id": body.ID, "reference": ex.Rows[0][1], "idempotent": true})
				return
			}
		}
		if body.SupplierID == "" {
			http.Error(w, "supplierId required", http.StatusBadRequest)
			return
		}
		check, err := db.Execute("SELECT company_name FROM suppliers WHERE id = ? AND is_archived = 0", lib.Text(body.SupplierID))
		if err != nil || len(check.Rows) == 0 {
			http.Error(w, "supplier not found", http.StatusNotFound)
			return
		}
		reference := strings.TrimSpace(body.Reference)
		if reference == "" {
			reference, err = nextPurchaseReference(db)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		id := body.ID
		if id == "" {
			id = lib.NewID()
		}
		now := time.Now().Unix()
		_, err = db.Execute(`INSERT INTO purchases
			(id, reference, supplier_id, state, total_amount, purchase_date, supplier_invoice_no, source_file_name, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			lib.Text(id), lib.Text(reference), lib.Text(body.SupplierID), lib.Text("Draft"),
			lib.Text("0.00"), nullableInt(body.PurchaseDate), nullableText(body.SupplierInvoiceNo),
			nullableText(body.SourceFileName), lib.Int(now), lib.Int(now))
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "UNIQUE") {
				status = http.StatusConflict
			}
			http.Error(w, err.Error(), status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": id, "reference": reference})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// nextPurchaseReference produces PUR-YY-NN, sequential per year.
func nextPurchaseReference(db *lib.TursoClient) (string, error) {
	yy := fmt.Sprintf("%02d", time.Now().Year()%100)
	prefix := "PUR-" + yy + "-"
	res, err := db.Execute("SELECT reference FROM purchases WHERE reference LIKE ?", lib.Text(prefix+"%"))
	if err != nil {
		return "", err
	}
	maxSeq := 0
	for _, row := range res.Rows {
		middle := strings.TrimPrefix(lib.ToString(row[0]), prefix)
		if n, err := strconv.Atoi(middle); err == nil && n > maxSeq {
			maxSeq = n
		}
	}
	return fmt.Sprintf("%s%02d", prefix, maxSeq+1), nil
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
