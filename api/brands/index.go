package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	db := lib.NewTurso()
	switch r.Method {
	case http.MethodGet:
		res, err := db.Execute("SELECT id, name, created_at, updated_at FROM brands ORDER BY name ASC")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := []map[string]interface{}{}
		for _, row := range res.Rows {
			items = append(items, map[string]interface{}{
				"id": row[0], "name": row[1], "createdAt": row[2], "updatedAt": row[3],
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"brands": items})

	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		now := time.Now().Unix()
		id := lib.NewID()
		_, err := db.Execute("INSERT INTO brands (id, name, created_at, updated_at) VALUES (?,?,?,?)",
			lib.Text(id), lib.Text(body.Name), lib.Int(now), lib.Int(now))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": id, "name": body.Name})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}