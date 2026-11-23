package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db := lib.NewTurso()
	q := `SELECT id, transaction_type, reference_id, description, amount, is_reversed, created_at
		FROM journal_entries`
	args := []lib.TursoArg{}
	where := []string{}
	if v := r.URL.Query().Get("type"); v != "" {
		where = append(where, "transaction_type = ?")
		args = append(args, lib.Text(v))
	}
	if v := r.URL.Query().Get("start"); v != "" {
		if n, ok := atoi64(v); ok {
			where = append(where, "created_at >= ?")
			args = append(args, lib.Int(n))
		}
	}
	if v := r.URL.Query().Get("end"); v != "" {
		if n, ok := atoi64(v); ok {
			where = append(where, "created_at < ?")
			args = append(args, lib.Int(n))
		}
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY created_at DESC LIMIT 300"
	res, err := db.Execute(q, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := []map[string]interface{}{}
	net := int64(0)
	for _, row := range res.Rows {
		amount := lib.ParseMoney(lib.ToString(row[4]))
		if lib.ToInt(row[5]) == 0 {
			net += amount
		}
		items = append(items, map[string]interface{}{
			"id": row[0], "transactionType": row[1], "referenceId": row[2],
			"description": row[3], "amount": row[4],
			"isReversed": lib.ToInt(row[5]) == 1, "createdAt": row[6],
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"entries": items, "net": lib.FormatMoney(net)})
}

func atoi64(s string) (int64, bool) {
	var n int64
	if s == "" {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
	}
	return n, true
}
