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
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		res, err := db.Execute("SELECT id, reference, category, description, amount, is_archived, created_at, updated_at FROM expenses WHERE id=?", lib.Text(id))
		if err != nil || len(res.Rows) == 0 {
			http.Error(w, "expense not found", http.StatusNotFound)
			return
		}
		row := res.Rows[0]
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"id": row[0], "reference": row[1], "category": row[2], "description": row[3], "amount": row[4], "isArchived": lib.ToInt(row[5]) == 1, "createdAt": row[6], "updatedAt": row[7]})

	case http.MethodDelete:
		if v := r.URL.Query().Get("baseUpdatedAt"); v != "" {
			if base, err := strconv.ParseInt(v, 10, 64); err == nil && base > 0 {
				cur, err := db.Execute("SELECT updated_at FROM expenses WHERE id=?", lib.Text(id))
				if err == nil && len(cur.Rows) > 0 && lib.ToInt64(cur.Rows[0][0]) != base {
					lib.WriteConflict(w, lib.ToInt64(cur.Rows[0][0]))
					return
				}
			}
		}
		now := time.Now().Unix()
		if _, err := db.Execute("UPDATE expenses SET is_archived=1, updated_at=? WHERE id=?", lib.Int(now), lib.Text(id)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
