package main

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		SetupSecret string `json:"setupSecret"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	expected := os.Getenv("ERP_SETUP_SECRET")
	if expected != "" && body.SetupSecret != expected {
		http.Error(w, "invalid setup secret", http.StatusUnauthorized)
		return
	}
	if body.SetupSecret == "" {
		http.Error(w, "setupSecret required", http.StatusBadRequest)
		return
	}
	db := lib.NewTurso()
	token := lib.NewToken()
	hash := lib.Sha256Hex(token)
	db.Execute("INSERT INTO agent_tokens (id, token_hash, created_at) VALUES (?,?,?)",
		lib.Text(lib.NewID()), lib.Text(hash), lib.Int(time.Now().Unix()))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"agentToken": token})
}