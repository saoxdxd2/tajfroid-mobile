package handler

import (
	"encoding/json"
	"net/http"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db := lib.NewTurso()
	res, err := db.Execute(`SELECT id, name, paired_at, revoked_at, created_at FROM devices ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := []map[string]interface{}{}
	for _, row := range res.Rows {
		items = append(items, map[string]interface{}{
			"id": row[0], "name": row[1], "pairedAt": row[2],
			"revokedAt": row[3], "baseRevokedAt": lib.ToInt64(row[3]),
			"createdAt": row[4],
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"devices": items})
}
