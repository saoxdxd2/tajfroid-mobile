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

var expenseCategories = map[string]bool{
	"Salary": true, "Utilities": true, "Rent": true, "Equipment": true,
	"Marketing": true, "Travel": true, "Tax": true, "Other": true,
}

func Handler(w http.ResponseWriter, r *http.Request) {
	db := lib.NewTurso()
	switch r.Method {
	case http.MethodGet:
		includeArchived := r.URL.Query().Get("archived") == "1"
		q := "SELECT id, reference, category, description, amount, is_archived, created_at FROM expenses"
		if !includeArchived {
			q += " WHERE is_archived = 0"
		}
		q += " ORDER BY created_at DESC LIMIT 300"
		res, err := db.Execute(q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := []map[string]interface{}{}
		total := int64(0)
		for _, row := range res.Rows {
			amount := lib.ParseMoney(lib.ToString(row[4]))
			total += amount
			items = append(items, map[string]interface{}{
				"id": row[0], "reference": row[1], "category": row[2],
				"description": row[3], "amount": row[4],
				"isArchived": lib.ToInt(row[5]) == 1, "createdAt": row[6],
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"expenses": items, "total": lib.FormatMoney(total)})

	case http.MethodPost:
		var body struct {
			ID          string `json:"id"` // optional client idempotency key
			Reference   string `json:"reference"`
			Category    string `json:"category"`
			Description string `json:"description"`
			Amount      string `json:"amount"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.ID != "" {
			if ex, err := db.Execute("SELECT id, reference FROM expenses WHERE id=?", lib.Text(body.ID)); err == nil && len(ex.Rows) > 0 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{"id": body.ID, "reference": ex.Rows[0][1], "idempotent": true})
				return
			}
		}
		if strings.TrimSpace(body.Description) == "" {
			http.Error(w, "description required", http.StatusBadRequest)
			return
		}
		if body.Category == "" {
			body.Category = "Other"
		}
		if !expenseCategories[body.Category] {
			http.Error(w, "invalid expense category", http.StatusBadRequest)
			return
		}
		amount := lib.ParseMoney(body.Amount)
		if amount <= 0 {
			http.Error(w, "expense amount must be strictly positive", http.StatusBadRequest)
			return
		}
		reference := strings.TrimSpace(body.Reference)
		if reference == "" {
			reference = nextExpenseReference(db)
		}
		id := body.ID
		if id == "" {
			id = lib.NewID()
		}
		now := time.Now().Unix()
		_, err := db.Execute(`INSERT INTO expenses
			(id, reference, category, description, amount, is_archived, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?)`,
			lib.Text(id), lib.Text(reference), lib.Text(body.Category),
			lib.Text(body.Description), lib.Text(lib.FormatMoney(amount)),
			lib.Int(0), lib.Int(now), lib.Int(now))
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "UNIQUE") {
				status = http.StatusConflict
			}
			http.Error(w, err.Error(), status)
			return
		}
		lib.Journal(db, lib.TxExpense, reference, body.Description, -amount, time.Now())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": id, "reference": reference})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func nextExpenseReference(db *lib.TursoClient) string {
	yy := fmt.Sprintf("%02d", time.Now().Year()%100)
	prefix := "EXP-" + yy + "-"
	res, err := db.Execute("SELECT reference FROM expenses WHERE reference LIKE ?", lib.Text(prefix+"%"))
	if err != nil {
		return prefix + "01"
	}
	maxSeq := 0
	for _, row := range res.Rows {
		middle := strings.TrimPrefix(lib.ToString(row[0]), prefix)
		if n, err := strconv.Atoi(middle); err == nil && n > maxSeq {
			maxSeq = n
		}
	}
	return fmt.Sprintf("%s%02d", prefix, maxSeq+1)
}
