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
	var body struct {
		DeviceID string `json:"deviceId"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.DeviceID == "" {
		http.Error(w, "deviceId required", http.StatusBadRequest)
		return
	}
	db := lib.NewTurso()
	token := lib.NewToken()
	hash := lib.Sha256Hex(token)
	db.Execute("INSERT INTO agent_tokens (id, token_hash, created_at) VALUES (?,?,?)",
		lib.Text(lib.NewID()), lib.Text(hash), lib.Int(time.Now().Unix()))
	// Store the device_id in a new column? For now, log it. TODO: add device_id column to agent_tokens.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"agentToken": token})
}