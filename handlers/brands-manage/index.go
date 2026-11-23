package handler

import (
	"encoding/json"
	"net/http"
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
			cur, err := db.Execute("SELECT updated_at FROM brands WHERE id=?", lib.Text(id))
			if err == nil && len(cur.Rows) > 0 && lib.ToInt64(cur.Rows[0][0]) != body.BaseUpdatedAt {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(map[string]interface{}{"error":"conflict: record changed on another device","currentUpdatedAt": lib.ToInt64(cur.Rows[0][0])})
				return
			}
		}
		now := time.Now().Unix()
		db.Execute("UPDATE brands SET name=?, updated_at=? WHERE id=?", lib.Text(body.Name), lib.Int(now), lib.Text(id))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	case http.MethodDelete:
		db.Execute("DELETE FROM brands WHERE id=?", lib.Text(id))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}