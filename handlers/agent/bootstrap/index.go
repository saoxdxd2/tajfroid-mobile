package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"tajfroid-mobile/lib"
)

// Bootstrap issues the desktop agent token. It is inherently unauthenticated
// (the agent has no token yet), so it is limited to the very first call: once
// one agent token exists, only an authenticated agent may mint another one.
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		DeviceID string `json:"deviceId"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.DeviceID == "" {
		http.Error(w, "deviceId required", http.StatusBadRequest)
		return
	}
	db := lib.NewTurso()
	count, err := db.Execute("SELECT id FROM agent_tokens LIMIT 1")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(count.Rows) > 0 {
		http.Error(w, "an agent is already registered; authenticate with its token", http.StatusConflict)
		return
	}
	token := lib.NewToken()
	hash := lib.Sha256Hex(token)
	if _, err := db.Execute("INSERT INTO agent_tokens (id, token_hash, device_id, created_at) VALUES (?,?,?,?)",
		lib.Text(lib.NewID()), lib.Text(hash), lib.Text(body.DeviceID), lib.Int(time.Now().Unix())); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"agentToken": token})
}
