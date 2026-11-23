package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	var body struct {
		Status            string `json:"status"`
		PurchaseReference string `json:"purchaseReference"`
		Message           string `json:"message"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if id == "" || body.Status == "" {
		http.Error(w, "id and status required", http.StatusBadRequest)
		return
	}
	db := lib.NewTurso()
	now := time.Now().Unix()
	terminal := body.Status == "imported" || body.Status == "rejected" || body.Status == "failed"
	terminalVal := lib.Null()
	if terminal {
		terminalVal = lib.Int(now)
	}
	db.Execute(`UPDATE scans SET status=?, purchase_reference=COALESCE(?,purchase_reference), message=COALESCE(?,message), updated_at=?, terminal_at=? WHERE id=?`,
		lib.Text(body.Status), nullableText(body.PurchaseReference), nullableText(body.Message), lib.Int(now), terminalVal, lib.Text(id))
	db.Execute("INSERT INTO scan_events (scan_id, to_status, at, note) VALUES (?,?,?,?)",
		lib.Text(id), lib.Text(body.Status), lib.Int(now), nullableText(body.Message))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func nullableText(s string) lib.TursoArg {
	if s == "" {
		return lib.Null()
	}
	return lib.Text(s)
}