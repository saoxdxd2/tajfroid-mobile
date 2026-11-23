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
		res, err := db.Execute("SELECT id, name, created_at, updated_at FROM categories WHERE id=?", lib.Text(id))
		if err != nil || len(res.Rows) == 0 {
			http.Error(w, "category not found", http.StatusNotFound)
			return
		}
		row := res.Rows[0]
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"id": row[0], "name": row[1], "createdAt": row[2], "updatedAt": row[3]})

	case http.MethodPut:
		var body struct {
			Name          string `json:"name"`
			BaseUpdatedAt int64  `json:"baseUpdatedAt"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		if body.BaseUpdatedAt > 0 {
			cur, err := db.Execute("SELECT updated_at FROM categories WHERE id=?", lib.Text(id))
			if err == nil && len(cur.Rows) > 0 && lib.ToInt64(cur.Rows[0][0]) != body.BaseUpdatedAt {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(map[string]interface{}{"error":"conflict: record changed on another device","currentUpdatedAt": lib.ToInt64(cur.Rows[0][0])})
				return
			}
		}
		now := time.Now().Unix()
		db.Execute("UPDATE categories SET name=?, updated_at=? WHERE id=?", lib.Text(body.Name), lib.Int(now), lib.Text(id))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	case http.MethodDelete:
		if v := r.URL.Query().Get("baseUpdatedAt"); v != "" {
			if base, err := strconv.ParseInt(v, 10, 64); err == nil && base > 0 {
				if cur, err := db.Execute("SELECT updated_at FROM categories WHERE id=?", lib.Text(id)); err == nil && len(cur.Rows) > 0 && lib.ToInt64(cur.Rows[0][0]) != base {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					json.NewEncoder(w).Encode(map[string]interface{}{"error": "conflict: record changed on another device", "currentUpdatedAt": lib.ToInt64(cur.Rows[0][0])})
					return
				}
			}
		}
		db.Execute("DELETE FROM categories WHERE id=?", lib.Text(id))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}