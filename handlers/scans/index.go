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
	switch r.Method {
	case http.MethodPost:
		var body struct {
			ClientID   string `json:"clientId"`
			CapturedAt int64  `json:"capturedAt"`
			SizeBytes  int64  `json:"sizeBytes"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.ClientID == "" {
			http.Error(w, "clientId required", http.StatusBadRequest)
			return
		}
		res, _ := db.Execute("SELECT id, status FROM scans WHERE client_id = ? LIMIT 1", lib.Text(body.ClientID))
		if len(res.Rows) > 0 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"scanId": res.Rows[0][0], "status": res.Rows[0][1], "idempotent": true,
			})
			return
		}
		id := lib.NewID()
		now := time.Now().Unix()
		if body.CapturedAt == 0 {
			body.CapturedAt = now
		}
		db.Execute("INSERT INTO scans (id, client_id, device_id, captured_at, status, size_bytes, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)",
			lib.Text(id), lib.Text(body.ClientID), lib.Text("dev-device"), lib.Int(body.CapturedAt),
			lib.Text("uploaded"), lib.Int(body.SizeBytes), lib.Int(now), lib.Int(now))
		db.Execute("INSERT INTO scan_events (scan_id, to_status, at) VALUES (?,?,?)",
			lib.Text(id), lib.Text("uploaded"), lib.Int(now))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"scanId": id, "status": "uploaded"})
	case http.MethodGet:
		since := int64(0)
		if v := r.URL.Query().Get("since"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				since = n
			}
		}
		res, err := db.Execute("SELECT id, client_id, status, purchase_reference, message, updated_at FROM scans WHERE updated_at > ? ORDER BY updated_at ASC LIMIT 200", lib.Int(since))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		scans := []map[string]interface{}{}
		cursor := since
		for _, row := range res.Rows {
			scans = append(scans, map[string]interface{}{
				"id": row[0], "clientId": row[1], "status": row[2],
				"purchaseReference": row[3], "message": row[4], "updatedAt": row[5],
			})
			if v, ok := row[5].(float64); ok && int64(v) > cursor {
				cursor = int64(v)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"scans": scans, "cursor": cursor})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}