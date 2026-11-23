package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "uploaded"
	}
	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}
	db := lib.NewTurso()
	res, err := db.Execute("SELECT id, client_id, captured_at, status, size_bytes, sha256, created_at FROM scans WHERE status = ? ORDER BY created_at ASC LIMIT ?",
		lib.Text(status), lib.Int(int64(limit)))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	scans := []map[string]interface{}{}
	for _, row := range res.Rows {
		scans = append(scans, map[string]interface{}{
			"id": row[0], "clientId": row[1], "capturedAt": row[2],
			"status": row[3], "sizeBytes": row[4], "sha256": row[5], "createdAt": row[6],
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"scans": scans})
}